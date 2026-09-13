# Pitfalls

Everything here cost real time while building this setup. Each entry says what
you will see, why, and what to do. Read it once before starting, and again
before debugging anything.

## Host

### WSL shares one network

**Symptom:** a second `tailscaled` in the preview-host distro fails with
`TUN device tailscale0 is busy`, or two distros fight over the same port.

**Why:** every WSL2 distro runs in one VM and shares one network namespace. If
another distro (often the user's dev distro) already runs Tailscale, it owns
`tailscale0`. A service listening on a port in one distro holds that port for
all of them.

**Fix:** run the preview host's `tailscaled` with
`--tun=userspace-networking` ([template](../templates/host/tailscaled.default)).
It creates no device and still delivers inbound tailnet connections to local
listeners. Keep preview ports (443, and anything a container publishes) off
ports the dev distro uses.

### WSL stops idle distros

**Symptom:** the runner goes offline a minute after the last terminal closes,
even though its systemd service is enabled.

**Why:** WSL shuts a distro down once no Windows process is attached to it;
services inside don't count.

**Fix:** the [keepalive scheduled task](../templates/host/keepalive-task.ps1)
holds the distro open from boot, with nobody logged in.

### `wsl.exe` runs your command through another shell

**Symptom:** `$(...)`, `$VAR` or quotes inside `wsl -d distro -- bash -c "..."`
expand to nothing or break, even inside single quotes.

**Why:** `wsl.exe` joins its arguments and hands them to the distro's default
shell, which parses them again before `bash -c` ever sees them.

**Fix:** pipe the script instead: `'...script...' | wsl -d distro -- bash -s`.
Watch for commands inside that script that read stdin (`docker compose exec`,
`docker run -i`) — give them `</dev/null`, or they swallow the rest of it.

### Windows can't reach the distro's files

**Symptom:** `\\wsl.localhost\<distro>\...` returns "network name cannot be
found" once the distro has interop and automount disabled.

**Fix:** move file contents through stdin, e.g. base64 into
`bash -c "echo ... | base64 -d > file"`, or `tar` piped into `wsl`. PowerShell
7.4+ pipes bytes between native commands intact; earlier versions don't.

### pnpm needs `libatomic1`

**Symptom:** `pnpm/action-setup` fails with
`libatomic.so.1: cannot open shared object file`.

**Why:** it installs pnpm's standalone binary, which links against libatomic.
GitHub's hosted runners ship it; a minimal Ubuntu doesn't.

**Fix:** `apt-get install libatomic1` on the host ([01](01-host.md)).

### Two runners under one user race on `~/setup-pnpm`

**Symptom:** intermittent pnpm install failures when two projects' previews
build at once.

**Why:** runners for different orgs on one host usually run as the same user,
and `pnpm/action-setup` installs to `~/setup-pnpm` by default.

**Fix:** `with: { dest: ${{ runner.temp }}/setup-pnpm }` in every workflow.

### Docker's build cache grows without bound

**Symptom:** tens of gigabytes under `/var/lib/docker` after a few weeks.

**Why:** every preview build adds cache. `docker builder prune` without `--all`
removes only dangling records, and `--max-used-space` removed nothing on
Docker 29's default driver.

**Fix:** the [weekly prune timer](../templates/host/docker-builder-prune.service):
`docker builder prune --all --force --filter until=72h`. Also set log rotation
([daemon.json](../templates/host/daemon.json)); it applies only to containers
created afterwards.

## Network and certificates

### Tagged Tailscale devices aren't users

**Symptom:** after replacing Tailscale's default allow-all policy, one machine
can no longer reach the preview host, even though its owner is in the allowed
group.

**Why:** a device with a tag (for example `tag:server`) loses its user identity.
Grants for `group:admins` don't cover it.

**Fix:** add a grant with the tag as the source, and add a `tests` block to the
policy so the admin console refuses a policy that locks anyone out
([04](04-private-network.md)).

### Adding the challenge CNAME must not replace the wildcard

**Symptom:** every preview hostname returns NXDOMAIN right after DNS "worked".

**Why:** when adding `_acme-challenge.preview` as a CNAME, it is easy to edit the
existing `*.preview` A record into it instead of adding a new record.

**Fix:** a project's zone needs *both* records ([06](06-dns.md)). Check with
`dig` against the zone's authoritative nameserver, not a cache.

### Resolvers remember the outage

**Symptom:** a hostname resolves from `1.1.1.1` but not from your machine, for up
to half an hour after a DNS mistake is fixed.

**Why:** resolvers cache "does not exist" answers for the zone's SOA minimum TTL
(1800 s on Cloudflare).

**Fix:** wait, or test with `curl --resolve host:443:<ip>` to bypass DNS.

### Let's Encrypt "secondary validation" failures come and go

**Symptom:** `During secondary validation: DNS problem: networking error looking
up TXT for _acme-challenge...`, for every name in a zone, on both staging and
production, while every DNS check you run is clean.

**Why:** Let's Encrypt validates from several networks; one of its remote vantage
points was failing. It cleared on its own within a day.

**Fix:** don't burn the production limit (5 failed validations per hostname per
hour). Retry against staging (`lego --server letsencrypt-staging`) until it
passes, then let Traefik request the real certificate.

