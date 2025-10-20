# SlowDrip Miner

Low-latency **edge node** for the SlowDrip network.
Ships with **MediaMTX** for real-time streaming (RTMP/RTSP/**WebRTC WHIP/WHEP**/HLS) and a lightweight **Go miner** that exposes health/metrics and stubs for Proof-of-Presence (PoP) and Proof-of-Service (PoS). Designed for a bottom-up bring-up: streaming first, then add presence/service, receipts, and wallet logic as modules.

---

## ✨ Features (v0 – bootstrap)

* **MediaMTX** prewired: RTMP ingest, RTSP, **WebRTC (WHIP/WHEP)**, HLS (LL-HLS capable).
* **JWT auth** (via JWKS URL) already configured in `mediamtx.yml`.
* **Miner (Go)** process with:

  * `/healthz`, `/readyz`, `/metrics` (Prometheus)
  * MediaMTX API watcher (paths/sessions) – *stub events now*
  * PoP/PoS agents – *stub loops now*
* Containers via **docker-compose**. Production-ready Dockerfiles.

---

## 🧱 Repository Layout

```
slowdrip-miner/
├─ README.md
├─ LICENSE
├─ .gitignore
├─ .env.example
├─ Makefile
├─ docker-compose.yml
├─ configs/
│  ├─ mediamtx.yml            # MediaMTX (RTMP/RTSP/WebRTC/HLS + JWT/JWKS)
│  └─ miner.yaml              # Miner config (ports, API, module flags)
├─ deploy/
│  ├─ Dockerfile.miner        # Builds Go miner
│  ├─ Dockerfile.tools        # (optional) ffmpeg/gst tools
│  └─ k8s/                    # (later) Helm/manifests
├─ scripts/
│  ├─ dev-up.sh
│  ├─ dev-down.sh
│  ├─ gen-keys.sh             # (optional) local JWKS generator
│  └─ wait-for.sh
├─ cmd/
│  └─ miner/
│     └─ main.go
├─ internal/
│  ├─ api/server.go           # /healthz /readyz /metrics
│  ├─ config/config.go
│  ├─ logger/logger.go
│  ├─ metrics/metrics.go
│  ├─ mediamtx/               # MediaMTX REST client + watcher
│  │  ├─ client.go
│  │  └─ watcher.go
│  ├─ presence/agent.go       # PoP stub
│  ├─ service/agent.go        # PoS stub
│  ├─ receipts/signer.go      # (stub)
│  └─ wallet/keystore.go      # (stub)
└─ pkg/
   └─ backoff/backoff.go
```

---

## ⚙️ Prerequisites

* **Docker** & **docker-compose** (or Compose v2)
* Linux host recommended (for `network_mode: host`); macOS/Windows supported with port mappings
* Public IP or DNS if you plan to expose **WebRTC** (and optional **TURN**)

---

## 🚀 Quickstart

1. **Copy env & configs**

```bash
cp .env.example .env
# edit configs/mediamtx.yml:
#  - set authJWTJWKS (your JWKS URL)
#  - set webrtcICEHostNAT1To1IPs to your public IP/hostname (prod)
#  - optional: add TURN in webrtcICEServers
```

2. **Bring it up**

```bash
docker compose up -d --build
```

3. **Verify**

* Miner admin: [http://localhost:8080/healthz](http://localhost:8080/healthz) (also `/readyz`, `/metrics`)
* MediaMTX API: [http://localhost:9997/v3/paths/list](http://localhost:9997/v3/paths/list), `/v3/sessions/list`
* HLS (after publishing): `http://localhost:8888/live/stream/index.m3u8`


> **JWT**: include `Authorization: Bearer <token>` in WHIP/WHEP/RTSP/RTMP requests.
> **TURN** is recommended for strict NAT/CGNAT environments.

---

## 🔌 Endpoints & URLs

* **WHIP ingest (WebRTC)**: `https://YOUR_HOST:8443/whip/live/stream`
* **WHEP playback (WebRTC)**: `https://YOUR_HOST:8443/whep/live/stream`
* **RTMP ingest**: `rtmp://YOUR_HOST/live/stream`
* **HLS playback**: `http://YOUR_HOST:8888/live/stream/index.m3u8`
* **MediaMTX API**: `http://YOUR_HOST:9997/v3/paths/list` | `/v3/sessions/list`
* **Miner admin**: `http://YOUR_HOST:8080/healthz` | `/readyz` | `/metrics`

> Use valid TLS for `:8443` in production (reverse proxy or certs).

---

## 🧪 Testing

### Publish with FFmpeg (RTMP)

```bash
ffmpeg -re -stream_loop -1 -i test.mp4 -c:v libx264 -preset veryfast -b:v 2500k \
  -c:a aac -f flv rtmp://localhost/live/stream
```

Open [http://localhost:8888/live/stream/index.m3u8](http://localhost:8888/live/stream/index.m3u8) in a player.

### Publish with WHIP (OBS or web client)

* URL: `https://YOUR_HOST:8443/whip/live/stream`
* Header: `Authorization: Bearer <jwt>`

Play via WHEP: `https://YOUR_HOST:8443/whep/live/stream`



---

## 🧭 Roadmap (miner)

* [ ] **Presence agent**: VRF challenges, randomized heartbeats, nullifiers
* [ ] **Service agent**: QoS acceptance (deadline, jitter), integrity commits
* [ ] **Receipts**: per-segment signed receipts → Merkle batches
* [ ] **Wallet**: EVM signer (checkpoint/claim integration later)
* [ ] **UI**: local dashboard (paths, sessions, latency)
* [ ] **Packaging**: GitHub Actions → `ghcr.io/slowdrip-network/slowdrip-miner`

---

## 🛡️ Production Checklist

* [ ] Set `webrtcICEHostNAT1To1IPs` to public IP/hostname
* [ ] Provide **TURN** in `webrtcICEServers`
* [ ] Use valid TLS for `:8443`
* [ ] Harden JWT/JWKS and rotate keys
* [ ] Persist logs/metrics to your stack (Loki/Prom/Grafana)
* [ ] Limit exposed ports if not using host networking

---

## 🧰 Make Targets

```bash
make up        # docker compose up -d --build
make down      # docker compose down -v
make logs      # follow logs
make build     # local go build
make test      # (placeholder)
```

---

## 🤝 Contributing

PRs welcome. Please keep changes modular (one module per PR when possible) and include:

* brief rationale,
* configuration notes,
* how to test locally.

---

## 📄 License

Apache-2.0
