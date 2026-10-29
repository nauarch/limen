# Security Policy

## Reporting a vulnerability

If you find a security vulnerability in limen, please report it privately.

**Do not open a public issue.** Use
[GitHub's private vulnerability reporting](https://github.com/nauarch/limen/security/advisories/new)
instead, so the details stay private until a fix is out.

Critical issues are fixed and released as soon as possible.

## Threat model

limen is built for a trusted LAN. These are choices, not vulnerabilities:

- **No TLS and no authentication.** Anyone who can reach limen can pull through it, start
  prefetches and read its metrics. Keep it on a network you trust, or put a TLS proxy in front.
- **The setup scripts trust the network.** `curl … | sudo sh` and `irm … | iex` run what
  arrives over plain HTTP. The SHA-256 check catches a corrupted download, not someone on the
  path who can rewrite both the script and the binary. Where that matters, download limenctl from
  a release and check it against `SHA256SUMS`.
- **Clients are known by address.** `GET /api/clients/me` tells an address only about itself;
  hosts behind one NAT share a count.

## Scope

Reports are welcome on anything that breaks these guarantees:

- **Cache integrity.** A blob is stored only after its content matches its digest, and a manifest
  asked for by digest only when it matches; a tag is the only thing limen trusts upstream to name.
  Any way to make limen store or serve content under a digest it does not match is in scope.
- **Upstream reach.** limen fetches only from the registries it is configured to serve, at their
  own API hosts or the `--upstream` URLs its operator gave. Any request that makes it fetch from
  elsewhere is in scope.
- **The store.** Repository names and tags are checked against the OCI distribution grammar, and
  blobs are stored by digest, so no request names a path outside the store. limen refuses to start
  on a folder without its marker file, so it never writes into the wrong one.
- **Private images.** limen pulls anonymously; an image that needs credentials is refused to
  limen, and the client pulls it with its own. Anything that lets one client's credentials, or a
  private image, reach the cache is in scope.
- **The setup scripts.** The address limen writes into the scripts comes from the request's
  `Host` header and must be a plain `host:port`. Any way to inject into a script is in scope.
- **limenctl on a host.** It runs as root on Linux and changes Docker's configuration. It writes
  only the files it lists in its dry run, backs up every file it replaces, and runs only the
  hooks the host's owner named in `/etc/limenctl.json` by absolute path. Anything that makes it
  write, remove or run something else is in scope.
- **Exhaustion.** The store keeps under `--max-size`, and the per-client record under a fixed
  number of addresses. Ways around those bounds are in scope.

## Supported versions

Only the latest release receives security fixes.
