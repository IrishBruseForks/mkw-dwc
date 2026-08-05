# mkw-dwc

Bring Mario Kart Wii online again, on your own terms.

One Go binary that pretends to be Nintendo's old Wi-Fi Connection servers.
Point your Wiis at it (LAN or VPS), build with `go build`, and race.

No Python service farm, no external database, no cluster to babysit.

## What it does

| Job | How |
|-----|-----|
| Account login | NAS HTTP auth (`/ac`, `/pr`) |
| Optional NAS proxy | `--proxy-bind` for Nintendo NAS hostnames on port 80 |
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
sudo ./mkw-dwc --config mkw-dwc.ini --proxy-bind :80
```

Health checks:

```shell
curl http://127.0.0.1:9000/                               # NAS -> ok
curl -H "Host: naswii.nintendowifi.net" http://127.0.0.1/ # proxy -> ok
```

```shell
go test ./tests/...
```

## CLI

| Flag | Default | Meaning |
|------|---------|---------|
| `--config` | `mkw-dwc.ini` | Path to server INI |
| `--proxy-bind` | empty (off) | NAS proxy listen address, e.g. `:80` |

`--proxy-bind :80` forwards `naswii.nintendowifi.net` and `nas.nintendowifi.net`
to NAS on port 9000. Binding port 80 usually needs `sudo` or
`CAP_NET_BIND_SERVICE`.

## Ports

Defaults from `mkw-dwc.ini` (and `--proxy-bind`):

| Service | Port | Proto |
|---------|------|-------|
| HTTP proxy (optional) | 80 | TCP |
| NAS | 9000 | TCP |
| QR | 27900 | UDP |
| NAT negotiation | 27901 | UDP |
| Server browser | 28910 | TCP |
| Profile | 29900 | TCP |
| GPSP (player search) | 29901 | TCP |

Skip `--proxy-bind` and expose NAS yourself? Open `9000/tcp` as well.

## Docs

- [Setup tutorial](docs/setup.md) - LAN / VPS hosting, DNS, Wii client, troubleshooting
- [Architecture](docs/architecture/README.md) - how online play works, glossary, per-service detail

## Related

- [Vega's MKW server setup guide](https://mariokartwii.com/showthread.php?tid=885)
- [Original dwc_network_server_emulator](https://github.com/barronwaffles/dwc_network_server_emulator) (Polaris)
