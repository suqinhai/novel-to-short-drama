'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const root = path.resolve(__dirname, '..');
const database = process.env.REBUILD_CLOSURE_DATABASE || 'short_drama_phase5_rebuild_closure';
const sourceDatabase = process.env.REBUILD_CLOSURE_SOURCE_DATABASE || 'short_drama_phase5_legacy_upgrade';
const postgresContainer = process.env.PHASE5_POSTGRES_CONTAINER || 'ai-short-drama-postgres-1';
const postgresUser = process.env.POSTGRES_USER || 'n8n';
const postgresPassword = process.env.POSTGRES_PASSWORD || 'change_me';
const postgresPort = process.env.POSTGRES_PORT || '5432';
const workerImage = process.env.PHASE5_MEDIA_WORKER_IMAGE || 'ai-short-drama-media-worker:n8n2.4.4-ffmpeg6.1.2';
const workerContainer = `rebuild-closure-${process.pid}`;
const safeDatabase = /^short_drama_phase5_[a-z0-9_]+$/;
if (!safeDatabase.test(database) || !safeDatabase.test(sourceDatabase)) {
  throw new Error('refusing unsafe rebuild closure database name');
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { cwd: root, env: process.env, encoding: 'utf8', windowsHide: true, ...options });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.status !== 0) throw new Error(`${command} ${args.join(' ')} exited ${result.status}`);
  return result;
}

function psql(target, sql) {
  run('docker', ['exec', '-i', postgresContainer, 'psql', '-X', '-v', 'ON_ERROR_STOP=1', '-U', postgresUser, '-d', target], { input: sql });
}

const storage = fs.mkdtempSync(path.join(os.tmpdir(), 'rebuild-closure-'));
let failed = false;
try {
  psql('postgres', `DROP DATABASE IF EXISTS ${database} WITH (FORCE);\nCREATE DATABASE ${database} TEMPLATE ${sourceDatabase};\n`);
  psql(database, `UPDATE drama.render_jobs SET status='cancelled',completed_at=now(),updated_at=now()
    WHERE status IN('pending','claimed','processing');`);
  const mounts = [
    `${storage}:/data/storage`,
    `${path.join(root, 'scripts/media-worker/worker.js')}:/app/worker.js:ro`,
    `${path.join(root, 'scripts/media-worker/rebuild-consumer.js')}:/app/rebuild-consumer.js:ro`,
    `${path.join(root, 'scripts/media-worker/ffmpeg-templates.js')}:/app/ffmpeg-templates.js:ro`,
  ];
  const args = ['run', '-d', '--name', workerContainer, '--add-host', 'host.docker.internal:host-gateway'];
  for (const mount of mounts) args.push('-v', mount);
  for (const entry of [
    `DATABASE_URL=postgres://${encodeURIComponent(postgresUser)}:${encodeURIComponent(postgresPassword)}@host.docker.internal:${postgresPort}/${database}?sslmode=disable`,
    'MEDIA_STORAGE_PATH=/data/storage', 'MEDIA_PUBLIC_BASE_URL=http://local.invalid/storage',
    'MEDIA_WORKER_PORT=8090', 'MEDIA_WORKER_POLL_INTERVAL_SECONDS=1', 'MEDIA_WORKER_HEARTBEAT_SECONDS=2',
    'MEDIA_WORKER_MAX_CONCURRENCY=1', 'MEDIA_WORKER_BATCH_SIZE=1', 'REBUILD_WORKER_ENABLED=true',
    'REBUILD_LOCAL_CONFORMANCE_ENABLED=true', 'REBUILD_LEASE_SECONDS=30', 'REBUILD_HEARTBEAT_SECONDS=2',
    'REBUILD_PROVIDER_TIMEOUT_SECONDS=30', 'REBUILD_RETRY_DELAY_SECONDS=1', 'MOCK_MODE=false',
  ]) args.push('-e', entry);
  args.push(workerImage);
  run('docker', args);
  const env = { ...process.env,
    REBUILD_CLOSURE_DATABASE_URL: `postgres://${encodeURIComponent(postgresUser)}:${encodeURIComponent(postgresPassword)}@127.0.0.1:${postgresPort}/${database}?sslmode=disable`,
    REBUILD_CLOSURE_STORAGE_DIR: storage,
  };
  run('go', ['test', '-count=1', '-p', '1', '-v', './internal/store', '-run', 'TestRebuildConsumerDeliveryClosureIntegration'], {
    cwd: path.join(root, 'cms/backend'), env,
  });
} catch (error) {
  failed = true;
  try { run('docker', ['logs', workerContainer, '--tail', '100']); } catch (_) {}
  console.error(error.message);
} finally {
  spawnSync('docker', ['rm', '-f', workerContainer], { encoding: 'utf8', windowsHide: true });
  try { psql('postgres', `DROP DATABASE IF EXISTS ${database} WITH (FORCE);\n`); } catch (error) { failed = true; console.error(error.message); }
  fs.rmSync(storage, { recursive: true, force: true });
}
if (failed) process.exitCode = 1;
