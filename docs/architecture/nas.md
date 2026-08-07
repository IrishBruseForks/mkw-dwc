# NAS (Nintendo Authentication Server)

First hop for Wi-Fi Connection. The Wii creates an account or logs in here,
receives an auth token, and learns where the other online services live.

Package: `internal/nas`  
Config: `[NasServer]` (default `0.0.0.0:80`)  
Protocol: HTTP over TCP

## What it is

NAS is the first stop when a Wii connects to Nintendo Wi-Fi Connection. It
handles account creation, login tokens, and service location. Retail Mario Kart
Wii talks to hostnames such as `naswii.nintendowifi.net` on port 80.

This build speaks **plain HTTP**. Retail clients build `https://` URLs, so you
need NoSSL (Gecko code or USB Loader GX Private Server set to NoSSL).

## Why it exists

Without NAS, the game never receives an auth token and cannot continue to
GameSpy (profile, room list, matchmaking). Everything online starts here.

## What the Wii does

Typical first-connect flow:

1. `POST /ac` with `action=acctcreate` on a fresh license (new 13-digit userid)
2. `POST /ac` with `action=login` (token + challenge)
3. `POST /ac` with `action=svcloc` (where other services live)
4. Later GameSpy login uses the NAS authtoken against the [profile](profile.md) server

## Endpoints

| Path | Method | Purpose |
|------|--------|---------|
| `/` | GET | Health check, returns `ok` |
| `/ac` | POST | Account control (`acctcreate`, `login`, `svcloc`) |
| `/pr` | POST | Profanity filter stub (always passes) |

### `/ac` actions

- **`acctcreate`**: reserves and returns a new userid (`returncd=002`) unless
  the IP is banned (`3913`). Unlike the Python reference, each call persists
  the next userid immediately so concurrent Dolphin clients cannot receive the
  same ID before either has a profile row.
- **`login`**: stores NAS login data, returns `token` (authtoken) and
  `challenge` (`returncd=001`). One active token per userid (same as the
  reference): a later login replaces the previous token.
- **`svcloc`**: returns `svchost` / tokens for service codes such as `9000` /
  `9001`

Responses are Nintendo-style query strings with base64-ish encoding (`=` becomes `*`).

## Shared state

- Writes **`nas_logins`** and reads **`banned`** in the [account store](database.md)
- `SvcHost` comes from `mkw-dwc.ini` (`NasServer.SvcHost`, default `dls1.nintendowifi.net`)

## Hostnames

Clients usually hit NAS through DNS spoofing:

- `naswii.nintendowifi.net`
- `nas.nintendowifi.net`

Default `[NasServer] Port = 80` matches what retail clients dial. Binding port
80 usually needs `sudo` or `CAP_NET_BIND_SERVICE`. If you put NAS on another
port, use [`--proxy-bind`](proxy.md) (or an external reverse proxy) so clients
still reach something on 80.

## Logs / health

```shell
curl -H "Host: naswii.nintendowifi.net" http://127.0.0.1/   # -> ok
```

When `[LoggingComponents]` `Nas = true` (default), expect lines like these on first Wii
connect (timestamps and colors depend on `[Logging]`):

```
INFO  nas     listening on :80
INFO  nas     acctcreate userid=1234567890123 gamecd=RMCE ip=192.168.1.50
INFO  nas     login userid=1234567890123 gamecd=RMCE ip=192.168.1.50
INFO  nas     svcloc userid=1234567890123 gamecd=RMCE ip=192.168.1.50
```

Banned clients log `WARN` with `returncd=3913` (acctcreate) or `3914` (login).
Bad POST bodies and unknown `/ac` actions also appear at `WARN`.

There is no HTML admin UI on this port.

## Related

- [Proxy](proxy.md) - optional front door when NAS is not on port 80
- [Profile](profile.md) - validates the NAS authtoken
- [Database](database.md) - stores tokens and bans