### Router and container names are global

**Symptom:** a preview for project B's PR 12 replaces project A's PR 12, or
Traefik logs router conflicts.

**Fix:** every container, router and service name carries the project prefix
([07](07-project-workflow.md)).

### Mount `/var/run/tailscale`, not the socket

**Symptom:** after a reboot Traefik can't get the dashboard's Tailscale
certificate, and `/var/run/tailscale/tailscaled.sock` is a directory.

**Why:** Docker may start Traefik before `tailscaled` creates its socket; a bind
mount of a missing file makes Docker create a directory in its place.

**Fix:** mount the directory ([compose template](../templates/proxy/compose.yaml)).

### Cloudflare's free certificate covers one subdomain level

Only relevant if you consider proxying previews through Cloudflare (a Tunnel or
orange-cloud records) instead of terminating TLS on the host: Universal SSL
covers `*.example.com` but not `*.preview.example.com`. This setup keeps DNS
records **DNS-only** and gets its own wildcard from Let's Encrypt, so the limit
never applies.

## Workflows and secrets

### Nested `doppler run` leaks the outer project

**Symptom:** `This token does not have access to requested project '<other>'`
from a freshly created, correct service token.

**Why:** `doppler run` exports `DOPPLER_PROJECT` and `DOPPLER_CONFIG` into the
command it runs. A second `doppler run` inside reads them as its own settings
and asks for the outer project with the inner token.

**Fix:** give the inner one `--project` and `--config` explicitly — or avoid
nesting ([08](08-secrets.md)).

### A build script wrapped in `doppler run` breaks inside Docker

**Symptom:** `doppler: not found` during `docker build`.

**Why:** projects often wrap `dev`/`build` scripts in `doppler run` for local
development. The image has no Doppler CLI and needs none — build values arrive as
build args.

**Fix:** have the Dockerfile run the underlying commands, not the wrapped script.

### Vercel still builds PR branches

**Symptom:** none, until it bites. Moving a project that deployed straight from
`main` to pull requests means Vercel starts building every pushed branch — and a
build that runs `convex deploy` uses whatever deploy key sits in Vercel's Preview
scope. If that is the production key, unreviewed backend code reaches production.

**Fix:** make each app's `vercel.json` skip non-main branches:

```json
"ignoreCommand": "[ \"$VERCEL_GIT_COMMIT_REF\" != \"main\" ] || (cd ../.. && npx turbo-ignore <app>)"
```

## Backend previews (Convex)

### Seeds run only when the preview is created

**Symptom:** a re-run deploy leaves the preview unseeded, or missing an
environment variable you have since added.

**Why:** `npx convex deploy --preview-run` is skipped when the preview deployment
already exists, and project default environment variables are copied only at
creation. A first deploy that fails *after* Convex created the deployment leaves
it half-built.

**Fix:** delete that preview deployment in the Convex dashboard, then re-run
([09](09-backend-previews.md)).

### Required environment variables block the first push

**Symptom:** `MissingEnvironmentVariables: ... Required environment variables are
not set: POSTHOG_PROJECT_TOKEN`.

**Why:** `convex.config.ts` can declare app env vars as required
(`v.string()`), and components such as PostHog's do.

**Fix:** set them in the project's **preview** default environment variables — a
placeholder is fine for analytics keys, and keeps previews out of product
analytics.

### A preview deploy key can delete a deployment but not find it

**Symptom:** you want teardown to delete `pr-<number>`'s backend, but the
Management API's lookup-by-reference and list endpoints reject a preview deploy
key.

**Fix:** record the deployment's URL as a label on the image at build time;
teardown reads it and calls `POST /v1/deployments/{name}/delete`, which does
accept the preview deploy key ([template](../templates/workflows/preview.yml)).

## Auth

### WorkOS rejects wildcards on public-suffix domains

A wildcard redirect URI must use a domain you own: `*.vercel.app`, `*.ngrok-free.app`
and `*.ts.net` are refused. The wildcard also matches exactly one subdomain level
and must be the leftmost label — `https://*.preview.example.com/callback` works;
`https://*.example.com` does not match `pr-1.preview.example.com`
([10](10-auth.md)).

### Clerk previews use the development instance

A production Clerk instance only accepts its own domain, so previews authenticate
against the development instance. If users reach the backend through Clerk's
`user.created` webhook, a preview never receives it — only accounts written by
the preview's seed can sign in.
