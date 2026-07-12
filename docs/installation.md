English | [简体中文](installation.zh-CN.md)

# Installation Guide

This document explains how to install, update, and uninstall SublinkPro.

---

## 📦 Run with Docker Compose, recommended

> [!TIP]
> **Docker Compose is recommended** because it makes configuration, upgrades, and maintenance easier.

> [!IMPORTANT]
> `db/`, `template/`, and `logs/` are runtime persistence directories. Keep them during upgrades and migrations.

> [!NOTE]
> This fork publishes the custom Docker image as `ghcr.io/chunlion/sublinkpro:custom` for `linux/amd64`.

Create `docker-compose.yml`:

```yaml
services:
  sublinkpro:
    image: ghcr.io/chunlion/sublinkpro:custom # Custom branch image
    container_name: sublinkpro
    ports:
      - "8000:8000"
    volumes:
      - "./db:/app/db"
      - "./template:/app/template"
      - "./logs:/app/logs"
    restart: unless-stopped
```

Optional Sub-Store sidecar for expanded subscription output formats:

```yaml
services:
  sublinkpro:
    image: ghcr.io/chunlion/sublinkpro:custom
    container_name: sublinkpro
    ports:
      - "8000:8000"
    volumes:
      - "./db:/app/db"
      - "./template:/app/template"
      - "./logs:/app/logs"
    restart: unless-stopped

  substore:
    image: xream/sub-store
    container_name: substore
    environment:
      - SUB_STORE_BACKEND_API_PORT=3000
      - SUB_STORE_BODY_JSON_LIMIT=10mb
    restart: unless-stopped
```

Keep the Sub-Store service inside the Compose network and do not publish its port unless you protect it separately. After both containers start, sign in and open **Application Settings -> Sub-Store** to enable the sidecar, set the base URL such as `http://substore:3000`, choose allowed output targets, and test the connection. Sub-Store integration is managed from that page, not through environment variables.

To expose the service through Cloudflare Tunnel, start the instance first, then open **Application Settings -> Cloudflare Tunnel**, enter the token, and start it. When auto connect is enabled, the Tunnel connects when the service starts. See [Cloudflare Tunnel remote access](features/cloudflare-tunnel.md) for the full flow.

The custom Docker image includes `cloudflared`. Non Docker deployments need `cloudflared` installed first according to Cloudflare's official documentation.

Start the service:

```bash
docker-compose up -d
```

---

## 🐳 Run with Docker

<details>
<summary><b>Custom branch image</b></summary>

```bash
docker run --name sublinkpro -p 8000:8000 \
  -v $PWD/db:/app/db \
  -v $PWD/template:/app/template \
  -v $PWD/logs:/app/logs \
  -d ghcr.io/chunlion/sublinkpro:custom
```

</details>

---

## 📝 Native Install or Update Script

> [!NOTE]
> This fork currently does not publish GitHub binary release assets. Do not use the one line install/update script for custom branch deployments. Use Docker Compose or Docker with `ghcr.io/chunlion/sublinkpro:custom`.
>
> The fork script no longer downloads upstream release assets. It only works after this fork publishes the expected `sublinkPro-linux-*` release files.

---

## 🗑️ One Line Uninstall Script

```bash
sh -c "$(wget -qO- https://raw.githubusercontent.com/Chunlion/sublinkPro/refs/heads/custom/uninstall.sh)"
```

> [!NOTE]
> The uninstall script asks whether to keep the data directories, including db, logs, and template. Keeping them allows later reinstalls to restore data.

---

## 🔄 Project Updates

### 📝 Native script update

Native script updates are not a supported custom branch update path until this fork publishes binary release assets.

### 📦 Manual Docker Compose update

```bash
# Enter the directory containing docker-compose.yml
cd /path/to/your/sublinkpro

# Pull the latest image
docker-compose pull

# Recreate and start the container
docker-compose up -d

# Optional: clean old images
docker image prune -f
```

### 🐳 Manual Docker update

```bash
# Stop and remove the old container
docker stop sublinkpro
docker rm sublinkpro

# Pull the latest image
docker pull ghcr.io/chunlion/sublinkpro:custom

# Start the container again with the same parameters used during installation
docker run --name sublinkpro -p 8000:8000 \
  -v $PWD/db:/app/db \
  -v $PWD/template:/app/template \
  -v $PWD/logs:/app/logs \
  -d ghcr.io/chunlion/sublinkpro:custom

# Optional: clean old images
docker image prune -f
```

---

## 🤖 Automatic Updates with Watchtower

Watchtower automatically updates Docker containers. It is useful if you want the project to stay current.

### Option 1: Run Watchtower separately

```bash
docker run -d \
  --name watchtower \
  -v /var/run/docker.sock:/var/run/docker.sock \
  containrrr/watchtower \
  --cleanup \
  --interval 86400 \
  sublinkpro
```

> [!NOTE]
> - `--cleanup`: remove old images after updates
> - `--interval 86400`: check for updates every 24 hours, in seconds
> - The final `sublinkpro` is the container name to monitor. If omitted, all containers are monitored.

### Option 2: Add Watchtower to Docker Compose

Add the Watchtower service to your `docker-compose.yml`:

```yaml
services:
  sublinkpro:
    image: ghcr.io/chunlion/sublinkpro:custom
    container_name: sublinkpro
    ports:
      - "8000:8000"
    volumes:
      - "./db:/app/db"
      - "./template:/app/template"
      - "./logs:/app/logs"
    restart: unless-stopped

  watchtower:
    image: containrrr/watchtower
    container_name: watchtower
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=Asia/Shanghai
      - WATCHTOWER_CLEANUP=true
      - WATCHTOWER_POLL_INTERVAL=86400
    restart: unless-stopped
    command: sublinkpro  # Only monitor the sublinkpro container
```

> [!TIP]
> **Advanced Watchtower configuration**:
> - Set `WATCHTOWER_NOTIFICATIONS` to configure update notifications, including email, Slack, Gotify, and others
> - See the [official Watchtower documentation](https://containrrr.dev/watchtower/) for more settings
