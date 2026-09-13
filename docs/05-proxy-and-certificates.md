# 05 — Proxy and certificates

**Needs:** `certificates.*`, `proxy.*`, `dashboard.*`

One Traefik on the host serves every project. It watches Docker, routes
`pr-<n>.<preview_domain>` to the container that claims it in its labels, and
fetches a wildcard certificate for each project the first time one of its
previews appears. Nothing project-specific is configured on the proxy: adding a
project never touches it.

## How certificates work here

Previews are private, so Let's Encrypt can't reach them over HTTP. Certificates
are issued with the **DNS-01** challenge instead: Traefik proves control of
`*.preview.myapp.com` by publishing a TXT record at
`_acme-challenge.preview.myapp.com`.

That would normally need an API token that can edit `myapp.com`'s DNS — every
project's zone, possibly across several DNS accounts. Instead, each project
points its challenge record, once, at a zone that exists only for this:

```
_acme-challenge.preview.myapp.com.  CNAME  myapp-preview.acme.example-infra.net.
```

Let's Encrypt follows the CNAME, and Traefik (through lego) writes the TXT
record at the target. The host's token can edit only
`certificates.challenge_delegation_zone`, which holds nothing but challenge
records. A leaked token can't touch a project's website, mail or anything else —
and projects in separate DNS accounts never share credentials.

Pick any cheap domain you own for the delegation zone. It never serves traffic.

## Set up the DNS token (Cloudflare)

The user, in the Cloudflare account holding the delegation zone:

1. Add the zone if it isn't there.
2. My Profile → API Tokens → Create Token → **Custom token**:
   - Permissions: **Zone → Zone → Read** and **Zone → DNS → Edit**
   - Zone resources: **Include → Specific zone →** the delegation zone only
3. Save the token for the next step. Don't paste it into the conversation.

Other DNS providers work too: Traefik supports most of them through lego. Change
`dnschallenge.provider` in the compose file and the environment variables it
reads; the rest of this setup is unchanged.

## Deploy

On the host, as `host.user`, clone this repository — the compose file builds the
dashboard from it:

```sh
git clone https://github.com/cloudexible-org/self-hosted-pr-previews.git ~/self-hosted-pr-previews
cd ~/self-hosted-pr-previews/templates/proxy
cp .env.example .env && chmod 600 .env
```

The user fills `.env` in their own editor (`ACME_EMAIL`, `CF_DNS_API_TOKEN`,
`DASHBOARD_HOST`). If `proxy.*` differs from the defaults (`previews`,
`websecure`, `acme`), or the dashboard is disabled, edit
[`compose.yaml`](../templates/proxy/compose.yaml) to match and commit that on a
local branch so pulling updates stays easy.

```sh
docker network create previews        # proxy.network, once
docker compose up --detach --build
```

The `previews` network is created by hand so it outlives the compose project:
recreating the proxy must not disconnect running previews.

Traefik has `restart: unless-stopped`, and Docker starts on boot, so the proxy
comes back after a reboot on its own.

**Verify:**

```sh
docker compose ps                                   # traefik, dashboard, docker-proxy up
docker compose logs traefik | grep -i error         # nothing about providers or config
curl -sk -o /dev/null -w '%{http_code}\n' https://localhost/   # 404 — Traefik answers, no route yet
```

Certificates are verified end to end after DNS, in [06](06-dns.md).

## Before the first real certificate

Let's Encrypt allows 5 failed validations per hostname per hour, and 5 identical
certificates per week. Don't find out a DNS mistake by burning those. For the
first certificate of a new setup, point the resolver at staging by adding

```yaml
      - --certificatesresolvers.acme.acme.caserver=https://acme-staging-v02.api.letsencrypt.org/directory
```

run the smoke test in 06, then remove the line, delete the stored staging
account and certificates, and recreate Traefik:

```sh
docker compose down
docker volume rm preview-host_letsencrypt
docker compose up --detach
```
