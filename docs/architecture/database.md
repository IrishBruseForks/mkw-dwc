# Account store (GPCM / JSON)

Where accounts live on disk. User IDs, login tokens, sessions, and bans.
Survives restarts. Room lists do **not** live here (those are the in-memory
[backend](backend.md)).

Package: `internal/database` (interface), `internal/database/json`  
Configured in `mkw-dwc.cfg` under `[Store]` (required, no CLI default):

```ini
[Store]
Type = "json" # "json"
Path = "data" # JSON data directory
```

Missing `[Store]`, `Type`, or `Path` is a startup error. Only `Type = "json"`
is supported today. The `database.Store` interface stays so a SQL (or other)
backend can be added later without changing NAS / profile callers.

## What it is

Persistent storage for Nintendo / GameSpy identity data. Created on first run
(`Initialize`). Unlike the [backend](backend.md) room list, this survives
restarts.

## Why it exists

[NAS](nas.md) and [Profile](profile.md) need durable userids, authtokens, and
GameSpy sessions. Ephemeral matchmaking stays in memory. Accounts do not.

## JSON layout (`Type = "json"`)

Under `Path` (example `data/`):

| File | Purpose |
|------|---------|
| `users.json` | Profile IDs, userids, passwords, uniquenick, console fields |
| `sessions.json` | Active GPCM sessions (sesskey, profileid, loginticket) |
| `nas_logins.json` | NAS authtoken -> login payload for later GPCM validation |
| `banned.json` | Per-game IP bans checked during NAS acctcreate/login |

Writes use temp-file + rename. Restarting reloads the same files.

## Who uses it

- **NAS**: allocate userid, store/fetch authtoken, ban checks
- **Profile**: validate authtoken, create user/session, logout cleanup

QR / browser / NATNEG do **not** use this store for rooms.

## Ops notes

- Paths are relative to the process working directory unless absolute
- Deleting the data directory forces fresh accounts

## Related

- [NAS](nas.md)
- [Profile](profile.md)
- [Backend](backend.md) - in-memory rooms (not persisted)
