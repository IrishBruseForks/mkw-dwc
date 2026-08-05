# Local testing with Dolphin (Linux)

Connect Flatpak Dolphin running Mario Kart Wii to `mkw-dwc` on the same Linux
machine. For LAN hardware Wii hosting, see [Setup](setup.md).

Walk through these steps in order. Default `mkw-dwc.ini` is fine for local use
(`[Store]` should already be `Type = "json"` / `Path = "data"`).
`RewriteDolphinLocalIP = true` under `[GameSpyQRServer]` is on by default so two
local Dolphin clients can join each other. For verbose debug / HTTP dumps, set
in `[Logging]`:

```ini
Level = debug
Timestamps = false
LogFile = logs/mkw-dwc.log
DumpFile = logs/http-traffic.log
```

## What you need

- Go 1.22+
- Dolphin via Flatpak: `flatpak install flathub org.DolphinEmu.dolphin-emu`
- A **clean** Mario Kart Wii ISO (no patches from other private servers)
- A Wii license with **no Friend Code** (new save in Dolphin if needed)

Retail MKWii uses HTTPS to NAS on port 80. This server uses plain HTTP, so you
also need a **NoSSL** Gecko code in Dolphin (step later).

## 1. Run the server

From the repo root:

```shell
sudo go run . --config mkw-dwc.ini --proxy-bind :80
```

`sudo` is only so the process can bind port 80. Retail clients expect NAS on
port 80. The proxy forwards those hostnames to NAS on 9000.

Leave this terminal open. You should see startup lines for `nas`, `profile`,
`gpsp`, `qr`, `browser`, `natneg`, and `proxy`.

### Port 80 already in use

If you see `listen tcp :80: bind: address already in use`, something else is on
port 80. Retail clients need NAS on 80, so free it for local testing:

```shell
ss -tulpn | grep ':80'
sudo lsof -iTCP:80 -sTCP:LISTEN
```

Stop that service for the session (example: Caddy):

```shell
sudo systemctl stop caddy
```

Then run step 1 again. Start the other service again when you are done testing
(`sudo systemctl start caddy`).

## 2. Health checks

In a second terminal:

```shell
curl http://127.0.0.1:9000/
curl -H "Host: naswii.nintendowifi.net" http://127.0.0.1/
```

Both should print `ok`.

## 3. Hosts file

Dolphin must resolve Nintendo hostnames to your machine. On Linux, edit
`/etc/hosts` (needs sudo):

```shell
sudo nano /etc/hosts
```

Add these lines (same machine as Dolphin and the server):

```
127.0.0.1 naswii.nintendowifi.net
127.0.0.1 nas.nintendowifi.net
127.0.0.1 mariokartwii.available.gs.nintendowifi.net
127.0.0.1 mariokartwii.master.gs.nintendowifi.net
127.0.0.1 mariokartwii.ms19.gs.nintendowifi.net
127.0.0.1 mariokartwii.natneg1.gs.nintendowifi.net
127.0.0.1 mariokartwii.natneg2.gs.nintendowifi.net
127.0.0.1 mariokartwii.natneg3.gs.nintendowifi.net
127.0.0.1 gpcm.gs.nintendowifi.net
127.0.0.1 gpsp.gs.nintendowifi.net
```

Save and exit. Quick check:

```shell
getent hosts naswii.nintendowifi.net
```

That should show `127.0.0.1`. Flatpak Dolphin uses the host `/etc/hosts`, so no
extra Flatpak config is needed for DNS.

## 4. Dolphin NoSSL

Retail MKWii builds `https://` NAS URLs. This server speaks plain HTTP. Checking
the NoSSL code in Properties is not enough: Dolphin must also have **cheats
enabled globally**, or the Gecko code never runs and you get **error 20100**.

### Option A: helper script

From the repo root:

```shell
scripts/local.js dolphin
```

