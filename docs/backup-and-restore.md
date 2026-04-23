# Backup and Restore

See [README.md](../README.md#backup-and-restore).

including the database, generated secrets, configuration, and invoice files.
Following this procedure ensures nothing is lost across restarts, updates,
and full machine replacements.

---

## What Must Be Backed Up

All critical data lives in one directory on the Pi host:

```
/srv/tockr/data/
├── tockr.db              ← SQLite database — ALL user data
├── tockr.db-wal          ← SQLite WAL file (active transactions)
├── tockr.db-shm          ← SQLite shared memory (WAL index)
├── .session_secret       ← HMAC key for session cookies
├── .admin_password       ← Bootstrap admin password (informational)
└── invoices/             ← Generated invoice HTML files
    ├── INV-2026-001.html
    └── ...
```

Additionally back up:

```
deployment/.env           ← Tunnel token, app config, regional settings
```

What each item means if lost:

| File | Impact if lost |
|---|---|
| `tockr.db` | All data gone — users, projects, timesheets, rates, everything |
| `tockr.db-wal` | Transactions since last checkpoint lost (recent data corruption risk) |
| `tockr.db-shm` | Consistency issue — always back up with the WAL |
| `.session_secret` | All active user sessions invalidated; users must log in again |
| `.admin_password` | Informational only; admin user still exists in the database |
| `invoices/` | Invoice PDF/HTML downloads broken for all existing invoices |
| `deployment/.env` | Must reconfigure `TUNNEL_TOKEN` and regional settings on restore |

---

## Backup Methods

### Method 1 — Offline backup (recommended, guaranteed safe)

Stops Tockr briefly to ensure SQLite is fully consistent, creates a complete
archive of the data directory, then restarts.

```sh
# Change to the directory containing docker-compose.yml
cd /path/to/tockr   # e.g. ~/tockr or /home/pi/tockr

BACKUP_DIR="/srv/tockr/backups"
BACKUP_DATE=$(date +%F)
mkdir -p "$BACKUP_DIR"

# Stop Tockr (requests will fail briefly)
docker compose -f deployment/docker-compose.yml stop tockr

# Archive the entire data directory
tar -czf "$BACKUP_DIR/tockr-data-$BACKUP_DATE.tgz" -C /srv/tockr/data .

# Back up the .env file (exclude secrets if you store .env in version control)
cp deployment/.env "$BACKUP_DIR/tockr-env-$BACKUP_DATE.env"

# Restart Tockr
docker compose -f deployment/docker-compose.yml start tockr

# Verify it's healthy
docker compose -f deployment/docker-compose.yml exec tockr \
  wget -qO- http://127.0.0.1:8080/healthz
```

Expected downtime: 5–15 seconds.

### Method 2 — Live SQLite backup (no downtime)

Uses SQLite's online backup API via a temporary container. The backup is
consistent even while Tockr is writing because SQLite serialises the backup
against the WAL.

```sh
BACKUP_DIR="/srv/tockr/backups"
BACKUP_DATE=$(date +%F)
mkdir -p "$BACKUP_DIR"

# Run sqlite3 in a temporary Alpine container to do an online backup
docker run --rm \
  -v /srv/tockr/data:/data:ro \
  -v "$BACKUP_DIR":/backup \
  --user 65532:65532 \
  alpine sh -c \
  "apk add --no-cache sqlite > /dev/null 2>&1 && \
   sqlite3 /data/tockr.db \".backup '/backup/tockr-$BACKUP_DATE.db'\""

# Back up secrets and invoices
tar -czf "$BACKUP_DIR/tockr-files-$BACKUP_DATE.tgz" \
  -C /srv/tockr/data \
  .session_secret .admin_password invoices/

# Back up env
cp deployment/.env "$BACKUP_DIR/tockr-env-$BACKUP_DATE.env"
```

This backup produces two files:
- `tockr-YYYY-MM-DD.db` — consistent SQLite backup
- `tockr-files-YYYY-MM-DD.tgz` — secrets and invoice files
- `tockr-env-YYYY-MM-DD.env` — environment config

---

## Automated Daily Backup

Create a cron job on the Pi to run a daily backup automatically.

Save this script as `/usr/local/bin/tockr-backup`:

```sh
#!/bin/sh
# Daily Tockr backup — runs as root or a user with docker access
set -e

COMPOSE_FILE="/path/to/tockr/deployment/docker-compose.yml"
BACKUP_DIR="/srv/tockr/backups"
BACKUP_DATE=$(date +%F)
KEEP_DAYS=30

mkdir -p "$BACKUP_DIR"

# Stop briefly for a guaranteed-consistent backup
docker compose -f "$COMPOSE_FILE" stop tockr

tar -czf "$BACKUP_DIR/tockr-data-$BACKUP_DATE.tgz" \
  -C /srv/tockr/data .

docker compose -f "$COMPOSE_FILE" start tockr

# Keep only the last KEEP_DAYS backups
find "$BACKUP_DIR" -name 'tockr-data-*.tgz' -mtime "+$KEEP_DAYS" -delete

echo "Backup complete: $BACKUP_DIR/tockr-data-$BACKUP_DATE.tgz"
```

Make it executable and install the cron job:

```sh
sudo nano /usr/local/bin/tockr-backup
# Paste the script, fix the COMPOSE_FILE path
sudo chmod +x /usr/local/bin/tockr-backup

# Schedule it: 2 AM daily
(crontab -l 2>/dev/null; echo "0 2 * * * /usr/local/bin/tockr-backup >> /var/log/tockr-backup.log 2>&1") | crontab -
```

Verify cron entry:

```sh
crontab -l
```

---

## Verify Backup Integrity

After each backup, verify the archive is valid:

```sh
BACKUP_FILE="/srv/tockr/backups/tockr-data-$(date +%F).tgz"

# List the archive contents
tar -tzf "$BACKUP_FILE" | head -20
# Must include: tockr.db, .session_secret, .admin_password, invoices/ (if any)

# Verify the SQLite database in the archive is not corrupt
docker run --rm \
  -v /srv/tockr/backups:/backup:ro \
  alpine sh -c "apk add --no-cache sqlite > /dev/null && \
    tar -xzf /backup/tockr-data-$(date +%F).tgz -C /tmp && \
    sqlite3 /tmp/tockr.db 'PRAGMA integrity_check;'"
# Must print: ok
```

---

## Off-Site Backup

The `/srv/tockr/backups/` directory should be copied off the Pi regularly.
Options:

```sh
# rsync to a remote machine
rsync -avz /srv/tockr/backups/ user@nas:/backups/tockr/

# Copy to a USB drive mounted at /mnt/usb
rsync -avz /srv/tockr/backups/ /mnt/usb/tockr-backups/
```

---

## Restore on the Same Pi

Use this when recovering from accidental deletion, data corruption, or any
situation where you still have the Pi but need to restore from a backup.

### 1. Stop Tockr

```sh
docker compose -f deployment/docker-compose.yml stop tockr
```

### 2. Clear the current data directory

```sh
sudo rm -rf /srv/tockr/data/*
```

### 3. Restore from the archive

```sh
BACKUP_FILE="/srv/tockr/backups/tockr-data-YYYY-MM-DD.tgz"  # ← set date

sudo tar -xzf "$BACKUP_FILE" -C /srv/tockr/data
sudo chown -R 65532:65532 /srv/tockr/data
```

### 4. Start Tockr

```sh
docker compose -f deployment/docker-compose.yml start tockr
```

### 5. Verify

```sh
docker compose -f deployment/docker-compose.yml exec tockr \
  wget -qO- http://127.0.0.1:8080/healthz
# Expected: {"status":"ok"}
```

Log in at `https://tockr.yourdomain.com` and confirm your data is present.

---

## Restore on a Fresh Raspberry Pi

Use this when setting up a new Pi, replacing a failed Pi, or rebuilding from scratch.

### 1. Prepare the new Pi

Install Docker and Docker Compose (see [docs/raspberry-pi-setup.md](raspberry-pi-setup.md)).

Enable Docker on boot:

```sh
sudo systemctl enable docker containerd
```

### 2. Get the deployment files

```sh
git clone https://github.com/Onellan/tockr.git
cd tockr
```

### 3. Restore `.env`

Copy the backed-up env file:

```sh
cp /path/to/backup/tockr-env-YYYY-MM-DD.env deployment/.env
```

Or create a new `.env` from the example and re-enter your `TUNNEL_TOKEN`.
The tunnel token is the only thing you absolutely cannot recover from the
data backup (it is a Cloudflare credential, not stored in the data directory).

If you have lost the token, create a new one in Cloudflare Zero Trust →
Tunnels → your tunnel → Configure → Token.

### 4. Create the data directory and restore

```sh
sudo mkdir -p /srv/tockr/data
sudo chown 65532:65532 /srv/tockr/data

BACKUP_FILE="/path/to/backup/tockr-data-YYYY-MM-DD.tgz"
sudo tar -xzf "$BACKUP_FILE" -C /srv/tockr/data
sudo chown -R 65532:65532 /srv/tockr/data
```

### 5. Start the stack

```sh
docker compose -f deployment/docker-compose.yml --env-file deployment/.env pull
docker compose -f deployment/docker-compose.yml --env-file deployment/.env up -d
```

### 6. Verify

```sh
docker compose -f deployment/docker-compose.yml ps
# Both services should be running

docker compose -f deployment/docker-compose.yml exec tockr \
  wget -qO- http://127.0.0.1:8080/healthz
# Expected: {"status":"ok"}
```

Open `https://tockr.yourdomain.com` and log in. All user data, sessions
(existing sessions will be valid if `.session_secret` was restored),
timesheets, projects, and invoices should be present.

---

## What Is Not Lost During Normal Operations

The following operations do not lose any data when the data directory is
properly mounted as a bind mount:

| Operation | Data safe? |
|---|---|
| `docker compose restart tockr` | ✓ Yes |
| `docker compose down && up` | ✓ Yes |
| `docker compose pull && up -d` (image update) | ✓ Yes |
| Pi reboot | ✓ Yes |
| Tunnel reconnect | ✓ Yes (no impact on data) |
| New container from same image | ✓ Yes |
| New container from updated image | ✓ Yes (migrations are idempotent) |
| `docker system prune` (volumes only) | ✓ Yes (using bind mount, not named volume) |
| Deleting the Docker named volumes | ✓ Yes (bind mount ignores named volumes) |

The only operations that lose data:

| Operation | Data safe? | How to recover |
|---|---|---|
| `sudo rm -rf /srv/tockr/data` | ✗ No | Restore from backup |
| Corrupted SD card | ✗ No | Restore from off-site backup |
| Accidental `docker volume rm tockr-data` | ✓ Yes | Bind mount is unaffected |

---

## Rollback After a Failed Update

See [docs/updating.md](updating.md) for the full rollback procedure.
