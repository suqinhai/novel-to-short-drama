#!/bin/sh
set -eu
DB="${DRAMA_DB:-short_drama}"
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -tc "SELECT 1 FROM pg_database WHERE datname='$DB'" | grep -q 1 || psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" -c "CREATE DATABASE \"$DB\""
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/init.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/02-script-storyboard.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/03-visual-assets-images.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/04-video-audio.sql
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$DB" -f /opt/drama/05-edit-qc-publish.sql
