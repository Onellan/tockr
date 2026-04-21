# systemd Deployment

Install the binary at `/opt/tockr/tockr` and static assets at `/opt/tockr/web/static`.

Example unit:

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
Environment=TOCKR_SESSION_SECRET=change-this-32-byte-production-secret
Environment=TOCKR_COOKIE_SECURE=false
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

Enable:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now tockr
```

