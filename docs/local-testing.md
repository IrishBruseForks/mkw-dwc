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
sudo go run . --config mkw-dwc.ini
```

`sudo` is only so the process can bind port 80. Retail clients expect NAS on
port 80. Default `[NasServer] Port = 80` serves them directly (no proxy).

Leave this terminal open. You should see startup lines for `nas`, `profile`,
`gpsp`, `qr`, `browser`, and `natneg`.

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
curl -H "Host: naswii.nintendowifi.net" http://127.0.0.1/
```

It should print `ok`.

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
and run menu automation in one step:

```shell
just test "/home/econn/data/Emulation/roms/wii/Mario Kart Wii.iso"
# or leave windows unmoved:
just test2 "/home/econn/data/Emulation/roms/wii/Mario Kart Wii.iso"
# or: MKWII_ISO=/path/to/MKWii.iso just test
```

Or launch only, then automate separately:

```shell
just launch "/home/econn/data/Emulation/roms/wii/Mario Kart Wii.iso"
# or: MKWII_ISO=/path/to/MKWii.iso just launch
```

That opens two Dolphin windows only (no input automation). Menu automation is
`just test` (tile on primary + WFC flow) or `just test2` (same flow, no window
move). Low-level keyboard helpers live in `scripts/automate.js`:

```javascript
import { createSession, pressA, dpad, sleep, run } from "./scripts/automate.js";

createSession();
sleep(45000); // wait for logos/title
pressA("both", 1);
sleep(5000);
pressA("both", 1);
sleep(5000);
dpad("both", "Down", 1);
pressA("both", 1);
await run();
```

`just test` tiles both windows on the **primary** monitor, then runs the WFC flow.
`just test2` is the same flow but does **not** move/tile windows.

Defaults wait 45s for boot, then 5s between each menu phase. Tune with:

```shell
MKWII_BOOT_WAIT=60 just test2 /path/to/MKWii.iso
MKWII_PHASE_WAIT=8 just test2 /path/to/MKWii.iso
MKWII_DPAD_DOWN_COUNT=1 just test2 /path/to/MKWii.iso
```

`just launch` / `just test` seeds NoSSL, EnableCheats, and controllers into
`tmp/dolphin-test/`. P1 keeps your Flatpak Xbox/GCPad mapping and ORs keyboard
binds as `(`XInput2/0/Virtual core pointer:Key`)` so both pad and automation
work. P2 uses keyboard-only so it does not steal the same SDL pad.
Ctrl+C on the launch terminal stops both Dolphin instances. Watch the `mkw-dwc`
terminal for NAS, profile, and gpsp traffic.

### Still getting 20100?

`just launch` seeds NoSSL and EnableCheats into `tmp/dolphin-test/`. If you still
see 20100:

- Confirm the server dump log shows plain HTTP, not `TLS handshake` / `NoSSL
  likely not running`
- Confirm health checks still return `ok` and hosts still resolve to
  `127.0.0.1`
- Use an unpatched ISO and a fresh license on each test window
- Delete `tmp/dolphin-test/` and rerun `just launch` so GameSettings are reseeded

### Still getting 23400?

- Confirm NoSSL is actually running (Enable Cheats + Gecko checked)
- Confirm NAS is listening on port 80 and health checks return `ok`
- Restart `mkw-dwc` from a build that includes the duplicate-Host fix
  (`internal/httpfix`). MKW/Dolphin send `Host` twice. Older builds return
  HTTP 400 for those requests

### Error 61020 and/or 60000 on dual Dolphin?

- **61020**: both clients shared one NAS userid, so the second NAS login
  replaced the first authtoken. `just open` / `launch` / `test` / `test2`
  seed distinct `Wii/shared2/DWC_AUTHDATA` (userid 2 and 3) and move the
  Flatpak default AUTHDATA aside so both windows cannot pick up the same id.
- **60000**: license still has an old Friend Code / profile id. Those same
  just recipes wipe `rksys.dat` so each window creates a fresh license (no
  Friend Code). Do not hex-edit the save, that corrupts checksums.
- After fixing identities, clear `data/nas_logins.json` (or the whole `data/`
  dir) and reconnect so tokens match the new AUTHDATA files.

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
