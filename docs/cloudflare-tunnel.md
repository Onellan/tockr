# Cloudflare Tunnel Setup for Tockr

This guide walks through creating and configuring a Cloudflare Tunnel that
exposes Tockr securely to the internet without opening any router ports.

---

## How It Works

```
Browser (HTTPS)
    │
    ▼
Cloudflare Edge  (TLS terminated here)
    │  encrypted tunnel
    ▼
cloudflared daemon  (running on the Pi host)
    │  plain HTTP inside Docker network
    ▼
Tockr container  (http://tockr:8080)
```

The connection between the Pi and Cloudflare is an outbound connection
initiated by cloudflared. Your router does not need any inbound port forwarding.
TLS is terminated at Cloudflare. Traffic between cloudflared and Tockr is plain
HTTP within the Docker bridge network (not public).

---

## Prerequisites

- A Cloudflare account (free)
- A domain with DNS managed by Cloudflare (the orange cloud must be on in DNS settings)
- Cloudflare Zero Trust enabled (free, at [one.dash.cloudflare.com](https://one.dash.cloudflare.com))

If you do not have a domain: Cloudflare provides free `*.trycloudflare.com`
subdomains for quick testing, but for production you need your own domain.

---

## Step 1 — Enable Cloudflare Zero Trust

1. Log into [one.dash.cloudflare.com](https://one.dash.cloudflare.com)
2. If prompted, complete the Zero Trust onboarding
3. Select or create a Zero Trust organization name
4. Choose the **Free** plan

---

## Step 2 — Create the Tunnel

1. In the Zero Trust dashboard, go to **Access → Tunnels**
2. Click **Create a tunnel**
3. Choose **Cloudflared** as the connector type
4. Give the tunnel a name: `tockr` (or any name you prefer)
5. Click **Save tunnel**

---

## Step 3 — Get the Tunnel Token

After saving the tunnel, Cloudflare shows a token and installation instructions.

1. On the **Install connector** screen, select **Docker** as the environment
2. Cloudflare shows a `docker run` command containing `--token <TOKEN>`
3. Copy the full token value (the long string after `--token`)

Example token format:

```
eyJhIjoiMTIzNDU2Nzg5...very_long_base64_string...
```

4. Paste this token into `deployment/.env` as the `TUNNEL_TOKEN` value
5. Click **Next** to proceed

---

## Step 4 — Configure the Public Hostname

This step maps `tockr.yourdomain.com` to the local Tockr service.

1. On the **Route tunnel** screen, click **Add a public hostname**
2. Fill in:

   | Field | Value |
   |---|---|
   | Subdomain | `tockr` (or whatever you want, e.g. `time`) |
   | Domain | Your domain (e.g. `example.com`) |
   | Service type | `HTTP` |
   | URL | `localhost:8080` |

   The full public URL will be `https://tockr.example.com`.
   `localhost:8080` is the port that Tockr's Docker container exposes on the
   Pi host. cloudflared running on the host reaches Tockr at this address.

3. Do **not** enable TLS for the origin — the origin is plain HTTP and that
   is correct. TLS termination happens at Cloudflare.

4. Click **Save hostname**

5. Click **Complete setup**

Cloudflare automatically creates a DNS CNAME record pointing your chosen
subdomain to the Cloudflare Tunnel infrastructure.

---

## Step 5 — Verify DNS

```sh
# From any machine
nslookup tockr.yourdomain.com
# Should return a Cloudflare IP or CNAME
```

The DNS record is created automatically. It may take a few minutes to propagate.

---

## Step 6 — Start cloudflared

cloudflared runs on the Pi host (not as part of the Docker Compose stack).
Start or restart your cloudflared service, then verify it is connected:

```sh
# If running as a systemd service
sudo systemctl restart cloudflared
sudo journalctl -u cloudflared -n 30
```

Expected log lines:

```
INF Registered tunnel connection connIndex=0 ...
INF Connection ... registered connIndex=0
```

---

## Step 7 — Validate End-to-End

1. Open a browser: `https://tockr.yourdomain.com`
2. You should see the Tockr login page served over HTTPS
3. Log in with your admin credentials
4. The app should work fully — pages load, data saves, session persists

---

## Cookie and Session Behaviour Through the Tunnel

Tockr session cookies are set with:

- `HttpOnly: true` — not accessible from JavaScript
- `SameSite: Lax` — sent on cross-site top-level navigations
- `Secure: <TOCKR_COOKIE_SECURE>` — must be `true` for HTTPS

**`TOCKR_COOKIE_SECURE=true` is required for the Cloudflare Tunnel deployment.**

When a user logs in through `https://tockr.yourdomain.com`:

1. Tockr sets the `tockr_session` cookie with `Secure=true`
2. The browser stores it and sends it on all subsequent requests
3. The cookie travels over HTTPS from the browser to Cloudflare
4. Cloudflare forwards the request (with cookie) to cloudflared → tockr
5. Sessions are stored in the SQLite database, not in memory

Result: Sessions survive container restarts, image updates, and tunnel reconnects
because the session store is the persistent SQLite database.

---

## CSRF Protection Through the Tunnel

Tockr uses token-based CSRF protection. CSRF tokens are stored in the session
(in the database) and submitted with each POST request either as a form field
(`csrf`) or as an HTTP header (`X-CSRF-Token`).

Since all form submissions POST to the same origin (`tockr.yourdomain.com`),
CSRF protection works correctly through the tunnel without any configuration.

---

## IP Address Forwarding

Cloudflare sets `X-Forwarded-For` and `CF-Connecting-IP` headers on requests
forwarded through the tunnel. Tockr's HTTP server includes `middleware.RealIP`
(from chi), which reads `X-Real-IP` and `X-Forwarded-For` to populate the
correct client IP in request context and logs.

No additional configuration is needed.

---

## Reconnect Behaviour

cloudflared maintains multiple connections to Cloudflare's edge for reliability.
If one connection drops, others continue serving traffic. cloudflared
automatically reconnects dropped connections.

On Pi reboot:

1. Docker starts (systemd service, enabled on boot)
2. `tockr` container starts, passes health check
3. `cloudflared` container starts and reconnects to Cloudflare
4. Tunnel is serving traffic again, typically within 20–30 seconds of boot

Typical reconnect time after a clean reboot: 15–30 seconds.
Typical reconnect time after cloudflared crashes and restarts: 2–5 seconds.

---

## Tunnel Token Rotation

If your tunnel token is compromised or you need to rotate it:

1. In Cloudflare Zero Trust → Tunnels, select your tunnel
2. Go to **Configure → Connectors** → revoke or rotate the token
3. Update `TUNNEL_TOKEN` in `deployment/.env`
4. Restart cloudflared:

```sh
docker compose -f deployment/docker-compose.yml restart cloudflared
```

This does not affect Tockr or any user data.

---

## Alternative: Credentials File Approach

The token approach is recommended for most deployments. It requires no local
config files and is simpler to manage.

The credentials file approach is available for environments where you want the
tunnel configuration in version control (without the credentials JSON, which
must remain secret). See `deployment/cloudflared/config.yml` for the template
and instructions.

---

## Troubleshooting

### Tunnel shows "Inactive" in Cloudflare dashboard

cloudflared is not connected. Check:

```sh
docker compose -f deployment/docker-compose.yml logs cloudflared
docker compose -f deployment/docker-compose.yml ps
```

If cloudflared is not running, check for errors in logs. Common causes:

- Invalid `TUNNEL_TOKEN` — re-copy from Cloudflare dashboard
- Pi clock is wrong — run `sudo timedatectl set-ntp true`
- Network not available — verify Pi has internet access

### App loads but login fails

The browser is receiving cookies but not sending them back. Most likely cause:
`TOCKR_COOKIE_SECURE` mismatch.

Verify `TOCKR_COOKIE_SECURE=true` in `deployment/.env` and that you are
accessing via `https://` (not `http://`).

### Error 502 Bad Gateway from Cloudflare

cloudflared is connected but cannot reach Tockr. Check:

```sh
docker compose -f deployment/docker-compose.yml ps tockr
# Must show (healthy)

docker compose -f deployment/docker-compose.yml exec tockr \
  wget -qO- http://127.0.0.1:8080/healthz
```

If tockr is unhealthy or not running, check its logs:

```sh
docker compose -f deployment/docker-compose.yml logs tockr
```

### Cloudflare service URL is wrong

If you accidentally set the service URL to `localhost:8080` or `127.0.0.1:8080`
instead of `tockr:8080`, cloudflared cannot reach Tockr because `localhost`
inside the cloudflared container refers to cloudflared itself.

Fix in Cloudflare Zero Trust → Tunnels → your tunnel → Public Hostnames →
edit the hostname and correct the service URL to `tockr:8080`.
