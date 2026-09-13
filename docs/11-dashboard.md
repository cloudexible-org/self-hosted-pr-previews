# 11 — Dashboard

**Needs:** `dashboard.*`

[`apps/dashboard`](../apps/dashboard) is a small Go server that lists every
preview on the host, grouped by project, with links to its pull request, commit
and Actions run, and streams any preview's container log live in the browser.
It also shows the logs of the proxy's own services, which is where certificate
problems show up.

It is deployed by the proxy's compose file ([05](05-proxy-and-certificates.md)),
so with `dashboard.enabled` true there is little left to do here.

## How it finds previews

Any container with `traefik.enable=true` that is either labelled
`preview.pr=<n>`, or routed to a hostname starting with `pr-<n>`. The `preview.*`
labels from [07](07-project-workflow.md#the-contract) add the project name,
repository, branch, commit and run link. A project that sets none still appears,
named after its preview domain.

## Security

The dashboard can read every container on the host, and containers hold secrets
in their configuration. It is built so that doesn't leak:

- **No Docker socket.** It talks to
  [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy), which
  allows only `GET` requests to container endpoints.
- **A private network to that proxy.** `docker-api` is an internal network only the
  dashboard and the proxy join. Preview containers, which run pull-request code,
  can't reach it.
- **Narrow decoding.** It reads only whether a container is running and uses a
  TTY; the environment in Docker's inspect response is never decoded, so it can't
  be rendered or logged.
- **Only previews' and the proxy's own logs**, even though the API would serve any
  container's.
- **Locked-down container:** read-only filesystem, no capabilities, no privilege
  escalation, non-root distroless image; strict Content-Security-Policy.

It has no login. Anyone who can reach port 443 on the host can open it — which,
with the policy in [04](04-private-network.md), means your testers. Container
logs can contain things testers shouldn't see. To keep it admin-only, give it its
own entrypoint on another port and grant that port only to admins:

```yaml
# traefik command
      - --entrypoints.dashboard.address=:8443
# traefik ports
      - "8443:8443"
# dashboard labels
      - traefik.http.routers.preview-dashboard.entrypoints=dashboard
```

## Certificate

It is served on the host's own tailnet name, `<node_name>.<tailnet>.ts.net`
(`dashboard.host`), with a certificate from the host's `tailscaled` through
Traefik's Tailscale resolver. That needs MagicDNS and HTTPS Certificates enabled
in the tailnet ([04](04-private-network.md)). No public DNS record is involved.

Note that issuing it publishes the tailnet name in certificate-transparency logs.

**Verify:** from a tailnet device, `https://<dashboard.host>/` lists the smoke
test container or a running preview, and its **Logs** page streams.

## Developing it

```sh
cd apps/dashboard
go test ./...
```

The Docker client speaks plain HTTP, not the Unix socket, so run it against a
socket proxy locally too:

```sh
docker run -d --name dp -p 127.0.0.1:2375:2375 -e CONTAINERS=1 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro tecnativa/docker-socket-proxy:v0.5.0
DOCKER_HOST=http://127.0.0.1:2375 go run .           # http://localhost:8080
```

| Variable | Default | |
| --- | --- | --- |
| `DOCKER_HOST` | `http://docker-proxy:2375` | Docker API, over HTTP |
| `INFRA_PROJECT` | — | compose project whose services are listed beside previews |
| `LISTEN` | `:8080` | listen address |

The image build runs `go vet` and the tests, so a failing test fails
`docker compose up --build`.
