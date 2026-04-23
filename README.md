# Tockr

Tockr is a lightweight time tracking app for Raspberry Pi 4B and small teams.

## Contents

- [Run Locally](#run-locally)
- [Docker Install](#docker-install)
- [Raspberry Pi](#raspberry-pi)
- [Cloudflare Tunnel](#cloudflare-tunnel)
- [systemd](#systemd)
- [Configuration](#configuration)
- [Update](#update)
- [Backup and Restore](#backup-and-restore)
- [Troubleshooting](#troubleshooting)
- [Features](#features)

---

## Run Locally

```sh
go run ./cmd/app
```

Default local seed login:

- `admin@example.com`
- `admin12345`

The local development fallback is intentionally simple. Docker installs
generate and persist the bootstrap password automatically.

---

## Docker Install

Tockr publishes multi-platform images (`linux/amd64`, `linux/arm64`) to
GitHub Container Registry.

### Docker Run

```sh
docker volume create tockr-data
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Retrieve the generated admin password:

```sh
docker exec tockr cat /app/data/.admin_password
```

Open `http://localhost:8029` and log in with `admin@example.com` and the
generated password.

### Docker Compose

Download and start:

```sh
curl -fsSL https://raw.githubusercontent.com/Onellan/tockr/main/docker-compose.prod.yml \
  -o docker-compose.prod.yml
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml exec tockr cat /app/data/.admin_password
```

### Local Development Build

```sh
docker compose up --build
```

Open `http://localhost:8029`.

---

## Raspberry Pi

Requirements: Raspberry Pi 4B or newer, 64-bit Raspberry Pi OS.
Confirm: `uname -m` must return `aarch64`.

### Quick Install

```sh
docker volume create tockr-data
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
docker exec tockr cat /app/data/.admin_password
```

Open `http://<pi-hostname-or-ip>:8029`.

Enable Docker on boot:

```sh
sudo systemctl enable docker containerd
```

### Install Docker

```sh
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
newgrp docker
sudo systemctl enable docker containerd
```

---

## Cloudflare Tunnel

Cloudflare Tunnel routes external HTTPS traffic to Tockr without port
forwarding. `cloudflared` runs on the Pi host and is configured separately.
The Docker Compose stack in `deployment/docker-compose.yml` exposes Tockr on
`127.0.0.1:8080` for cloudflared to reach.

### Requirements

- Cloudflare account with a domain
- Cloudflare Zero Trust (free tier) with a tunnel already created and
  cloudflared installed and running on the host
- In Cloudflare Zero Trust → Access → Tunnels → your tunnel, set the
  public hostname: service type `HTTP`, URL `localhost:8080`

### Deploy

Create the data directory (Tockr runs as UID 65532):

```sh
sudo mkdir -p /srv/tockr/data
sudo chown 65532:65532 /srv/tockr/data
sudo chmod 750 /srv/tockr/data
```

Get the deployment files and create your env file:

```sh
cp deployment/.env.example deployment/.env
nano deployment/.env
```

Minimum `.env` values:

```env
TOCKR_COOKIE_SECURE=true
TOCKR_ADMIN_EMAIL=you@example.com
TOCKR_DEFAULT_TIMEZONE=UTC
TOCKR_DEFAULT_CURRENCY=USD
```

`TOCKR_COOKIE_SECURE=true` is required — Cloudflare Tunnel serves HTTPS to
the browser and the browser requires `Secure=true` on session cookies.

Start:

```sh
docker compose -f deployment/docker-compose.yml --env-file deployment/.env up -d
docker compose -f deployment/docker-compose.yml exec tockr cat /app/data/.admin_password
```

Validate:

```sh
docker compose -f deployment/docker-compose.yml exec tockr \
  wget -qO- http://127.0.0.1:8080/healthz
# Expected: {"status":"ok"}
```

Open `https://tockr.yourdomain.com` and log in.

Sessions are stored in the SQLite database and survive container restarts and
tunnel reconnects. Do not commit `deployment/.env` to version control.

For the full Cloudflare Zero Trust setup walkthrough (tunnel creation, dashboard
configuration, token management) see [docs/cloudflare-tunnel.md](docs/cloudflare-tunnel.md).

---

## systemd

Install the binary at `/opt/tockr/tockr` and static assets at
`/opt/tockr/web/static`.

Create a session secret:

```sh
sudo install -d -m 0750 -o tockr -g tockr /etc/tockr
sudo sh -c 'printf "TOCKR_SESSION_SECRET=%s\n" \
  "$(od -An -tx1 -N32 /dev/urandom | tr -d " \n")" > /etc/tockr/tockr.env'
sudo chown tockr:tockr /etc/tockr/tockr.env
sudo chmod 0600 /etc/tockr/tockr.env
```

`/etc/systemd/system/tockr.service`:

```ini
[Unit]
Description=Tockr time tracking
After=network-online.target
Wants=network-online.target

[Service]
User=tockr
Group=tockr
WorkingDirectory=/opt/tockr
Environment=TOCKR_ADDR=:8080
Environment=TOCKR_DB_PATH=/var/lib/tockr/tockr.db
Environment=TOCKR_DATA_DIR=/var/lib/tockr
Environment=TOCKR_COOKIE_SECURE=false
Environment=TOCKR_TOTP_MODE=disabled
EnvironmentFile=/etc/tockr/tockr.env
ExecStart=/opt/tockr/tockr
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full
ReadWritePaths=/var/lib/tockr

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now tockr
```

Logs:

```sh
journalctl -u tockr -f
```

---

## Configuration

No configuration file is needed for Docker installs. All secrets are generated
automatically on first start and stored in the data volume.

| Variable | Docker default | Local default | Notes |
|---|---|---|---|
| `TOCKR_ADDR` | `:8080` | `:8029` | Change the host port mapping, not this. |
| `TOCKR_DB_PATH` | `/app/data/tockr.db` | `data/tockr.db` | Keep inside the volume. |
| `TOCKR_DATA_DIR` | `/app/data` | `data` | Keep inside the volume. |
| `TOCKR_SESSION_SECRET` | Generated | — | Stored in volume. Changing it logs everyone out. |
| `TOCKR_ADMIN_EMAIL` | `admin@example.com` | `admin@example.com` | Used only on first start. |
| `TOCKR_ADMIN_PASSWORD` | Generated | `admin12345` | Stored in volume. Used only on first start. |
| `TOCKR_DEFAULT_TIMEZONE` | `UTC` | `UTC` | IANA timezone for seeded data. |
| `TOCKR_DEFAULT_CURRENCY` | `USD` | `USD` | ISO 4217 currency for seeded data. |
| `TOCKR_FUTURE_TIME_POLICY` | `end_of_day` | `end_of_day` | `allow` / `deny` / `end_of_day` / `end_of_week` |
| `TOCKR_TOTP_MODE` | `disabled` | `disabled` | `disabled` / `optional` / `required` |
| `TOCKR_COOKIE_SECURE` | `false` | `false` | Set `true` behind HTTPS. |
| `TOCKR_WEBHOOK_MAX_RETRIES` | `5` | `5` | Max webhook delivery attempts. |

Retrieve generated values:

```sh
# Docker run
docker exec tockr cat /app/data/.admin_password

# Docker Compose
docker compose -f docker-compose.prod.yml exec tockr cat /app/data/.admin_password
```

---

## Update

Back up before any update.

### Docker Run

```sh
docker pull ghcr.io/onellan/tockr:latest
docker rm -f tockr
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
curl -fsS http://localhost:8029/healthz
```

### Docker Compose

```sh
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
curl -fsS http://localhost:8029/healthz
```

### Raspberry Pi (Cloudflare Tunnel deploy)

```sh
docker compose -f deployment/docker-compose.yml pull
docker compose -f deployment/docker-compose.yml --env-file deployment/.env up -d
docker compose -f deployment/docker-compose.yml exec tockr \
  wget -qO- http://127.0.0.1:8080/healthz
```

Migrations run automatically on startup. Do not roll back across schema
migrations without a backup from before the update.

### Roll Back to a Previous Tag

```sh
docker pull ghcr.io/onellan/tockr:1.2.3
docker rm -f tockr
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:1.2.3
```

---

## Backup and Restore

### What Must Be Preserved

| File | Impact if lost |
|---|---|
| `tockr.db` | All data — users, projects, timesheets, everything |
| `tockr.db-wal` | Recent transactions — must be backed up with `.db` |
| `tockr.db-shm` | WAL index — must be backed up with `.db` |
| `.session_secret` | All active sessions invalidated; everyone logs out |
| `.admin_password` | Informational; admin user still exists in the database |
| `invoices/` | Invoice downloads broken for all existing invoices |

### Backup — Named Volume

```sh
docker stop tockr
docker run --rm \
  -v tockr-data:/data \
  -v "$(pwd)":/backup \
  alpine tar -czf "/backup/tockr-backup-$(date +%F).tgz" -C /data .
docker start tockr
```

### Backup — Bind Mount (`/srv/tockr/data`)

```sh
docker compose -f deployment/docker-compose.yml stop tockr
mkdir -p /srv/tockr/backups
tar -czf "/srv/tockr/backups/tockr-$(date +%F).tgz" -C /srv/tockr/data .
docker compose -f deployment/docker-compose.yml start tockr
```

### Automated Daily Backup (Bind-Mount Install)

Save as `/usr/local/bin/tockr-backup` (set `COMPOSE_FILE` to your path):

```sh
#!/bin/sh
set -e
COMPOSE_FILE="/path/to/tockr/deployment/docker-compose.yml"
BACKUP_DIR="/srv/tockr/backups"
mkdir -p "$BACKUP_DIR"
docker compose -f "$COMPOSE_FILE" stop tockr
tar -czf "$BACKUP_DIR/tockr-$(date +%F).tgz" -C /srv/tockr/data .
docker compose -f "$COMPOSE_FILE" start tockr
find "$BACKUP_DIR" -name 'tockr-*.tgz' -mtime +30 -delete
```

```sh
sudo chmod +x /usr/local/bin/tockr-backup
(crontab -l 2>/dev/null; echo "0 2 * * * /usr/local/bin/tockr-backup") | crontab -
```

### Restore — Named Volume

```sh
docker stop tockr
docker run --rm \
  -v tockr-data:/data \
  -v "$(pwd)":/backup \
  alpine sh -c "rm -rf /data/* && tar -xzf /backup/tockr-backup-YYYY-MM-DD.tgz -C /data"
docker start tockr
curl -fsS http://localhost:8029/healthz
```

### Restore — Bind Mount

```sh
docker compose -f deployment/docker-compose.yml stop tockr
sudo rm -rf /srv/tockr/data/*
sudo tar -xzf /srv/tockr/backups/tockr-YYYY-MM-DD.tgz -C /srv/tockr/data
sudo chown -R 65532:65532 /srv/tockr/data
docker compose -f deployment/docker-compose.yml start tockr
```

### Restore on a Fresh Machine

1. Install Docker and get deployment files (see [Docker Install](#docker-install) or [Raspberry Pi](#raspberry-pi))
2. Restore the data archive as above
3. Restore or recreate `deployment/.env` — cloudflared token must be re-entered from Cloudflare
4. Start the stack — migrations run automatically, seeding is skipped because users already exist

---

## Troubleshooting

Start every diagnosis with:

```sh
docker ps --filter name=tockr
docker logs tockr --tail 100
curl -fsS http://localhost:8029/healthz
```

### Container Exits Immediately

| Symptom | Fix |
|---|---|
| `bind: address already in use` | Change the host port: `-p 8030:8080` |
| `permission denied` under `/app/data` | Use a named volume or `sudo chown 65532:65532 <dir>` |
| `open database failed` | Confirm the data path is mounted and writable |

### Health Check Stays Unhealthy

```sh
docker logs tockr --tail 100
docker exec tockr wget -qO- http://127.0.0.1:8080/healthz
docker inspect tockr --format='{{.State.Health.Status}}'
```

### Cannot Log In

1. Retrieve the password: `docker exec tockr cat /app/data/.admin_password`
2. Confirm the email matches `TOCKR_ADMIN_EMAIL` (default `admin@example.com`)
3. The bootstrap credentials are used only when the users table is empty — if
   the admin changed their password in the UI, use that instead

Reset a test install:

```sh
docker rm -f tockr
docker volume rm tockr-data
docker volume create tockr-data
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

### Login Fails Through Cloudflare Tunnel

Ensure `TOCKR_COOKIE_SECURE=true` in `deployment/.env`. The browser requires
`Secure=true` on cookies served over HTTPS.

### Image Pull Fails

```sh
docker pull ghcr.io/onellan/tockr:latest
```

If `unauthorized`, log in first: `docker login ghcr.io`. If `manifest unknown`,
check the package page: `https://github.com/Onellan/tockr/pkgs/container/tockr`

### Database Integrity

```sh
docker run --rm -v tockr-data:/data alpine sh -c \
  "apk add --no-cache sqlite && sqlite3 /data/tockr.db 'PRAGMA integrity_check'"
```

Restore from backup if integrity fails.

### Logs for a Bug Report

```sh
docker ps --filter name=tockr
docker inspect tockr --format='{{.State.Status}} {{.State.Health.Status}}'
docker logs tockr --tail 200
```

---

## Features

- Login/logout with secure signed session cookies.
- Optional TOTP two-factor authentication with recovery codes via `TOCKR_TOTP_MODE`.
- CSRF protection for mutating routes.
- Users, customers, projects, activities, tags, rates.
- Tasks under projects for more precise tracking.
- Favorite/pinned timer starts for repeated work.
- Timesheet start/stop and manual time entries.
- Read-only weekly calendar for reviewing scoped time entries.
- Account self-service for display name, timezone, password, and TOTP.
- Workspace switcher for users with access to multiple workspaces.
- Organization-level workspace administration, workspace creation, and workspace member management.
- Project membership and group assignment editing for project managers/admins.
- Bulk group/project membership editors.
- Workspace-scoped project templates for repeatable project setup.
- Future-time policy: `allow`, `deny`, `end_of_day`, `end_of_week`.
- Dashboard, project insights, entity reports, task reports, and saved report definitions.
- Effective-dated billable rates and user cost rates for future profitability reporting.
- Basic invoice creation, metadata API, and invoice download.
- Compact JSON APIs with pagination.
- Signed webhook delivery with persisted retry queue.
- SQLite WAL storage and migration utility for legacy Kimai SQLite exports.
