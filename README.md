# limen

A pull-through cache for container registries on your LAN, in the spirit of
[lancache](https://lancache.net/): pull an image once from the internet, and every other
machine on the network gets it from limen. Clients keep their image names (`nginx`,
`ghcr.io/org/app`); limen sits in front of the registries as a mirror, and when it is down
or cannot serve something, clients fall back to the real registry on their own.

- **Transparent.** Nothing changes in compose files or manifests. Docker, containerd and Talos
  all have mirror settings; limen is just the endpoint.
- **Many registries.** Docker Hub, ghcr.io, quay.io, registry.k8s.io, gcr.io, lscr.io,
  mcr.microsoft.com, public.ecr.aws, nvcr.io, docker.elastic.co and factory.talos.dev by default.
- **Safe to lose.** Blobs are checked against their digests; tags are rechecked upstream after
  a minute, and served stale only when the registry is down. If limen dies, pulls go direct
  ([at a cost](#when-limen-is-down)).
- **limenctl** points Docker hosts at limen, undoes it, checks it worked, and moves hosts from
  Docker's classic image store to the containerd store (which is what lets Docker use a mirror
  for registries other than Docker Hub), with a dry run, a journal and a rollback.
- **No dependencies.** Two static Go binaries, standard library only.

## How clients reach it

| Client | Mechanism | Registries |
|---|---|---|
| Docker Engine, containerd image store | `/etc/docker/certs.d/<registry>/hosts.toml`, read on every pull | all |
| Docker Engine, classic image store | `registry-mirrors` in `daemon.json` | Docker Hub only (`limenctl migrate` moves the host to the containerd store) |
| Docker Desktop (Windows) | `%USERPROFILE%\.docker\certs.d\<registry>\hosts.toml`, copied into Desktop's VM when it starts | all |
| containerd, Kubernetes, Talos | the runtime's own mirror settings ([guide](#talos-kubernetes-and-containerd)) | all |

containerd tells a mirror which registry it wants with `?ns=<registry>`; limen fetches from that
registry. A request limen cannot answer (a registry it does not serve, an image it cannot fetch)
gets an error, and the client goes to the registry itself.

### When limen is down

A limen that refuses connections costs nothing: the client goes direct at once. A limen host
that is off costs more, because each pull first waits for its connection to fail. Measured with
Docker 29.9.2: about 29 s per pull on the containerd store and 6 s on the classic store when the
host is on the same subnet, and about 2 minutes when its address is routed but never answers.
Run limen on a host that stays up.

## Quick start

### 1. Run limen

On a Linux host with systemd, as a service (a dry run without `--yes`):

```sh
sudo ./limen install --max-size=200G --yes
```

It copies itself to `/usr/local/bin`, runs as its own `limen` user with the store in
`/var/lib/limen`, and restarts on failure; a systemd watchdog restarts it if it ever hangs.
`sudo limen uninstall --yes` removes the service and keeps the store.

Or in Docker:

```yaml
services:
  limen:
    image: ghcr.io/nauarch/limen:latest
    command: ["--init", "--max-size=200G"]
    ports: ["5000:5000"]
    volumes: ["limen:/var/lib/limen"]
    restart: unless-stopped
volumes:
  limen:
```

The image has a health check built in. Give limen a fixed IP and point clients at the IP, not a
name: a host whose DNS runs in a container should not need DNS to pull.

### 2. Point hosts at it

limen hands out limenctl itself, so a host needs nothing installed. On Linux (as root):

```sh
curl -fsSL http://10.0.0.5:5000/setup | sudo sh -s -- plan            # report only
curl -fsSL http://10.0.0.5:5000/setup | sudo sh -s -- configure --yes
curl -fsSL http://10.0.0.5:5000/setup | sudo sh -s -- verify
```

On Windows with Docker Desktop (PowerShell, no admin needed):

```powershell
irm http://10.0.0.5:5000/setup.ps1 | iex                                            # report only
& ([scriptblock]::Create((irm http://10.0.0.5:5000/setup.ps1))) configure --yes
& ([scriptblock]::Create((irm http://10.0.0.5:5000/setup.ps1))) verify
```

The scripts download limenctl for the machine, check its SHA-256, run it with `--limen` set to
the limen they came from, and delete it afterwards. limenctl is also on the
[releases page](../../releases).

## limenctl

Every command that changes something prints exactly what it would do, and does it only with
`--yes`.

| Command | Does |
|---|---|
| `plan` | Reads the host: Docker version and image store, compose projects, containers not from compose, used and unused images, free space, DNS, and how it stands towards a limen. Findings come as info, warn, or block (what stops a migration). Changes nothing. |
| `configure` | Writes a `hosts.toml` for every registry limen serves. On the classic store it also puts limen first in `daemon.json`'s `registry-mirrors` and restarts Docker. Files it did not write are left alone unless `--force`; every replaced file is backed up. |
| `unconfigure` | Removes what configure wrote, restoring a file `--force` replaced from its backup. |
| `verify` | Pulls `hello-world`, asks limen how many requests from this host it counted meanwhile, and removes the image again if the host did not have it. Exits 1 when the pull went around limen. |
| `migrate` | Moves a classic-store host to the containerd image store (below). |
| `rollback` | Moves a migrated host back. Its old containers and images are still there. |
| `cleanup-old-store` | Deletes the classic store's data once you are happy. After this there is no rollback. |
| `version` | Prints the version. |

On Docker Desktop, `configure` and `unconfigure` work on the files under `%USERPROFILE%\.docker`
and restart Desktop (`docker desktop restart`), which reads them only when it starts. `migrate`
and its relatives refuse: Desktop switches image stores in its own settings.

### Migrating to the containerd image store

Docker's classic image store sends only Docker Hub pulls to a mirror. The containerd store reads
`hosts.toml` for every registry, and changing it needs no restart. `limenctl migrate --limen URL`
does the move in this order:

1. Saves the images built on the host (no registry has them) to a file, to load back later.
2. Optionally (`--prune`) removes images no container uses.
3. Has limen fetch every image the running projects use, for the host's platform. Nothing has
   stopped yet; if limen cannot fetch them, the migration stops here.
4. Stops the compose projects, those serving the host's DNS last.
5. Writes `daemon.json` (containerd store on, limen as mirror) and the `hosts.toml` files, and
   restarts Docker.
6. Loads the saved images, then pulls each project's images and starts it, DNS first. Their
   layers come from limen at LAN speed. The pull happens before a project's start hook, while its
   stop hook's arrangements still hold: on the containerd store a pull also asks the registry
   itself for the image's referrers, which needs the host to resolve names.

Each stage is recorded in `/var/lib/limenctl/migration.json`, so a migration that fails half way
can be rolled back. One that stopped after Docker came back on the new store can instead be
finished: `migrate` again only pulls and starts the projects. Volumes and bind mounts are untouched; data a container wrote outside its
volumes stays in the old store (`plan` warns about it). `plan` blocks a migration under swarm,
user-namespace remapping, too little disk, or without `docker compose` (v2).

Containers not from compose are refused unless `--force`: they stop at the switch, limenctl does
not recreate them, and only a rollback brings them back.

If a project must stop or start through its own scripts (say, a DNS server that swaps
`resolv.conf` around its restart), name them in `/etc/limenctl.json`:

```json
{"projects": {"dns": {"stop": "/srv/dns/stop.sh", "start": "/srv/dns/start.sh"}}}
```

limenctl runs each as a program of its own, in the project's folder, so each must be executable
(`chmod +x`); `plan` and `migrate` refuse hooks that are missing or are not.

## Talos, Kubernetes and containerd

These runtimes have mirror settings of their own, so limenctl is not needed. **Not yet tested
with limen**: they use containerd's resolver, the same one Docker's containerd store uses (which
is tested), so they are expected to send `?ns=` and fall back the same way.

Talos, as a machine config patch (one entry per registry; Talos falls back to the registry
unless `skipFallback` is set):

```yaml
machine:
  registries:
    mirrors:
      docker.io:
        endpoints: [http://10.0.0.5:5000]
      ghcr.io:
        endpoints: [http://10.0.0.5:5000]
      registry.k8s.io:
        endpoints: [http://10.0.0.5:5000]
```

containerd (including most Kubernetes nodes): set `config_path = "/etc/containerd/certs.d"` for
the CRI registry in `config.toml`, and write `/etc/containerd/certs.d/<registry>/hosts.toml`:

```toml
server = "https://ghcr.io"

[host."http://10.0.0.5:5000"]
  capabilities = ["pull", "resolve"]
```

For Docker Hub the server is `https://registry-1.docker.io`.

## Running limen

| Flag | Default | |
|---|---|---|
| `--dir` | (required) | The store's folder. `--init` makes it, with the marker file limen checks so it never writes into the wrong folder. |
| `--addr` | `:5000` | Where to listen. |
| `--max-size` | none | The store's cap, such as `200G`; least recently used blobs are evicted beyond it. |
| `--registries` | see above | Comma-separated registries to serve. |
| `--upstream` | | `name=url`: reach a registry somewhere other than `https://<its API host>`. |
| `--tag-ttl` | `1m` | How long a tag is answered without asking upstream. |
| `--answer-within` | `15s` | How long a pull waits for upstream before failing, so the client can fall back. |
| `--stall-after` | `1m` | How long a download may receive nothing before it is given up. |
| `--resolver` | system | A DNS server (`host:port`) to look registries up with. |
| `--self-check` | `30s` | How often limen asks its own `/healthz`, exiting after 3 failures so its supervisor restarts it. |

Endpoints besides the mirror (`/v2/`):

| Endpoint | |
|---|---|
| `GET /healthz` | 200 when limen can serve. |
| `GET /metrics` | Prometheus counters: blob hits, misses and joined downloads, bytes served and fetched, tag checks, evictions, store size, and `limen_build_info`. Counters start at 0 when limen starts. |
| `GET /api/registries` | The registries served. |
| `POST /api/prefetch` | `{"images": ["nginx:1.27"], "platforms": ["linux/arm64"]}` starts fetching ahead; `GET /api/prefetch/<id>` follows it. |
| `GET /api/clients/me` | How many pull requests limen counted from the asking address (what `verify` uses). |
| `GET /setup`, `/setup.ps1`, `/get/<file>` | The setup scripts and the limenctl binaries. |

limen pulls anonymously. Images that need credentials are not cached: the registry refuses limen,
limen answers the client with an error (403), and the client pulls directly with its own
credentials, so private images never pass through it.

## Security

limen is built for a trusted LAN.

- **No TLS, no authentication.** Anyone who can reach limen can pull through it, start
  prefetches, and read its metrics. The store's cap bounds what they can make it hold. Keep it on
  a network you trust, or put a TLS proxy in front.
- **Content is verified.** Every blob is checked against its digest before it is stored, so a
  bad upstream answer cannot be cached and served.
- **The setup scripts trust the network.** `curl … | sudo sh` over plain HTTP runs what arrives.
  The SHA-256 check catches a corrupted download, not someone on the path who can rewrite both
  the script and the binary. Where that matters, download limenctl from a release and check it
  against `SHA256SUMS`.
- How to report a vulnerability, and what counts as one: [SECURITY.md](SECURITY.md).
- `/api/clients/me` tells an address only about itself. Behind NAT, hosts share an address, and
  `verify` counts their pulls together.

## What has been tested

| Platform | State |
|---|---|
| limen on Linux amd64 (systemd, Ubuntu 26.04) | in use: a cache for a fleet of Docker hosts |
| Docker Engine 29.9.2, containerd store, Linux amd64 | `configure`, `verify`, pulls from several registries |
| Docker Engine 29.9.2, classic store, Linux amd64 | `migrate`, `rollback`, `migrate` again, `cleanup-old-store` (lab); `migrate` on a real host |
| Docker Engine 29.2.1, Linux arm64 | `plan` |
| Docker Desktop 4.96 (engine 29.9.2), Windows 11 | `configure`, `verify`; docker.io, ghcr.io and quay.io through limen |
| Docker Desktop on macOS | not built, not tested |
| Talos, Kubernetes, plain containerd | not tested (see the guide) |

## Building

```sh
go generate ./internal/frontdoor/setup/dist   # limenctl for every platform limen hands out
go build ./cmd/limen ./cmd/limenctl
go test ./...
```

A limen built without `go generate` works, but serves no limenctl. The Dockerfile does both and
cross-compiles: `docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=v1.0.0 .`

## License

Copyright 2026 darki73. Licensed under Apache-2.0: see [LICENSE](LICENSE).

If you redistribute limen, or something built from it (a fork, a binary, an image, a product),
the licence asks you to keep the copyright headers, ship the [NOTICE](NOTICE), and say what you
changed. Please also link back to this repository.
