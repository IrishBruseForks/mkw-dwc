# Setup tutorial

Host and configure `mkw-dwc` for LAN or VPS use. For a plain-English map of
how online play works (plus a glossary of NAS / GameSpy terms), see
[Architecture](architecture/README.md).

## 1. Packages

```shell
sudo apt-get update && sudo apt-get upgrade && sudo apt-get dist-upgrade
sudo apt-get autoremove && sudo apt-get autoclean
```

Reboot if upgrades ran. Then install what this guide needs:

```shell
sudo apt-get update && sudo apt-get install ufw dnsmasq curl
```

Install a Go toolchain (1.22+) if needed (`go version`). Use your distro package or [go.dev](https://go.dev/dl/).

---

## 2. Firewall and port forwarding

If you already have firewall rules, clear or back them up first.

```shell
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 80/tcp
sudo ufw enable
sudo ufw status verbose
```

Then open the GameSpy ports:

```shell
sudo ufw allow 27900/udp
sudo ufw allow 27901/udp
sudo ufw allow 28910/tcp
sudo ufw allow 29900/tcp
sudo ufw allow 29901/tcp
```

Most online play also needs a wide UDP range:

```shell
sudo ufw allow 2:65535/udp
sudo ufw reload
```

Add `sudo ufw allow 9000/tcp` only if you skip `--proxy-bind`.

**Router:** if the host is behind a home router, forward the same ports to the machine running `mkw-dwc`.

You do not need ports for storage, admin UI, stats, or DLS. Those services are not part of this build.

---

## 3. Build

```shell
cd mkw-dwc
go build -o mkw-dwc .
go test ./tests/...
```

Put the folder wherever you like. This guide uses `/home/yourusername/mkw-dwc`.

---

## 4. Config

Skip Apache/Nginx. The binary has a built-in proxy.

Default `mkw-dwc.ini` listens on all interfaces. Edit ports/IPs if needed.
`[Store]` is required. Optional `[Logging]` controls verbosity, colors, and
per-service log toggles:

```ini
[Logging]
Level = info # debug | info | warn | error
Color = auto # auto | always | never (ANSI colors when stderr is a TTY)
Timestamps = true # prefix each line with date/time
Nas = true # NAS HTTP server
Profile = true # GPCM profile server
Qr = true # QR / master server
Browser = true # server browser
Natneg = true # NAT negotiation
Proxy = true # reverse proxy
App = true # startup and lifecycle
LogFile = logs/mkw-dwc.log # mirror stderr INFO/WARN/ERROR lines (empty to disable)
DumpFile = logs/http-traffic.log # raw NAS/proxy TCP dumps (empty to disable; debug 20100/23400)

[NasServer]
IP = 0.0.0.0 # bind address
Port = 9000 # TCP listen port
SvcHost = dls1.nintendowifi.net # hostname returned for NAS service location

[GameSpyQRServer]
IP = 0.0.0.0 # bind address
Port = 27900 # UDP listen port

[GameSpyNatNegServer]
IP = 0.0.0.0 # bind address
Port = 27901 # UDP listen port

[GameSpyServerBrowserServer]
IP = 0.0.0.0 # bind address
Port = 28910 # TCP listen port

[GameSpyProfileServer]
IP = 0.0.0.0 # bind address
Port = 29900 # TCP listen port

[GameSpyPlayerSearchServer]
IP = 0.0.0.0 # bind address
Port = 29901 # TCP listen port

[Store]
Type = "json" # "json"
Path = "data" # JSON data directory
```

Typical run:

```shell
sudo ./mkw-dwc --config mkw-dwc.ini --proxy-bind :80
```

---

## 5. DNS (dnsmasq or hosts)

Wiis must resolve `nintendowifi.net` to your server.

### LAN: dnsmasq

Edit `/etc/dnsmasq.conf`. Find a commented example like:

```
#address=/double-click.net/127.0.0.1
```

Uncomment it, replace `double-click.net` with `nintendowifi.net`, and set the IP to your machine's **LAN** address (not public). Find it with:

```shell
hostname -I
```

Use the IPv4 address (no colons). Example for `192.168.1.9`:

```
address=/nintendowifi.net/192.168.1.9
```

Restart:

```shell
sudo service dnsmasq restart
```

### Dolphin / single PC: hosts file

Skip dnsmasq. Point these names at your server IP (example `192.168.1.100`):

```
192.168.1.100 naswii.nintendowifi.net
192.168.1.100 nas.nintendowifi.net
192.168.1.100 mariokartwii.available.gs.nintendowifi.net
192.168.1.100 mariokartwii.master.gs.nintendowifi.net
192.168.1.100 mariokartwii.ms19.gs.nintendowifi.net
192.168.1.100 mariokartwii.natneg1.gs.nintendowifi.net
192.168.1.100 mariokartwii.natneg2.gs.nintendowifi.net
192.168.1.100 mariokartwii.natneg3.gs.nintendowifi.net
192.168.1.100 gpcm.gs.nintendowifi.net
192.168.1.100 gpsp.gs.nintendowifi.net
```

- Windows: `C:\Windows\System32\drivers\etc\hosts` (edit as Administrator)
- Linux / macOS: `/etc/hosts`

---

## 6. First boot

```shell
cd /home/yourusername/mkw-dwc
sudo ./mkw-dwc --config mkw-dwc.ini --proxy-bind :80
```

Expected log lines (timestamps and colors depend on `[Logging]`):

```
INFO  app     store: type=json path=data
INFO  app     logging: level=debug color=auto timestamps=false log_file="logs/mkw-dwc.log" dump_file="logs/http-traffic.log"
INFO  app     nas: :9000
INFO  app     profile: :29900
INFO  app     gpsp: :29901
INFO  app     qr: :27900
INFO  app     browser: :28910
INFO  app     natneg: :27901
INFO  app     proxy: :80
INFO  app     starting nas
INFO  nas     listening on :9000
INFO  app     starting profile
...
```

Store files under `[Store]` `Path` are created on first run. Set
`Type = "json"` in `mkw-dwc.ini`.

Confirm with the [health checks](../README.md#quick-start) in the README. If both return `ok`, move on to the Wii.

There is no admin page (`:9009`) or stats page (`:9001`) in this build. Use health checks instead.

---

## 7. Wii connection test

1. **Wii Settings** -> **Internet Settings**.
2. Create or edit a connection and complete the initial test.
3. **Change Settings** -> set **Auto-Obtain DNS** to **No**.
4. Set **Primary DNS** and **Secondary DNS** to the same LAN IP used in dnsmasq.
5. Save and run the Connection Test again.

Watch the server terminal for request logs. On success, exit Wii Settings. If it fails, see [Troubleshooting](#troubleshooting).

---

## 8. Connect with MKWii

Use a clean unmodified ISO/WBFS. Do not use ISOs patched for other servers.

1. Load the ISO/WBFS from USB via HBC as usual.
2. Disable SSL:
   - USB Loader GX (rev 1256+): Loader Settings -> Private Server -> **NoSSL**
   - Other loaders: NoSSL Gecko code from [Vega's guide](https://mariokartwii.com/showthread.php?tid=885)
3. Boot the game.
4. Use a **fresh license** with no Friend Code.
5. **Nintendo WFC** -> **Wi-Fi Connection**.

Heavy log output on first connect is normal. Keep-alives afterward are normal too.

### Dolphin

- Same hosts entries or DNS as above
- NoSSL Gecko code (NAS must be HTTP, not HTTPS)
- Clean rip and fresh license

Retail MKWii builds `https://` NAS URLs. This server speaks plain HTTP. Without NoSSL (or an equivalent Gecko hook), you get SSL/connection errors even when `mkw-dwc` is up.

---

## 9. Public hosting (VPS)

Prefer a VPS over a random home PC. You need root and a public IPv4. Two options:

- Users set Wii DNS to the VPS **public IP**
- Users connect via a **domain** that points at the VPS

Either way, open the same ports in both `ufw` and the provider firewall panel.
Hard reboot the VPS before launching `mkw-dwc`. Some providers block UDP or game
traffic, so the public IP method is not guaranteed everywhere.

### Public IP method

Repeat sections 1-6 on the VPS with these changes:

- dnsmasq: use the VPS public IPv4 in `address=/nintendowifi.net/...`
- No home-router port forwarding

Connect like LAN (sections 7-8), but use the public IP for Wii DNS.

### Domain method

Repeat sections 1-4 and 6 on the VPS. Skip dnsmasq (remove it if you installed it earlier).

On your DNS provider, point records at the VPS, for example:

```
www
nas
naswii
dls1
conntest
*.gs
```

DNS can take 15+ minutes to propagate. On the Wii, use NoSSL plus a Domain Full
Name Changer code (see [Vega's guide, Chapter 12](https://mariokartwii.com/showthread.php?tid=885)).
USB Loader GX Private Server set to NoSSL already covers the SSL part.

Either way: clean ISO/WBFS, then try connecting. Failures go to [Troubleshooting](#troubleshooting).

---

## Troubleshooting

**Stop the server:** exit the `mkw-dwc` terminal, or
`sudo systemctl stop mkw-dwc` if using systemd. Disconnecting while online
usually yields EC 84010 or 91010.

### `mkw-dwc` won't start

- Confirm Go 1.22+, a valid `mkw-dwc.ini`, and free ports (`ss -tulpn`)
- Port 80 needs `sudo`, `CAP_NET_BIND_SERVICE`, or another `--proxy-bind` port matched on the client side

### Health check fails

- Confirm `mkw-dwc` is still running: `curl http://127.0.0.1:9000/` should return `ok`
- Confirm `ufw` allows `80/tcp` and you passed `--proxy-bind :80`
- From another device, use the machine's LAN IP instead of `localhost`

### Wii Connection Test fails

- Primary and Secondary DNS must both be the server IP (watch for typos)
- Confirm `mkw-dwc` is running
- Restart dnsmasq: `sudo service dnsmasq restart`
- Reload firewall: `sudo ufw reload`

### MKWii error codes

- **20100** - dirty ISO, NoSSL missing, wrong Wii DNS, or very bad network
- **234XX** - SSL/proxy mismatch, or NAS HTTP rejected (duplicate `Host`
  headers from MKW/Dolphin used to 400 in Go). Enable NoSSL, use
  `--proxy-bind :80`, and run a build that strips duplicate Host headers
- **23502** - something on port 80, but `mkw-dwc` is not running
- **5XXXX** - no Wii internet connection, or network issue
- **60000** - license already has a Friend Code. Use a brand new license
- **84010 / 91010** - `mkw-dwc` stopped or the terminal was closed

## Running as a service (Linux)

Example unit at `/etc/systemd/system/mkw-dwc.service`:

```ini
[Unit]
Description=mkw-dwc
After=network.target

[Service]
Type=simple
User=mkw-dwc
WorkingDirectory=/opt/mkw-dwc
ExecStart=/opt/mkw-dwc/mkw-dwc \
  --config mkw-dwc.ini \
  --proxy-bind :80
Restart=on-failure
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

```shell
sudo systemctl daemon-reload
sudo systemctl enable --now mkw-dwc
```
