# Docker Automation Plan

## Goals

- No manual secret creation for Docker installs.
- No required `.env` file for the default install.
- App starts with a single `docker run` or `docker compose up -d`.
- Generated values persist across restarts and updates.
- Existing deployments that set explicit environment variables keep working.

## Implemented Automation

- Generate `TOCKR_SESSION_SECRET` when unset and store it at
  `/app/data/.session_secret`.
- Generate `TOCKR_ADMIN_PASSWORD` when unset and store it at
  `/app/data/.admin_password`.
- Export generated values before starting the Go app.
- Keep `TOCKR_ADMIN_EMAIL` defaulting to `admin@example.com`.
- Keep explicit `TOCKR_SESSION_SECRET` and `TOCKR_ADMIN_PASSWORD` overrides
  supported for compatibility.
- Document password retrieval with `docker exec`.

## Security Notes

- Generated secrets use `/dev/urandom`.
- Generated files are created with `umask 077`.
- The password is not printed directly in logs.
- The generated password is only used when the database has no users; later UI
  password changes are not overwritten by startup.

## Rollout

- Existing Docker installs with users are unaffected.
- Existing installs with explicit `TOCKR_ADMIN_PASSWORD` continue to use it.
- New installs can omit all secret/password variables.
