# systemd Deployment

Install the binary at `/opt/tockr/tockr` and static assets at `/opt/tockr/web/static`.

Create a stable session secret once:

```sh
sudo install -d -m 0750 -o tockr -g tockr /etc/tockr
sudo sh -c 'printf "TOCKR_SESSION_SECRET=%s\n" "$(od -An -tx1 -N32 /dev/urandom | tr -d " \n")" > /etc/tockr/tockr.env'
sudo chown tockr:tockr /etc/tockr/tockr.env
sudo chmod 0600 /etc/tockr/tockr.env
```

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

Enable:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now tockr
```
