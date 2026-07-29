#!/bin/sh
set -eu

database_name="${1:?temporary database name required}"
created=0
cleanup() {
  if [ "$created" = "1" ]; then
    dropdb -U "$POSTGRES_USER" "$database_name" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

createdb -U "$POSTGRES_USER" "$database_name"
created=1
for migration in \
  init.sql \
  02-script-storyboard.sql \
  03-visual-assets-images.sql \
  04-video-audio.sql \
  05-edit-qc-publish.sql \
  06-narrative-ir-foundation.sql \
  07-adaptation-compiler-audit.sql \
  08-chapter-impact-analysis.sql \
  09-phase5-contract-corrections.sql \
  11-pgcrypto-runtime-prerequisite.sql \
  12-rolling-episode-production.sql \
  13-adaptation-diagnostics-pacing-quality.sql \
  14-multi-candidate-selection.sql \
  14-multi-candidate-selection.sql
do
  psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$database_name" -f "/opt/drama/$migration"
done
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$database_name" \
  -f /opt/drama/14-verify-multi-candidate-selection.sql
