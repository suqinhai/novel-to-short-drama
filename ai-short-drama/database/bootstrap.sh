#!/bin/sh
set -eu
DB="${DRAMA_DB:-short_drama}"
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -tc "SELECT 1 FROM pg_database WHERE datname='$DB'" | grep -q 1 || psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -c "CREATE DATABASE \"$DB\""
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/init.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/02-script-storyboard.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/03-visual-assets-images.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/04-video-audio.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/05-edit-qc-publish.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/06-narrative-ir-foundation.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/07-adaptation-compiler-audit.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/08-chapter-impact-analysis.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/09-phase5-contract-corrections.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/10-backfill-singular-script-dialogues.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/11-pgcrypto-runtime-prerequisite.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/12-rolling-episode-production.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/13-adaptation-diagnostics-pacing-quality.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/14-multi-candidate-selection.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/15-local-editing-workbench.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/16-performance-continuity-visual-qc.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/17-post-production-creative-workbench.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/18-effective-input-resolver.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/19-unified-versioned-change-entry.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/20-ir-merge-closure.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/21-pluggable-candidate-providers.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/22-restore-effective-input-stage-requirements.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/23-versioned-production-routing.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/24-authoritative-production-inputs.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/25-season-planning-workbench.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/26-atomic-multi-shot-editor.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/27-lightweight-nle.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/28-cross-layer-quality-gate.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/29-prompt-lab-professional-export.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/30-step-0-7-p0-p1-closure.sql