That writes the NoSSL Gecko code into Flatpak game settings (`RMCE01` /
`RMCJ01` / `RMCP01`) and sets `EnableCheats = True` in Flatpak
`Dolphin.ini`.

### Option B: manual

1. In Dolphin: **Config** -> **General** -> enable **Enable Cheats**.
2. Right-click Mario Kart Wii -> **Properties** -> **Gecko Codes**.
3. Add and enable Fix94's NoSSL code (name it `$NoSSL` or `$NoSSL [Fix94]`):

```
C0000000 0000000E
3C004E80 60000020
900F0000 3D808000
618C3000 3C00017F
6000CFFC 7C0903A6
3D607474 616B7073
800C0000 7C005800
40A20034 394C0003
392C0002 7D455378
38600000 8C050001
2C000000 38630001
4082FFF4 8C0A0001
9C090001 3463FFFF
4082FFF4 398C0001
4200FFC0 4E800020
```

4. Confirm the code is checked under **Gecko Codes**.

Restart Dolphin fully after changing cheats settings.

## 5. Connect

With the server already running (`just run`), launch two muted Dolphin clients
(NoSSL + cheats seeded, settings UI hidden):

```shell
just test "/home/econn/data/Emulation/roms/wii/Mario Kart Wii.iso"
# or: MKWII_ISO=/path/to/MKWii.iso just test
```

After both windows appear, run menu automation separately:

```shell
just auto
# or: ./scripts/test-mkwii.sh auto
```

That presses on each window: title A -> license A -> Down Down (Nintendo WFC) -> A (1
Player). Needs an existing license on each save (create one once if the slots are
empty).

Tune automation with env:

```shell
MKWII_BOOT_WAIT=50 just auto        # slower boot / longer intro
MKWII_SKIP_AS=2 just auto           # extra A to skip logos
```

`just auto` defaults to no boot wait. On Wayland it uses Cinnamon window focus plus
`ydotool` for clicks and keys (needs `ydotoold` running). `just test` copies your
Flatpak Dolphin controller profile (`WiimoteNew.ini`, `GCPadNew.ini`) into each
isolated user dir and ensures Wiimote A also includes mouse left click for
automation. Override the source with `MKWII_CONTROLLER_CONFIG` if needed. Ctrl+C
stops both. Watch the `mkw-dwc` terminal for NAS, profile, and gpsp traffic.

### Still getting 20100?

`just test` seeds NoSSL and EnableCheats into `tmp/dolphin-test/`. If you still
see 20100:

- Confirm the server dump log shows plain HTTP, not `TLS handshake` / `NoSSL
  likely not running`
- Confirm health checks still return `ok` and hosts still resolve to
  `127.0.0.1`
- Use an unpatched ISO and a fresh license on each test window
- Delete `tmp/dolphin-test/` and rerun `just test` so GameSettings are reseeded

### Still getting 23400?

- Confirm NoSSL is actually running (Enable Cheats + Gecko checked)
- Confirm `--proxy-bind :80` and health checks return `ok`
- Restart `mkw-dwc` from a build that includes the duplicate-Host fix
  (`internal/httpfix`). MKW/Dolphin send `Host` twice. Older builds return
  HTTP 400 for those requests

### Stuck on "Connecting to Nintendo WFC..."?

NAS and profile can succeed while the game still hangs here. Mario Kart Wii
then needs [GPSP](architecture/gpsp.md) on port 29901.

- Confirm startup logs include `gpsp: :29901` and `gpsp listening on :29901`
- Add `127.0.0.1 gpsp.gs.nintendowifi.net` to `/etc/hosts` (see step 3)
- Check `getent hosts gpsp.gs.nintendowifi.net` resolves to `127.0.0.1`
- On reconnect, look for `gpsp otherslist` in the server terminal (empty
  friends still send `opids=0`)

## Related

- [Setup](setup.md)
- [Architecture](architecture/README.md)
