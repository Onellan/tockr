# Updating, Backup, And Restore

See [README.md](../README.md#update) for update steps and [README.md](../README.md#backup-and-restore) for backup and restore procedures.


## Update A Docker Run Install

Pull the newest image:

```sh
docker pull ghcr.io/onellan/tockr:latest
```

Recreate the container with the same volume:

```sh
docker rm -f tockr
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Validate:

```sh
docker ps --filter name=tockr
curl -fsS http://localhost:8029/healthz
```

## Update A Compose Install

```sh
docker compose -f docker-compose.prod.yml pull
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml ps
curl -fsS http://localhost:8029/healthz
```

## Backup

Stop the app so SQLite is quiet while the archive is created:

```sh
docker stop tockr
docker run --rm \
  -v tockr-data:/data \
  -v "$(pwd)":/backup \
  alpine tar -czf "/backup/tockr-backup-$(date +%F).tgz" -C /data .
docker start tockr
```

The backup archive contains:

- `tockr.db`
- SQLite WAL/SHM files if present
- `.session_secret`
- `.admin_password`
- app-generated files

## Restore

Stop the app:

```sh
docker stop tockr
```

Restore the archive:

```sh
docker run --rm \
  -v tockr-data:/data \
  -v "$(pwd)":/backup \
  alpine sh -c "rm -rf /data/* && tar -xzf /backup/tockr-backup-YYYY-MM-DD.tgz -C /data"
```

Start and validate:

```sh
docker start tockr
curl -fsS http://localhost:8029/healthz
```

Replace `YYYY-MM-DD` with the backup date.

## Roll Back To A Previous Tag

Use a version tag if one was published:

```sh
docker pull ghcr.io/onellan/tockr:1.2.3
docker rm -f tockr
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:1.2.3
```

Do not roll back across database migrations unless you have a backup from
before the update.
