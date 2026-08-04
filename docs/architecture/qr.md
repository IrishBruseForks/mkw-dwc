# QR / Master (Query & Reporting)

Hosts advertise rooms here with UDP heartbeats. Guests do **not** search rooms
on this port. They use the [server browser](browser.md).

Package: `internal/gamespy/qr`  
Config: `[GameSpyQRServer]` (default `0.0.0.0:27900`)  
Protocol: GameSpy QR binary over UDP  
Hostnames: `mariokartwii.master.gs.nintendowifi.net`, `mariokartwii.available.gs.nintendowifi.net`

## What it is

QR is the GameSpy "master" / query-and-reporting server. **Room hosts** talk to
it over UDP to advertise that a room exists. The live list is stored in the
in-memory [backend](backend.md). Clients do **not** browse
rooms here. They use the [server browser](browser.md) on TCP 28910.

If nobody is hosting, the browser returns zero rooms even when QR is healthy.

## Why it exists

Matchmaking needs a registry of current rooms (public IP/port, local endpoints,
game fields such as `dwc_pid`). QR is how hosts register and refresh that data.

## What a host does

1. Sends heartbeats (`0x03`) with key/value fields (`gamename`, `publicip`, ...)
2. Server challenges the client (`0xfe 0xfd 0x01` + challenge string)
3. Client answers (`0x01`). Server verifies with the game secret key (`mariokartwii` -> `9r3Rmy`)
4. After success, heartbeats update the backend room list
5. Keepalives (`0x08`) refresh the session. Idle sessions are pruned (~61s)
6. `statechanged=2` removes the room

Availability probes (`0x09`) get a short ack so the game knows the master is up.

## Commands (client -> server)

| Byte | Meaning |
|------|---------|
| `0x01` | Challenge response |
| `0x03` | Heartbeat (room fields) |
| `0x08` | Keepalive |
| `0x09` | Availability |

## Relay role

QR also implements `browser.Relay`. When the [browser](browser.md) needs to push
a join/connect payload to a host, it calls `ForwardClientMessage`, which sends an
`FE FD 06` UDP packet to that host's QR session.

## Shared state

- Writes rooms into [backend](backend.md) via `UpdateServerList` / `DeleteServer`
- Does **not** use SQLite for the room list (memory only, lost on restart)

## Logs to expect

```
qr: listening on :27900
qr: room registered gamename=mariokartwii session=...
qr: room removed gamename=mariokartwii session=...
```

## Related

- [Backend](backend.md) - where rooms live
- [Browser](browser.md) - reads rooms and relays via QR
- [NATNEG](natneg.md) - uses public/local endpoints from heartbeats
