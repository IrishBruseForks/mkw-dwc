# NAT Negotiation

After two players pick the same room, NATNEG exchanges public (and local)
addresses so their Wiis can punch through home NATs and talk directly. It does
not carry race traffic.

Package: `internal/gamespy/natneg`  
Config: `[GameSpyNatNegServer]` (default `0.0.0.0:27901`)  
Protocol: GameSpy NATNEG v0x03 over UDP  
Hostnames: `mariokartwii.natneg1.gs.nintendowifi.net` (and natneg2 / natneg3)

## What it is

NATNEG helps two Mario Kart Wii clients discover each other's public (and
local) UDP endpoints so they can open a peer-to-peer game connection through
home NATs. It does not carry race traffic itself. It coordinates hole punching.

This build implements protocol **version `0x03`** (what MKWii uses). Packets
start with magic `fd fc 1e 66 6a b2`.

## Why it exists

After a room is found via the [browser](browser.md), players still need a direct
UDP path. Carrier-grade and residential NATs block unsolicited inbound packets.
NATNEG exchanges observed addresses and tells each peer where to send CONNECT
traffic.

## Rough flow

1. Each peer sends **INIT** with a shared session / cookie and local address
2. Server replies **INIT_ACK**
3. When two peers share a session, server sends **CONNECT** packets pointing
   each side at the other (using [backend](backend.md) lookups when needed)
4. Peers ack (**CONNECT_ACK**), may run address check / natify / report steps
5. Sessions idle out after **30 minutes**; backend NATNEG entries are cleared

Outgoing replies are slightly delayed (~50ms) to match expected timing.

## Record types handled

| Type | Name |
|------|------|
| `0x00` | INIT |
| `0x01` | INIT_ACK (server) |
| `0x05` | CONNECT (server) |
| `0x06` | CONNECT_ACK |
| `0x0a` | ADDRESS_CHECK |
| `0x0b` | ADDRESS_REPLY (server) |
| `0x0c` | NATIFY |
| `0x0d` | REPORT |
| `0x0e` | REPORT_ACK (server) |

## Shared state

- Session map in process memory (cookie -> client slots)
- `GetNatnegServer` / `AddNatnegServer` / `DeleteNatnegServer` on [backend](backend.md)
- Looks up peer public/local endpoints via `FindServerByAddress` /
  `FindServerByLocalAddress` (data originally from [QR](qr.md) heartbeats)

## Firewall note

NAT punch also needs a wide UDP range open on the host (see [Setup](../setup.md)).
Only opening `27901/udp` is not enough for actual gameplay traffic between Wiis.

## Related

- [Browser](browser.md) - join path that may seed NATNEG cookies
- [QR](qr.md) - host public/local fields used for lookups
- [Backend](backend.md) - NATNEG registry + server list
