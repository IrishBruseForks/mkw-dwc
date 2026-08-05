# HTTP Proxy

Optional reverse proxy on port 80. Retail Wiis expect NAS on that port. This
forwards Nintendo NAS hostnames to [NAS](nas.md) on 9000 so you do not need
Apache or Nginx.

Package: `internal/proxy`  
Config: CLI `--proxy-bind` (default empty / disabled)  
Protocol: HTTP over TCP

## What it is

An optional host-based reverse proxy that sits in front of [NAS](nas.md). When
enabled (typically `--proxy-bind :80`), it accepts HTTP on that address and
forwards only Nintendo NAS hostnames to the local NAS backend (port 9000).

## Why it exists

Retail Wiis expect NAS on port 80 (via the spoofed hostname). Running NAS itself
on 9000 keeps the backend simple. The proxy bridges that gap without Apache or
Nginx.

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
server IP, the Wii opens `http://naswii.nintendowifi.net/...` on port 80. The
proxy rewrites the request to `http://127.0.0.1:9000/...` and returns the NAS
response.

## When to skip it

If you terminate HTTP yourself (another reverse proxy, or clients that can hit
`:9000`), leave `--proxy-bind` empty. Then open `9000/tcp` to clients instead.

## Health check

```shell
curl -H "Host: naswii.nintendowifi.net" http://127.0.0.1/
# -> ok  (same body as NAS GET /)
```

## Related

- [NAS](nas.md) - the backend this proxies to
- [Setup](../setup.md) - firewall and DNS for port 80
