#!/bin/sh
# Tockr container entrypoint.
#
# Secrets that are safe to automate are generated once and stored in the data
# volume. Explicit environment variables still take precedence for existing
# installs and scripted deployments.
set -e

DATA_DIR="${TOCKR_DATA_DIR:-/app/data}"
DB_PATH="${TOCKR_DB_PATH:-${DATA_DIR}/tockr.db}"
SECRET_FILE="${DATA_DIR}/.session_secret"
ADMIN_PASSWORD_FILE="${DATA_DIR}/.admin_password"

mkdir -p "$DATA_DIR"
umask 077

if [ -z "$TOCKR_SESSION_SECRET" ]; then
  if [ ! -f "$SECRET_FILE" ]; then
    od -An -tx1 -N32 /dev/urandom | tr -d ' \n' > "$SECRET_FILE"
    printf '{"level":"INFO","msg":"session secret generated","path":"%s"}\n' \
      "$SECRET_FILE" >&2
  fi
  TOCKR_SESSION_SECRET=$(cat "$SECRET_FILE")
  export TOCKR_SESSION_SECRET
fi

if [ -z "$TOCKR_ADMIN_PASSWORD" ]; then
  if [ ! -f "$ADMIN_PASSWORD_FILE" ]; then
    od -An -tx1 -N18 /dev/urandom | tr -d ' \n' > "$ADMIN_PASSWORD_FILE"
    printf '{"level":"INFO","msg":"admin bootstrap password generated","path":"%s","email":"%s","note":"used only when the database has no users"}\n' \
      "$ADMIN_PASSWORD_FILE" "${TOCKR_ADMIN_EMAIL:-admin@example.com}" >&2
  fi
  TOCKR_ADMIN_PASSWORD=$(cat "$ADMIN_PASSWORD_FILE")
  export TOCKR_ADMIN_PASSWORD
fi

printf '{"level":"INFO","msg":"container bootstrap ready","data_dir":"%s","db_path":"%s"}\n' \
  "$DATA_DIR" "$DB_PATH" >&2

exec /app/tockr "$@"
