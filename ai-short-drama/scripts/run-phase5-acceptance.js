'use strict';

const fs = require('fs');
const path = require('path');
const {spawnSync} = require('child_process');

const root = path.resolve(__dirname, '..');
// Schema validation needs the jsonschema package. The system interpreter is
// the project-supported default; PHASE5_PYTHON can pin another prepared runtime.
const pythonCommand = process.env.PHASE5_PYTHON || 'python';
const container = process.env.PHASE5_POSTGRES_CONTAINER || 'ai-short-drama-postgres-1';
const freshDatabase = process.env.PHASE5_FRESH_DATABASE || 'short_drama_phase5_acceptance';
const legacyDatabase = process.env.PHASE5_LEGACY_DATABASE || 'short_drama_phase5_legacy_upgrade';
const compilerDatabase = process.env.PHASE5_COMPILER_DATABASE || 'short_drama_phase5_compiler_e2e';
const isolationBaseDatabase = process.env.PHASE5_ISOLATION_BASE_DATABASE || 'short_drama_phase5_isolation_base';
const safeDatabase = /^short_drama_phase5_[a-z0-9_]+$/;
for (const database of [freshDatabase, legacyDatabase, compilerDatabase, isolationBaseDatabase]) {
  if (!safeDatabase.test(database)) throw new Error(`refusing unsafe test database name: ${database}`);
}

const migrationFiles = [
  'database/init.sql', 'database/02-script-storyboard.sql', 'database/03-visual-assets-images.sql',
  'database/04-video-audio.sql', 'database/05-edit-qc-publish.sql', 'database/06-narrative-ir-foundation.sql',
  'database/07-adaptation-compiler-audit.sql', 'database/08-chapter-impact-analysis.sql',
  'database/09-phase5-contract-corrections.sql',
  'database/10-backfill-singular-script-dialogues.sql',
  'database/11-pgcrypto-runtime-prerequisite.sql',
  'database/12-rolling-episode-production.sql',
  'database/13-adaptation-diagnostics-pacing-quality.sql',
  'database/14-multi-candidate-selection.sql',
  'database/15-local-editing-workbench.sql',
  'database/16-performance-continuity-visual-qc.sql',
  'database/17-post-production-creative-workbench.sql',
  'database/18-effective-input-resolver.sql',
  'database/19-unified-versioned-change-entry.sql',
  'database/20-ir-merge-closure.sql',
  'database/21-pluggable-candidate-providers.sql',
  'database/22-restore-effective-input-stage-requirements.sql',
  'database/23-versioned-production-routing.sql',
  'database/24-authoritative-production-inputs.sql',
  'database/25-season-planning-workbench.sql',
  'database/26-atomic-multi-shot-editor.sql',
  'database/27-lightweight-nle.sql',
  'database/28-cross-layer-quality-gate.sql',
  'database/29-prompt-lab-professional-export.sql',
  'database/30-step-0-7-p0-p1-closure.sql',
  'database/31-step-8-10-p0-p1-closure.sql',
  'database/32-final-delivery-chain-closure.sql',
];
const legacyBaseFiles = migrationFiles.slice(0, 5);
const contractFiles = migrationFiles.slice(5);
const verifyFiles = [
  'database/06-verify-narrative-foundation.sql', 'database/07-verify-adaptation-compiler.sql',
  'database/08-verify-chapter-impact-analysis.sql', 'database/09-verify-phase5-contract-corrections.sql',
  'database/14-verify-multi-candidate-selection.sql',
  'database/16-verify-performance-continuity-visual-qc.sql',
  'database/17-verify-post-production-creative-workbench.sql',
  'database/18-verify-effective-input-resolver.sql',
  'database/19-verify-unified-versioned-change-entry.sql',
  'database/20-verify-ir-merge-closure.sql',
  'database/21-verify-pluggable-candidate-providers.sql',
  'database/22-verify-effective-input-stage-requirements.sql',
  'database/23-verify-versioned-production-routing.sql',
  'database/24-verify-authoritative-production-inputs.sql',
  'database/25-verify-season-planning-workbench.sql',
  'database/26-verify-atomic-multi-shot-editor.sql',
  'database/27-verify-lightweight-nle.sql',
  'database/28-verify-cross-layer-quality-gate.sql',
  'database/29-verify-prompt-lab-professional-export.sql',
  'database/31-verify-step-8-10-p0-p1-closure.sql',
  'database/32-verify-final-delivery-chain-closure.sql',
];

