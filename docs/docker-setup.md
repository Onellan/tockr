# Docker Setup Guide

This is the canonical Docker install path for Tockr. It starts from a machine
with Docker installed and ends with a running app, persistent data, and a
generated admin password. You do not need to create secrets or edit an
environment file.

## What Docker Runs

| Item | Value |
|---|---|
| Image | `ghcr.io/onellan/tockr:latest` |
| Container port | `8080` |
| Default host port | `8029` |
| Database | SQLite at `/app/data/tockr.db` |
| Persistent storage | Docker volume `tockr-data` |
| Session secret | Generated at `/app/data/.session_secret` |
| Admin password | Generated at `/app/data/.admin_password` |
| Health endpoint | `http://localhost:8029/healthz` |

On first start the container creates the data directory, generates missing
secrets, runs SQLite migrations, and seeds the first admin user if the database
does not already contain users.

## Prerequisites

- Docker 24 or newer.
- Docker Compose v2 if you prefer Compose.
- Internet access to pull from GitHub Container Registry.
- Host port `8029` available, or another free port you choose.
- Raspberry Pi users should use 64-bit Raspberry Pi OS. The published image
  supports `linux/arm64` and `linux/amd64`.

Check Docker:

```sh
docker --version
docker compose version
```

## Install With Docker Run

Create the persistent volume:

```sh
docker volume create tockr-data
```

Start Tockr:

```sh
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Wait for the health check:

```sh
docker ps --filter name=tockr
```

Expected status after startup:

```text
Up ... (healthy)
```

Retrieve the generated admin password:

```sh
docker exec tockr cat /app/data/.admin_password
```

Open the app:

```text
http://localhost:8029
```

Log in with:

- Email: `admin@example.com`
- Password: the value printed from `/app/data/.admin_password`

## Install With Docker Compose

Download the production Compose file:

```sh
curl -fsSL https://raw.githubusercontent.com/Onellan/tockr/main/docker-compose.prod.yml \
  -o docker-compose.prod.yml
```

Start Tockr:

```sh
docker compose -f docker-compose.prod.yml up -d
```

Retrieve the generated admin password:

```sh
docker compose -f docker-compose.prod.yml exec tockr cat /app/data/.admin_password
```

Open:

```text
http://localhost:8029
```

## Configuration

No configuration file is required. The defaults are production-aware for a
single-container self-hosted install:

| Variable | Docker default | User action |
|---|---|---|
| `TOCKR_ADDR` | `:8080` | Leave unchanged inside Docker. |
| `TOCKR_DB_PATH` | `/app/data/tockr.db` | Leave in the volume. |
| `TOCKR_DATA_DIR` | `/app/data` | Leave mounted to persistent storage. |
| `TOCKR_SESSION_SECRET` | Generated | Do not set unless rotating or sharing across replicas. |
| `TOCKR_ADMIN_EMAIL` | `admin@example.com` | Optional; set before first start if desired. |
| `TOCKR_ADMIN_PASSWORD` | Generated | Optional; set before first start only if you need a chosen value. |
| `TOCKR_DEFAULT_TIMEZONE` | `UTC` | Optional; set before first start for local defaults. |
| `TOCKR_DEFAULT_CURRENCY` | `USD` | Optional; set before first start for local defaults. |
| `TOCKR_COOKIE_SECURE` | `false` | Set `true` when served through HTTPS. |

See [configuration.md](configuration.md) for the full reference.

## Validate The Install

Check the container:

```sh
docker ps --filter name=tockr
```

Check logs:

```sh
docker logs tockr --tail 50
```

Healthy first-start logs include:

```json
{"level":"INFO","msg":"session secret generated","path":"/app/data/.session_secret"}
{"level":"INFO","msg":"admin bootstrap password generated","path":"/app/data/.admin_password","email":"admin@example.com","note":"used only when the database has no users"}
{"level":"INFO","msg":"container bootstrap ready","data_dir":"/app/data","db_path":"/app/data/tockr.db"}
{"level":"INFO","msg":"server listening","addr":":8080"}
```

Check the health endpoint:

```sh
curl -fsS http://localhost:8029/healthz
```

Expected:

```json
{"status":"ok"}
```

Confirm data persistence:

```sh
docker restart tockr
curl -fsS http://localhost:8029/healthz
docker exec tockr test -s /app/data/tockr.db
docker exec tockr test -s /app/data/.session_secret
docker exec tockr test -s /app/data/.admin_password
```

## Day-2 Operations

Stop:

```sh
docker stop tockr
```

Start:

```sh
docker start tockr
```

Restart:

```sh
docker restart tockr
```

Follow logs:

```sh
docker logs -f tockr
```

Update:

```sh
docker pull ghcr.io/onellan/tockr:latest
docker rm -f tockr
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Compose update:

```sh
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
```

Backup:

```sh
docker stop tockr
docker run --rm \
  -v tockr-data:/data \
  -v "$(pwd)":/backup \
  alpine tar -czf "/backup/tockr-backup-$(date +%F).tgz" -C /data .
docker start tockr
```

Restore:

```sh
docker stop tockr
docker run --rm \
  -v tockr-data:/data \
  -v "$(pwd)":/backup \
  alpine sh -c "rm -rf /data/* && tar -xzf /backup/tockr-backup-YYYY-MM-DD.tgz -C /data"
docker start tockr
```

Replace `YYYY-MM-DD` with the backup date.

## Troubleshooting

Use [troubleshooting.md](troubleshooting.md) for detailed fixes. The shortest
diagnostic loop is:

```sh
docker ps --filter name=tockr
docker logs tockr --tail 100
curl -fsS http://localhost:8029/healthz
```

Common fixes:

- Port conflict: change `-p 8029:8080` to another host port, such as
  `-p 8030:8080`.
- Image pull fails: confirm the GitHub package exists and is public, then run
  `docker pull ghcr.io/onellan/tockr:latest`.
- Lost password: read it again with
  `docker exec tockr cat /app/data/.admin_password`. If you changed the
  password in the UI later, use the current UI password instead.
- Bind mount permission issue: prefer the named volume `tockr-data`; for host
  directories, make the directory writable by UID/GID `65532`.
