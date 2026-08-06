# HTTP Proxy

Optional reverse proxy when NAS is not on port 80. Retail Wiis expect NAS on
that port. If you keep NAS on another port (for example 9000), this forwards
Nintendo NAS hostnames to [NAS](nas.md) so you do not need Apache or Nginx.

Package: `internal/proxy`  
Config: CLI `--proxy-bind` (default empty / disabled)  
Protocol: HTTP over TCP

## What it is

An optional host-based reverse proxy that sits in front of [NAS](nas.md). When
enabled (for example `--proxy-bind :80` with NAS on 9000), it accepts HTTP on
that address and forwards only Nintendo NAS hostnames to the local NAS backend.

## Why it exists

The default config puts NAS on port 80 directly. Use this proxy only when you
want NAS on a different port and still need a front door on 80 (or another
listen address) without Apache or Nginx.

Binding port 80 usually needs `sudo` or `CAP_NET_BIND_SERVICE`.

## What it forwards

Only these Host values are proxied:

- `naswii.nintendowifi.net`
- `nas.nintendowifi.net`

Any other Host gets `404 unhandled host`.

Mario Kart Wii sends the `Host` header twice. Go's `net/http` rejects that with
`400`, which shows up in-game as **error 23400**. Both this proxy and NAS wrap
their listeners with `internal/httpfix` to drop the duplicate before parsing.

Keep-alive is supported: every request header block is rewritten, not only the
first on the connection. Without that, a second POST on the same TCP socket
(duplicate `Host` again) gets HTTP 400 from Go.

With `[Logging] DumpFile` set to a path, the wrapper writes verbose raw
`accept` / `recv` / `send` records to that file (not stderr) and still warns
on stderr if the client opens with a TLS handshake (typical cause of **error
20100** when NoSSL is not running).

## What the Wii does

With DNS pointing `*.nintendowifi.net` (or at least the NAS names) at your
server IP, the Wii opens `http://naswii.nintendowifi.net/...` on port 80. With
the proxy enabled, that request is rewritten to the configured NAS backend and
the NAS response is returned.

## When to skip it

Default: leave `--proxy-bind` empty and run NAS on port 80
(`[NasServer] Port = 80`). That is the usual path for retail clients.

## Health check

With NAS on 80 (default):

```shell
curl -H "Host: naswii.nintendowifi.net" http://127.0.0.1/
# -> ok
```

With the proxy in front of a non-80 NAS backend, the same curl hits the proxy
listen address instead.

## Related

- [NAS](nas.md) - the backend this proxies to when enabled
- [Setup](../setup.md) - firewall and DNS for port 80
