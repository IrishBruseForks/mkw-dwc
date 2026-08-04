# Server Browser

Clients search for rooms over TCP, get a filtered list, pick one, and the
server can relay a join message to the host. Not a web page, despite the name.

Package: `internal/gamespy/browser`  
Config: `[GameSpyServerBrowserServer]` (default `0.0.0.0:28910`)  
Protocol: encrypted GameSpy server-browser binary over TCP  
Hostname: `mariokartwii.ms19.gs.nintendowifi.net`

## What it is

The server browser is how Mario Kart Wii **searches for rooms** and kicks off
joining. It is a TCP service on port 28910 that speaks a binary protocol (SBCM /
related frames), not HTTP.

Opening `http://host:28910` in a web browser will show nothing useful. The log
line `browser: listening on :28910` only means the TCP listener is up.

## Why it exists

[QR](qr.md) collects host advertisements. Clients need a query interface:
filter rooms, receive encrypted result lists, and send messages toward a chosen
host. That is this service.

## What the Wii does

1. Connects to `mariokartwii.ms19.gs.nintendowifi.net:28910`
2. Sends a **server list** request (`cmd 0x00`) with game name, challenge,
   filter expression, and requested fields
3. Server queries [backend](backend.md) `FindServers`, builds an encrypted reply
4. To join / negotiate, client may send **send message** (`cmd 0x02`); the
   browser looks up the host and relays via QR (`FE FD 06`)
5. **Keep-alive** (`cmd 0x03`) keeps the TCP session warm

Own-IP probes (empty filter/fields or `optNoServerList`) return the client's
public IP as seen by the server (used for NAT-related checks).

## Commands

| Cmd | Name | Role |
|-----|------|------|
| `0x00` | Server list | Query rooms / own-IP |
| `0x02` | Send message | Relay payload to a host via QR |
| `0x03` | Keep-alive | No-op ack path |

Responses are encrypted with EncTypeX using the game secret from `keys.go`.

## Empty room list

`returning 0 room(s)` is normal when:

- No host has registered via QR yet
- Rooms timed out (~61s without heartbeat)
- Filter expression matched nothing

You need at least one client **hosting** (QR heartbeats) before another client
can see a room here.

## Shared state

- Reads room list from [backend](backend.md)
- May register NATNEG cookie mappings via `AddNatnegServer` when join packets
  carry NATNEG magic
- Relays through the QR server (`browser.Relay`)

## Logs to expect

```
browser: listening on :28910 ...
browser: connection from ...
browser: server list query ... game="mariokartwii" ...
browser: returning N room(s) for game="mariokartwii"
```

If you never see `connection from`, DNS for `ms19` (or your wildcard) is wrong
or the Wii never reached GameSpy login.

## Related

- [QR](qr.md) - source of rooms and UDP relay target
- [Backend](backend.md) - `FindServers` / filters
- [NATNEG](natneg.md) - peer connect after join messaging
