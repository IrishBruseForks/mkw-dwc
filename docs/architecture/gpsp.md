# GPSP (GameSpy Player Search)

Friend lookup after GameSpy profile login. Mario Kart Wii queries here during
"Connecting to Nintendo WFC..." to resolve friend profile IDs to nicks, even when
the friend list is empty.

Package: `internal/gamespy/gpsp`  
Config: `[GameSpyPlayerSearchServer]` (default `0.0.0.0:29901`)  
Protocol: GameSpy text wire over TCP  
Hostname: `gpsp.gs.nintendowifi.net`

## What it is

GPSP (GameSpy Player Search) is a small GameSpy TCP service on port 29901.
After [Profile (GPCM)](profile.md) login succeeds, the Wii opens a short-lived
connection here and sends an `otherslist` request.

This is **not** a web page. Clients speak `\key\value\final\` messages on TCP
port 29901.

## Why it exists

Profile login proves identity, but the game still needs to finish the WFC connect
step. Mario Kart Wii calls GPSP during **"Connecting to Nintendo WFC..."** to
look up friend profile IDs (`opids`) and get matching `uniquenick` values.

A fresh license with no friends still sends `otherslist` (often `opids=0`). The
server must reply with a valid `\otherslist\` message ending in `\oldone\`, or
the client hangs on that screen even when NAS and profile already succeeded.

If `gpsp.gs.nintendowifi.net` is missing from DNS/hosts, it resolves to Nintendo's
old GameSpy IP and the TCP connect times out forever.

## Connection flow

1. Wii resolves `gpsp.gs.nintendowifi.net` and connects on TCP 29901 (after profile login)
2. Client sends `\otherslist\` with `sesskey`, `profileid`, `numopids`, `opids`
   (pipe-separated profile IDs, or `0` when empty), plus `namespaceid` and
   `gamename` (`mariokartwii`)
3. Server replies `\otherslist\`, then for each requested ID: `\o\<profileid>\uniquenick\<nick>\`
   (empty `uniquenick` if the profile is unknown)
4. Reply ends with `\oldone\` and `\final\`
5. Client closes the connection

Example reply shape for `opids=123|456`:

```
\otherslist\\o\123\uniquenick\alice\o\456\uniquenick\bob\oldone\\final\
```

Empty friend list (`opids=0`):

```
\otherslist\\o\0\uniquenick\\oldone\\final\
```

## Commands implemented

| Client command | Behavior |
|----------------|----------|
| `otherslist` | Map `opids` to `o` / `uniquenick` pairs, append `oldone` |

Unknown commands are logged and ignored.

## Shared state

- Reads **`users`** / profile records via `GetProfile` to fill `uniquenick`
- Does not create sessions or write to the store

## Related

- [Profile](profile.md) - GameSpy login must finish first
- [Database](database.md) - profile ID to nick lookup
- [NAS](nas.md) - earlier login step
