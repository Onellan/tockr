# Raspberry Pi Setup

See [README.md](../README.md#raspberry-pi) and [README.md](../README.md#cloudflare-tunnel).
 External HTTPS access
is provided by a Cloudflare Tunnel that you have already configured separately.
All data persists across container restarts, image updates, and Pi reboots.

## What This Deployment Does

- Runs Tockr in Docker on the Pi, listening on `127.0.0.1:8080`
- Stores all data at `/srv/tockr/data` (bind mount — easy to back up)
- Cloudflare Tunnel (configured separately) routes external HTTPS traffic to port 8080
- Survives reboots, updates, and container recreation without data loss

---

## Prerequisites

### Hardware and OS

- Raspberry Pi 4B (2 GB RAM or more) or Pi 5
- **64-bit Raspberry Pi OS** (Bookworm or Bullseye 64-bit)
  - Run `uname -m` — must return `aarch64`
  - 32-bit OS is not supported by the Tockr image

### Software on the Pi

Install Docker and Docker Compose:

```sh
# Install Docker
curl -fsSL https://get.docker.com | sh

# Add your user to the docker group so you don't need sudo every time
sudo usermod -aG docker $USER

# Apply group change without logging out
newgrp docker

# Verify
docker --version
docker compose version
```

Enable Docker on boot:

```sh
sudo systemctl enable docker
sudo systemctl enable containerd
```

---

## Phase 1 — Prepare the Pi

### 1. Get the deployment files

Clone the repository or download just the deployment directory:

```sh
# Clone full repo
git clone https://github.com/Onellan/tockr.git
cd tockr

# Or download just the deployment files
mkdir -p ~/tockr
curl -fsSL https://raw.githubusercontent.com/Onellan/tockr/main/deployment/docker-compose.yml \
  -o ~/tockr/docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/Onellan/tockr/main/deployment/.env.example \
  -o ~/tockr/.env.example
```

### 2. Create the persistent data directory

The app runs as UID/GID 65532 (non-root). The data directory must be owned by
that user:

```sh
sudo mkdir -p /srv/tockr/data
sudo chown 65532:65532 /srv/tockr/data
sudo chmod 750 /srv/tockr/data
```

This directory will hold the database, session secret, admin password, and
invoice files. **Never delete it.** Back it up regularly.

### 3. Create the environment file

```sh
# If working from the repo
cp deployment/.env.example deployment/.env

# If working from a standalone directory
cp .env.example .env
```

Edit `.env` and set at minimum:

```sh
nano deployment/.env   # or nano .env
```

Required values:

| Variable | What to set |
|---|---|
| `TOCKR_COOKIE_SECURE` | Keep as `true` (Cloudflare Tunnel = HTTPS) |
| `TOCKR_ADMIN_EMAIL` | Your admin email address |
| `TOCKR_DEFAULT_TIMEZONE` | Your timezone (e.g. `Europe/London`) |
| `TOCKR_DEFAULT_CURRENCY` | Your currency code (e.g. `USD`) |

Example completed `.env`:

```env
TOCKR_COOKIE_SECURE=true
TOCKR_ADMIN_EMAIL=you@example.com
TOCKR_DEFAULT_TIMEZONE=America/New_York
TOCKR_DEFAULT_CURRENCY=USD
TOCKR_FUTURE_TIME_POLICY=end_of_day
TOCKR_TOTP_MODE=disabled
```

Do not commit `.env` to version control.

---

## Phase 2 — Start the Stack

### Pull and start

```sh
# From the repo root
docker compose -f deployment/docker-compose.yml --env-file deployment/.env pull
docker compose -f deployment/docker-compose.yml --env-file deployment/.env up -d
```

Docker will:

1. Pull `ghcr.io/onellan/tockr:latest` (arm64 image, ~30 MB)
2. Start `tockr`, wait for the health check to pass, and begin listening on `127.0.0.1:8080`

### Watch startup

```sh
docker compose -f deployment/docker-compose.yml logs -f
```

Expected log output:

```
tockr | {"level":"INFO","msg":"session secret generated","path":"/app/data/.session_secret"}
tockr | {"level":"INFO","msg":"admin bootstrap password generated",...}
tockr | {"level":"INFO","msg":"container bootstrap ready",...}
tockr | {"level":"INFO","msg":"server listening","addr":":8080"}
```

### Retrieve the admin password

```sh
docker compose -f deployment/docker-compose.yml exec tockr cat /app/data/.admin_password
```

Save this password securely. It was generated once on first start and will not
change unless you delete the data directory or create a new one.

---

## Phase 3 — Validate

### Local health check (on the Pi)

```sh
docker compose -f deployment/docker-compose.yml ps
# Expected: tockr (healthy)

docker compose -f deployment/docker-compose.yml exec tockr \
  wget -qO- http://127.0.0.1:8080/healthz
# Expected: {"status":"ok"}
```

### Tunnel access

Open a browser on any device and go to:

```
https://tockr.yourdomain.com
```

You should see the Tockr login page.

Log in with:

- Email: the value of `TOCKR_ADMIN_EMAIL` in your `.env`
- Password: the value from `/srv/tockr/data/.admin_password`

### Verify data persists

```sh
# Create a test project or timesheet entry in the app, then:
docker compose -f deployment/docker-compose.yml restart tockr

# Wait for health check to pass (~20 seconds), then refresh the browser.
# The test data must still be there.
```

---

## Phase 4 — Enable Automatic Start on Boot

Docker is already configured to restart containers with `restart: unless-stopped`.
Ensure Docker itself starts on boot:

```sh
sudo systemctl enable docker
sudo systemctl is-enabled docker   # should print "enabled"
```

Test a full reboot:

```sh
sudo reboot
# Wait ~60 seconds, then:
docker compose -f deployment/docker-compose.yml ps
```

Tockr should be running and healthy.

---

## Daily Operations

### Check status

```sh
docker compose -f deployment/docker-compose.yml ps
```

### View logs

```sh
# Live logs
docker compose -f deployment/docker-compose.yml logs -f

# Last 50 lines
docker compose -f deployment/docker-compose.yml logs --tail 50 tockr
```

### Health check

```sh
docker compose -f deployment/docker-compose.yml exec tockr \
  wget -qO- http://127.0.0.1:8080/healthz
```

### Start / stop / restart

```sh
docker compose -f deployment/docker-compose.yml restart tockr

docker compose -f deployment/docker-compose.yml down

docker compose -f deployment/docker-compose.yml --env-file deployment/.env up -d
```

### Get admin password (if lost)

```sh
cat /srv/tockr/data/.admin_password
```

---

## Updating Tockr

See **[docs/updating.md](updating.md)** for the full update procedure with
pre-update backup steps and rollback instructions.

Quick update (no data loss — data is on a bind mount):

```sh
# Back up first (see docs/backup-and-restore.md)
docker compose -f deployment/docker-compose.yml pull
docker compose -f deployment/docker-compose.yml --env-file deployment/.env up -d
docker compose -f deployment/docker-compose.yml exec tockr wget -qO- http://127.0.0.1:8080/healthz
```

---

## Backup and Restore

See **[docs/backup-and-restore.md](backup-and-restore.md)** for the complete
backup and restore procedures, including automated daily backups.

---

## Troubleshooting

### App does not respond

```sh
docker compose -f deployment/docker-compose.yml ps tockr
docker compose -f deployment/docker-compose.yml logs tockr
```

### Login does not work through tunnel

Symptom: Login redirects back to login page with no error.

Cause: `TOCKR_COOKIE_SECURE=false` with a browser that requires `Secure` cookies
over HTTPS, or `TOCKR_COOKIE_SECURE=true` but accessing over plain HTTP.

Fix: Ensure `TOCKR_COOKIE_SECURE=true` in `.env` and always access Tockr via
`https://tockr.yourdomain.com` (HTTPS), never via `http://pi-ip:8029`.

### Permission denied on /srv/tockr/data

```sh
sudo chown -R 65532:65532 /srv/tockr/data
sudo chmod 750 /srv/tockr/data
```

### Pi does not have enough memory

Tockr uses approximately 30–50 MB of RAM. Add swap if running a 1 GB Pi:

```sh
sudo dphys-swapfile swapoff
sudo sed -i 's/CONF_SWAPSIZE=.*/CONF_SWAPSIZE=512/' /etc/dphys-swapfile
sudo dphys-swapfile setup && sudo dphys-swapfile swapon
```
