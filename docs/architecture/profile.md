# Profile (GPCM)

GameSpy login after Nintendo login. The Wii shows the NAS auth token here and
gets a GameSpy session (profile ID, session key) so it can use matchmaking.

Package: `internal/gamespy/profile`  
Config: `[GameSpyProfileServer]` (default `0.0.0.0:29900`)  
Protocol: GameSpy text wire over TCP  
Hostname: `gpcm.gs.nintendowifi.net`

## What it is

GPCM (GameSpy Connection Manager / profile server) is the GameSpy login service.
After [NAS](nas.md) issues an authtoken, Mario Kart Wii connects here to prove
that token and receive a GameSpy session (`sesskey`, `profileid`, login ticket).

This is **not** a web page. Clients speak `\key\value\final\` messages on TCP
port 29900.

## Why it exists

NAS proves "Nintendo account OK". Profile proves "GameSpy identity OK" and ties
that identity to a profile ID / friend-code style nick in the [account store](database.md).
Room listing and matchmaking expect a logged-in GameSpy session.

## Connection flow

1. Server sends `\lc\1\challenge\<10 letters>\id\1\final\`
2. Client sends `\login\` with `authtoken`, its own `challenge`, and `response`
3. Server recomputes the expected response from NAS challenge + tokens
4. On success: `\lc\2\` with `sesskey`, `proof`, `userid`, `profileid`, `uniquenick`, `lt`
5. Client may send `\updatepro\` (store firstname/lastname) and `\getprofile\`
   (server replies `\pi\`)
6. Keep-alives: `\ka\` echoed as `\ka\\final\`
7. `\logout\` deletes the session and closes the connection

Invalid authtoken or bad response returns a GameSpy `\error\` message.

## Shared state

- Reads **`nas_logins`** to validate the authtoken
- Creates / updates **`users`** via login-from-auth
- Creates **`sessions`** (`sesskey`, `loginticket`)

## Commands implemented

| Client command | Behavior |
|----------------|----------|
| `login` | Validate NAS token + crypto response, open session |
| `getprofile` | Reply `\pi\` with profile fields (`nick`, `email`, `sig`, ...) |
| `updatepro` | Persist profile fields (`firstname`, `lastname`, ...) |
| `status` | Accept online status (friends broadcast not implemented yet) |
| `ka` | Keep-alive reply |
| `logout` | Delete session, close TCP |

Unknown commands are logged and ignored.

## Related

- [NAS](nas.md) - issues the authtoken used here
- [Database](database.md) - users and sessions
- [GPSP](gpsp.md) - friend lookup during WFC connect (right after profile login)
- [QR](qr.md) / [Browser](browser.md) - used after a successful profile login
