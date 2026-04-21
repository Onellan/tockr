# Raspberry Pi 4B Notes

Recommended:

- Raspberry Pi OS 64-bit.
- Use the native binary or Docker image.
- Store `/var/lib/tockr` on reliable storage.
- Keep `TOCKR_ADDR` bound to localhost when using a reverse proxy.
- Use SQLite WAL backups, not raw copying of a live database file.

## Backup

```sh
sqlite3 /var/lib/tockr/tockr.db ".backup '/var/backups/tockr-$(date +%F).db'"
tar -czf "/var/backups/tockr-files-$(date +%F).tgz" /var/lib/tockr/invoices
```

## Restore

Stop the service, replace the database and invoice files, then start again:

```sh
sudo systemctl stop tockr
sudo cp /var/backups/tockr-YYYY-MM-DD.db /var/lib/tockr/tockr.db
sudo systemctl start tockr
```

## Logging

Logs are JSON on stdout/stderr. With systemd:

```sh
journalctl -u tockr -f
```

