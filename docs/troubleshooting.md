# Troubleshooting

See [README.md](../README.md#troubleshooting).


```sh
docker ps --filter name=tockr
docker logs tockr --tail 100
curl -fsS http://localhost:8029/healthz
```

For Compose installs, replace `docker logs tockr` with:

```sh
docker compose -f docker-compose.prod.yml logs --tail 100 tockr
```

## Container Exits Immediately

Check logs:

```sh
docker logs tockr
```

Common causes:

| Symptom | Fix |
|---|---|
| `bind: address already in use` | Change the host port, for example `-p 8030:8080`. |
| `permission denied` under `/app/data` | Use the named volume `tockr-data`, or make your bind mount writable by UID/GID `65532`. |
| `open database failed` | Confirm the data path is mounted and writable. |

## Health Check Stays Unhealthy

Wait at least 40 seconds, then run:

```sh
docker logs tockr --tail 100
docker exec tockr wget -qO- http://127.0.0.1:8080/healthz
docker inspect tockr --format='{{.State.Health.Status}}'
```

If the manual health command works but Docker still says `starting`, wait for
the next health interval. If it says `unhealthy`, the logs should show the
startup error.

## App Is Not Reachable In The Browser

Check:

- The container is running: `docker ps --filter name=tockr`.
- The URL uses the host port: `http://localhost:8029`.
- On a Raspberry Pi, use `http://<pi-hostname-or-ip>:8029`.
- A firewall is not blocking the port.

If port `8029` is already used, recreate the container with another host port:

```sh
docker rm -f tockr
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8030:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Then open `http://localhost:8030`.

## Find The Generated Admin Password

Docker run:

```sh
docker exec tockr cat /app/data/.admin_password
```

Compose:

```sh
docker compose -f docker-compose.prod.yml exec tockr cat /app/data/.admin_password
```

Important: this file is the first-run bootstrap password. If the admin password
was changed in the UI, use the current UI password instead.

## Cannot Log In

1. Confirm the email is `admin@example.com` unless you set
   `TOCKR_ADMIN_EMAIL` before first start.
2. Retrieve the generated password from `/app/data/.admin_password`.
3. Check whether the database already existed. The bootstrap email/password are
   only used when the users table is empty.
4. If this is a disposable test install, reset it:

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

## Volume Permissions

Named Docker volumes are recommended:

```sh
docker volume create tockr-data
```

If you bind-mount a host directory, make it writable by UID/GID `65532`:

```sh
sudo mkdir -p /opt/tockr-data
sudo chown 65532:65532 /opt/tockr-data
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v /opt/tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

## Image Pull Fails

Try:

```sh
docker pull ghcr.io/onellan/tockr:latest
```

If you see `manifest unknown`, the image has not been published for that tag.
Check the Actions page and GHCR package page:

- `https://github.com/Onellan/tockr/actions`
- `https://github.com/Onellan/tockr/pkgs/container/tockr`

If you see `unauthorized`, the package may still be private. Public GHCR images
can be pulled without login. Private packages require:

```sh
docker login ghcr.io
```

## Database Or Migration Problems

Check disk space:

```sh
df -h
```

Check SQLite integrity:

```sh
docker run --rm -v tockr-data:/data alpine sh -c \
  "apk add --no-cache sqlite && sqlite3 /data/tockr.db 'PRAGMA integrity_check'"
```

Restore from backup if integrity fails. See [updating.md](updating.md).

## Stale Container Or Image

Update safely:

```sh
docker pull ghcr.io/onellan/tockr:latest
docker rm -f tockr
docker run -d --name tockr \
  --restart unless-stopped \
  -p 8029:8080 \
  -v tockr-data:/app/data \
  ghcr.io/onellan/tockr:latest
```

Confirm the image reference:

```sh
docker inspect tockr --format='{{.Config.Image}}'
```

## Logs To Include In A Bug Report

```sh
docker ps --filter name=tockr
docker inspect tockr --format='{{.State.Status}} {{.State.Health.Status}}'
docker logs tockr --tail 200
```
