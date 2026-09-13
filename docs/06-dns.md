# 06 — DNS

**Needs:** `projects[].base_domain`, `projects[].preview_domain`,
`projects[].name`, `certificates.challenge_delegation_zone`, the host's tailnet IP
(`tailscale ip -4`)

Each project needs two records in its own zone. The user adds them — an agent
should list them and wait, not edit DNS.

| Name | Type | Value | Proxy |
| --- | --- | --- | --- |
| `*.preview` | A | the host's tailnet IP, e.g. `100.101.102.103` | DNS only |
| `_acme-challenge.preview` | CNAME | `<name>-preview.acme.<delegation zone>` | DNS only |

Both are separate records. Adding the CNAME must not replace the wildcard
([pitfalls](pitfalls.md#adding-the-challenge-cname-must-not-replace-the-wildcard)).

Notes:

- **The A record is public but useless outside the tailnet.** `100.64.0.0/10`
  addresses aren't routable on the internet; to anyone else the name resolves and
  goes nowhere. If publishing it bothers you, use Tailscale's split DNS with your
  own nameserver instead — at the cost of testers needing MagicDNS on.
- **DNS only.** On Cloudflare, an orange-cloud (proxied) record sends traffic to
  Cloudflare, which can't reach a tailnet address.
- **The CNAME target doesn't exist** until Traefik writes a TXT record there during
  issuance. That's expected; lego follows the CNAME regardless.
- **Unique target per project** (`<name>-preview`), so two projects renewing at
  the same moment don't overwrite each other's challenge.
- With `access.mode: public`, the A record holds the public IP instead.

**Verify** against the zone's authoritative nameserver, so caches don't lie:

```sh
NS=$(dig +short NS myapp.com | head -1)
dig +short @"$NS" anything.preview.myapp.com A              # the tailnet IP
dig +short @"$NS" _acme-challenge.preview.myapp.com CNAME   # the delegation target
```

A resolver may keep answering NXDOMAIN for up to 30 minutes after a mistake is
fixed ([pitfalls](pitfalls.md#resolvers-remember-the-outage)).

## Smoke test: a certificate, end to end

Before wiring up a real project, start a throwaway container that claims a
preview hostname exactly the way a project's workflow will:

```sh
D=preview.myapp.com
docker run --detach --name smoke --network previews \
  --label traefik.enable=true \
  --label "traefik.http.routers.smoke.rule=Host(\`smoke.$D\`)" \
  --label traefik.http.routers.smoke.entrypoints=websecure \
  --label traefik.http.routers.smoke.tls.certresolver=acme \
  --label "traefik.http.routers.smoke.tls.domains[0].main=*.$D" \
  traefik/whoami
docker compose -f ~/self-hosted-pr-previews/templates/proxy/compose.yaml logs -f traefik
```

Issuance takes one to two minutes. Then, from a device on the tailnet:

```sh
curl -v https://smoke.preview.myapp.com/ 2>&1 | grep -E 'subject|issuer|Hostname'
```

The certificate's subject is `*.preview.myapp.com`. Remove the container with
`docker rm -f smoke`; the certificate stays in Traefik's store and is reused by
every preview of the project.

If issuance fails with *secondary validation* errors while every DNS check above
is clean, see [pitfalls](pitfalls.md#lets-encrypt-secondary-validation-failures-come-and-go)
before retrying.
