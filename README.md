# mkw-dwc

Bring Mario Kart Wii online again, on your own terms.

One Go binary that pretends to be Nintendo's old Wi-Fi Connection servers.
Point your Wiis at it (LAN or VPS), build with `go build`, and race.

No Python service farm, no external database, no cluster to babysit.

## What it does

| Job | How |
|-----|-----|
| Account login | NAS HTTP auth on port 80 (`/ac`, `/pr`) |
| GameSpy login | Profile server (GPCM) |
| Friend lookup | GPSP player search (`otherslist`) |
| Room advertise / search | QR (UDP) + server browser (TCP) |
| NAT punch-through | NAT negotiation (UDP) |
| Player data | JSON account store via `[Store]` in `mkw-dwc.ini` |

Not included: storage, admin UI, game stats, or DLS.

## Requirements

- Go 1.22+ to build
- Linux recommended for hosting
- DNS pointing Nintendo domains at your server (dnsmasq, hosts file, or custom DNS)
- Retail MKWii: NoSSL Gecko code, or USB Loader GX Private Server set to **NoSSL**

## Quick start

```shell
git clone <repo-url>
cd mkw-dwc
go build -o mkw-dwc .
```

Set `[Store]` in `mkw-dwc.ini` before running:

```ini
[Store]
Type = "json"
Path = "data"
```

```shell
sudo ./mkw-dwc --config mkw-dwc.ini
```

`sudo` (or `CAP_NET_BIND_SERVICE`) is only so NAS can bind port 80. Retail
clients expect NAS there.

Health check:

```shell
curl -H "Host: naswii.nintendowifi.net" http://127.0.0.1/ # NAS -> ok
```

```shell
go test ./tests/...
```

## CLI

| Flag | Default | Meaning |
|------|---------|---------|
| `--config` | `mkw-dwc.ini` | Path to server INI |
| `--proxy-bind` | empty (off) | Optional reverse proxy when NAS is not on 80 |

Default NAS listens on port 80 (`[NasServer] Port`). Use `--proxy-bind` only if
you put NAS on another port (for example 9000) and want a front door that
forwards `naswii.nintendowifi.net` / `nas.nintendowifi.net` to that backend.

## Ports

Defaults from `mkw-dwc.ini`:

| Service | Port | Proto |
|---------|------|-------|
| NAS | 80 | TCP |
| QR | 27900 | UDP |
| NAT negotiation | 27901 | UDP |
| Server browser | 28910 | TCP |
| Profile | 29900 | TCP |
| GPSP (player search) | 29901 | TCP |

Optional HTTP proxy via `--proxy-bind` if you move NAS off port 80.

## Docs

- [Local testing with Dolphin](docs/local-testing.md) - Linux + Flatpak Dolphin, step by step
- [Setup tutorial](docs/setup.md) - LAN / VPS hosting, DNS, Wii client, troubleshooting
- [Architecture](docs/architecture/README.md) - how online play works, glossary, per-service detail

## Related

- [Vega's MKW server setup guide](https://mariokartwii.com/showthread.php?tid=885)
- [Original dwc_network_server_emulator](https://github.com/barronwaffles/dwc_network_server_emulator) (Polaris)