function loadEnv() {
  const result = {...process.env};
  for (const name of ['.env.example', '.env', 'cms/config/cms-managed.env']) {
    const file = path.join(root, name);
    if (!fs.existsSync(file)) continue;
    for (const line of fs.readFileSync(file, 'utf8').split(/\r?\n/)) {
      const match = line.match(/^([A-Za-z_][A-Za-z0-9_]*)=(.*)$/);
      if (match && !Object.prototype.hasOwnProperty.call(process.env, match[1])) result[match[1]] = match[2].trim().replace(/^['"]|['"]$/g, '');
    }
  }
  return result;
}
const commandEnv = loadEnv();
const testEnv = {...commandEnv, CANDIDATE_ENABLE_DETERMINISTIC_MOCK: 'true'};
const postgresUser = commandEnv.POSTGRES_USER || 'n8n';
const postgresPort = commandEnv.POSTGRES_PORT || '5432';
const postgresPassword = commandEnv.POSTGRES_PASSWORD || '';
const databaseURL = (database) => `postgres://${encodeURIComponent(postgresUser)}:${encodeURIComponent(postgresPassword)}@127.0.0.1:${postgresPort}/${database}?sslmode=disable`;

const outcomes = [];
function record(label, result) {
  const status = result.status == null ? 1 : result.status;
  outcomes.push({label, status});
  process.stdout.write(`\n[exit ${status}] ${label}\n`);
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.stderr) process.stderr.write(result.stderr);
  if (result.error) process.stderr.write(`${result.error.message}\n`);
  if (status !== 0) throw new Error(`${label} failed with exit ${status}`);
}
function run(label, command, args, options = {}) {
  process.stdout.write(`\n> ${label}\n`);
  const result = spawnSync(command, args, {cwd: root, env: commandEnv, encoding: 'utf8', windowsHide: true, ...options});
  record(label, result);
  return result;
}
function dockerPSQL(database, sql, label, quiet = true) {
  const args = ['exec', '-e', 'PGOPTIONS=-c client_min_messages=warning', '-i', container,
    'psql', '-X', '-v', 'ON_ERROR_STOP=1', '-U', postgresUser, '-d', database];
  if (quiet) args.push('-q');
  return run(label, 'docker', args, {input: sql});
}
function sqlFile(database, relative, label = relative) {
  const sql = fs.readFileSync(path.join(root, relative), 'utf8').replace(/^\uFEFF/, '');
  return dockerPSQL(database, sql, `${label} [${database}]`);
}
function recreate(database) {
  dockerPSQL('postgres', `DROP DATABASE IF EXISTS ${database};\nCREATE DATABASE ${database};\n`, `recreate isolated database ${database}`, false);
}
function drop(database) {
  dockerPSQL('postgres', `DROP DATABASE IF EXISTS ${database};\n`, `cleanup isolated database ${database}`, false);
}
function recreateClone(database, sourceDatabase) {
  dockerPSQL('postgres', `DROP DATABASE IF EXISTS ${database};\nCREATE DATABASE ${database} TEMPLATE ${sourceDatabase};\n`,
    `recreate isolated clone ${database} from ${sourceDatabase}`, false);
}
let isolatedCounter = 0;
function runIsolatedIntegration(label, envName, tests, options = {}) {
  isolatedCounter += 1;
  const database = `short_drama_phase5_case_${String(isolatedCounter).padStart(2, '0')}`;
  recreateClone(database, isolationBaseDatabase);
  const env = {...(options.env || commandEnv), [envName]: databaseURL(database)};
  try {
    return run(label, 'go', ['test', '-count=1', '-p', '1', '-v', './internal/store', '-run', tests],
      {cwd: options.cwd, env});
  } finally {
    drop(database);
  }
}

let failed = false;
try {
  recreate(freshDatabase);
  for (const file of migrationFiles) sqlFile(freshDatabase, file, `fresh apply ${file}`);
  for (let replay = 1; replay <= 2; replay += 1) {
    for (const file of migrationFiles) sqlFile(freshDatabase, file, `idempotent reapply ${replay} ${file}`);
  }
  for (const file of verifyFiles) sqlFile(freshDatabase, file, `verify ${file}`);
  sqlFile(freshDatabase, 'test-data/phase5-core-acceptance.sql', 'Phase 5 core relation/performance acceptance');

  recreate(legacyDatabase);
  for (const file of legacyBaseFiles) sqlFile(legacyDatabase, file, `legacy base ${file}`);
  sqlFile(legacyDatabase, 'test-data/phase1-legacy-seed.sql', 'seed explicit legacy IDs');
  for (const file of contractFiles) sqlFile(legacyDatabase, file, `legacy upgrade ${file}`);
  for (const file of verifyFiles) sqlFile(legacyDatabase, file, `legacy verify ${file}`);
  sqlFile(legacyDatabase, 'test-data/phase1-contract-seed.sql', 'seed traced Narrative IR fixture');
  sqlFile(legacyDatabase, 'test-data/phase3-compiler-db-seed.sql', 'seed adaptation compiler fixture');
  sqlFile(legacyDatabase, 'test-data/phase5-postproduction-fixture.sql', 'seed complete Phase 5 post-production mock episode');
  sqlFile(legacyDatabase, 'test-data/phase18-effective-input-resolver-acceptance.sql',
    'Effective Input Resolver authority, isolation, blocking and provenance acceptance');
  recreateClone(isolationBaseDatabase, legacyDatabase);

  recreate(compilerDatabase);
  for (const file of legacyBaseFiles) sqlFile(compilerDatabase, file, `compiler isolated base ${file}`);
  sqlFile(compilerDatabase, 'test-data/phase1-legacy-seed.sql', 'compiler seed explicit legacy IDs');
  for (const file of contractFiles) sqlFile(compilerDatabase, file, `compiler isolated upgrade ${file}`);
  sqlFile(compilerDatabase, 'test-data/phase1-contract-seed.sql', 'compiler seed traced Narrative IR fixture');
  sqlFile(compilerDatabase, 'test-data/phase3-compiler-db-seed.sql', 'compiler seed only its own operation');

  const backendCwd = path.join(root, 'cms/backend');
  run('Go backend unit tests', 'go', ['test', '-p', '1', './...'], {cwd: backendCwd, env: testEnv});
  run('Go Phase 2 source/spec integration including 1000 chapters', 'go', ['test', '-count=1', '-p', '1', '-v', './internal/store',
    '-run', 'TestAdaptationProjectAndSpecIntegration|TestSourceV2LifecycleIntegration|TestSourceV2ThousandChapterBatchIntegration'],
  {cwd: backendCwd, env: {...commandEnv, PHASE2_DATABASE_URL: databaseURL(freshDatabase)}});
  run('Go Phase 3 compiler integration', 'go', ['test', '-count=1', '-p', '1', '-v', './internal/store', '-run', 'TestCompilerRunLifecycleIntegration'],
    {cwd: backendCwd, env: {...commandEnv, PHASE3_DATABASE_URL: databaseURL(legacyDatabase)}});
  run('Go Phase 4 incremental extraction integration', 'go', ['test', '-count=1', '-p', '1', '-v', './internal/store', '-run', 'TestPublishedChapterRevisionQueuesOnlyChangedChapterIR'],
    {cwd: backendCwd, env: {...commandEnv, PHASE4_DATABASE_URL: databaseURL(freshDatabase)}});
  run('Go Phase 20 reviewed IR merge, UTF-8 spans and exact impact integration', 'go', ['test', '-count=1', '-p', '1', '-v', './internal/store',
    '-run', 'TestIRMergePublishesAtomicFullSnapshotAndPreservesUTF8Spans'],
  {cwd: backendCwd, env: {...commandEnv, PHASE20_DATABASE_URL: databaseURL(freshDatabase)}});

  run('Phase 3 compiler PostgreSQL E2E (valid + adversarial zero-write)', 'node', ['scripts/run-phase3-db-integration.js'], {
    env: {...commandEnv, PHASE3_TEST_DATABASE: compilerDatabase, PHASE3_POSTGRES_CONTAINER: container},
  });
  runIsolatedIntegration('Go Phase 15 exact local edit integration on latest schema', 'PHASE15_DATABASE_URL',
    'TestLocalEditingFourScenariosIntegration', {cwd: backendCwd});
  runIsolatedIntegration('Go atomic multi-shot concurrency, rollback, continuity and restore integration', 'PHASE15_DATABASE_URL',
    'TestAtomicMultiShotEditorIntegration', {cwd: backendCwd});
  runIsolatedIntegration('Go Phase 4 performance/continuity integration on latest schema', 'PHASE4_DATABASE_URL',
    'TestPerformanceContinuityPhase4Integration', {cwd: backendCwd, env: {...testEnv, MOCK_MODE: 'true'}});
  runIsolatedIntegration('Go Phase 5 complete mock, timing, template, restore and exact rebuild E2E', 'PHASE5_POST_DATABASE_URL',
    'TestPhase5PostProductionMockChainIntegration', {cwd: backendCwd});
  run('Go Effective Input Resolver read-only integration', 'go', ['test', '-count=1', '-p', '1', '-v', './internal/store', '-run', 'TestEffectiveInputResolverIntegration'],
    {cwd: backendCwd, env: {...commandEnv, PHASE18_DATABASE_URL: databaseURL(legacyDatabase)}});
  sqlFile(legacyDatabase, 'test-data/phase4-chapter-impact-e2e.sql', 'Phase 4 exact stale propagation E2E');
  run('Go Phase 4 impact review/regeneration integration', 'go', ['test', '-count=1', '-p', '1', '-v', './internal/store', '-run', 'TestChapterImpactReadAndDecisionIntegration'],
    {cwd: backendCwd, env: {...commandEnv, PHASE4_DATABASE_URL: databaseURL(legacyDatabase)}});
  run('Go contiguous IR-merge/change-plan/candidate/reviewer/Resolver production E2E', 'go', ['test', '-count=1', '-p', '1', '-v', './internal/store',
    '-run', 'TestPluggableCandidateFrozenReplayAndDownstreamConsumption'],
  {cwd: backendCwd, env: {...testEnv, PHASE21_DATABASE_URL: databaseURL(legacyDatabase)}});
  run('n8n seven-stage authoritative snapshot-loader E2E', 'node', ['scripts/validate-authoritative-n8n-e2e.js'], {
    env: {...commandEnv, PHASE5_TEST_DATABASE: legacyDatabase, PHASE5_POSTGRES_CONTAINER: container},
  });
  runIsolatedIntegration('Go whole-season workbench adversarial version/approval/queue integration', 'PHASE25_DATABASE_URL',
    'TestSeasonWorkbenchVersionApprovalAndQueueGateIntegration', {cwd: backendCwd});
  runIsolatedIntegration('Go final Step 8-10 target QA/export closure integration', 'PHASE31_DATABASE_URL',
    'TestStep810P0P1ClosureIntegration', {cwd: backendCwd});
  run('Go backend vet', 'go', ['vet', './...'], {cwd: backendCwd, env: commandEnv});

  const frontendCwd = path.join(root, 'cms/frontend');
  if (process.platform === 'win32') {
    run('CMS frontend unit tests', process.env.ComSpec || 'cmd.exe', ['/d', '/s', '/c', 'npm test'], {cwd: frontendCwd});
    run('CMS frontend production build', process.env.ComSpec || 'cmd.exe', ['/d', '/s', '/c', 'npm run build'], {cwd: frontendCwd});
  } else {
    run('CMS frontend unit tests', 'npm', ['test'], {cwd: frontendCwd});
    run('CMS frontend production build', 'npm', ['run', 'build'], {cwd: frontendCwd});
  }
  const mediaWorkerCwd = path.join(root, 'scripts/media-worker');
  if (process.platform === 'win32') {
    run('Media worker post-production tests', process.env.ComSpec || 'cmd.exe', ['/d', '/s', '/c', 'npm test'], {cwd: mediaWorkerCwd});
    run('Media worker syntax checks', process.env.ComSpec || 'cmd.exe', ['/d', '/s', '/c', 'npm run check'], {cwd: mediaWorkerCwd});
  } else {
    run('Media worker post-production tests', 'npm', ['test'], {cwd: mediaWorkerCwd});
    run('Media worker syntax checks', 'npm', ['run', 'check'], {cwd: mediaWorkerCwd});
  }
  const veoAdapterCwd = path.join(root, 'scripts/veo-adapter');
  if (process.platform === 'win32') {
    run('Veo adapter tests', process.env.ComSpec || 'cmd.exe', ['/d', '/s', '/c', 'npm test'], {cwd: veoAdapterCwd});
    run('Veo adapter syntax checks', process.env.ComSpec || 'cmd.exe', ['/d', '/s', '/c', 'npm run check'], {cwd: veoAdapterCwd});
  } else {
    run('Veo adapter tests', 'npm', ['test'], {cwd: veoAdapterCwd});
    run('Veo adapter syntax checks', 'npm', ['run', 'check'], {cwd: veoAdapterCwd});
  }

  for (const script of ['validate-phase1.js', 'validate-phase2.js', 'validate-phase2-ir.js',
    'validate-phase3-compiler.js', 'validate-phase4-impact.js', 'validate-phase4.js',
    'validate-phase5.js', 'validate-phase13.js', 'validate-phase14.js', 'validate-phase15.js',
    'validate-phase4-performance-continuity.js', 'validate-phase17.js',
    'validate-phase18.js',
    'validate-phase20.js', 'validate-phase21.js', 'validate-phase27.js', 'validate-phase28.js',
    'validate-phase29.js', 'validate-phase31.js', 'validate-phase32.js',
    'validate-video-provider-retry-idempotency.js',
    'adaptation-compiler.test.js']) {
    run(`node scripts/${script}`, 'node', [`scripts/${script}`]);
  }
  run('python scripts/validate-phase1-json-schemas.py', pythonCommand, ['scripts/validate-phase1-json-schemas.py']);
  run('all workflow JSON syntax validation', 'node', ['-e', `const fs=require('fs');for(const f of fs.readdirSync('workflows').filter(x=>x.endsWith('.json'))){JSON.parse(fs.readFileSync('workflows/'+f,'utf8').replace(/^\\uFEFF/,''))}console.log('PASS workflow JSON')`]);
  run('all workflow PostgreSQL statements PREPARE', 'node', ['scripts/validate-workflow-sql.js'], {
    env: {...commandEnv, WORKFLOW_SQL_DATABASE: freshDatabase},
  });
  run('Docker Compose resolved configuration', 'docker', ['compose', '--env-file', '.env.example', 'config', '--quiet']);
  run('Docker service health snapshot', 'docker', ['compose', '--env-file', '.env.example', 'ps', '--format', 'json']);

  const diff = spawnSync('git', ['diff', '--no-ext-diff', '--', '.'], {cwd: root, encoding: 'utf8', windowsHide: true});
  const leaked = (diff.stdout || '').match(/(?:sk-[A-Za-z0-9_-]{20,}|Bearer\s+[A-Za-z0-9._-]{24,}|(?:api[_-]?key|password)\s*[:=]\s*['"][^$<{\s][^'"]{12,})/gi) || [];
  if (leaked.length) throw new Error(`potential secret material in git diff (${leaked.length} match(es))`);
  process.stdout.write('\nPASS secret diff scan: no concrete API key/password pattern found\n');

  const failures = outcomes.filter((item) => item.status !== 0);
  if (failures.length) throw new Error(`${failures.length} acceptance command(s) failed`);
  process.stdout.write(`\nPASS Phase 5 automated acceptance: ${outcomes.length} commands exited 0\n`);
} catch (error) {
  failed = true;
  console.error(`\nPHASE 5 FAILED: ${error.message}`);
} finally {
  if (process.env.PHASE5_KEEP_DATABASES === '1') {
    process.stdout.write('\nKeeping isolated Phase 5 databases for diagnosis as explicitly requested by PHASE5_KEEP_DATABASES=1\n');
  } else {
    for (const database of [freshDatabase, legacyDatabase, compilerDatabase, isolationBaseDatabase]) {
      try { drop(database); } catch (error) { failed = true; console.error(`cleanup failed for ${database}: ${error.message}`); }
    }
  }
}
if (failed) process.exitCode = 1;
