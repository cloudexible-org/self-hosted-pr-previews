# 12 — Operations

Day-to-day running, and the checks that prove the setup survives being left
alone.

## The reboot test

The setup isn't done until this passes. Restart the machine and **don't log in**
(on Windows, leave it at the lock screen). After a few minutes, from another
device:

- the runners show **Idle** in GitHub;
- an existing preview loads;
- the dashboard loads.

If the runners are offline on WSL2, check the keepalive task's last result in Task
Scheduler ([01](01-host.md)).

## Monitoring

A preview host fails quietly: nobody notices a dead runner or a failing
certificate renewal until a PR's preview doesn't appear. Each project carries
[`templates/workflows/preview-health.yml`](../templates/workflows/preview-health.yml),
which runs every six hours on **GitHub-hosted** runners — a check on the host
can't report that the host is down — and fails when:

- **a job has waited over 45 minutes for the self-hosted runner.** The daily
  cleanup run in `preview.yml` guarantees there is a job to wait, even with no PR
  activity.
- **the project's wildcard certificate expires within 21 days.** Traefik renews at
  30 days, so this means a week of failed renewals. Previews aren't reachable from
  GitHub, so expiry comes from certificate-transparency logs (Cert Spotter's
  public API).

GitHub emails a failed scheduled run to whoever last edited that workflow's
schedule. No tokens are involved: the repository's own `GITHUB_TOKEN` can read its
runs' jobs, and neither check needs org-level runner permissions. Cost is a few
minutes of hosted-runner time a month per project.

**Verify:** run it once by hand (Actions → Preview health → Run workflow). To see
it fail, stop a runner service for an hour and push to a PR.

What it doesn't catch: a full disk, or a broken app inside a running container.
The weekly prune ([03](03-docker.md)) handles the first; testers notice the second.

## Adding a project

1. Add it to `config.yaml`.
2. DNS records and the smoke test for its preview domain ([06](06-dns.md)).
3. Its workflow, environment and secrets ([07](07-project-workflow.md),
   [08](08-secrets.md)), backend ([09](09-backend-previews.md)) and auth
   ([10](10-auth.md)).

Nothing on the proxy changes. A project in a different GitHub org needs its own
runner ([02](02-github-runner.md)).

## Adding or removing a tester

Tailscale admin console: invite or remove the user, and update `group:testers`
([04](04-private-network.md#inviting-a-tester)). Removing someone from the tailnet
cuts their access to every preview at once.

## Updating

- **Runners** update themselves.
- **Host packages:** `apt-get update && apt-get upgrade` — Docker and Tailscale
  come from their own repositories. A Docker upgrade restarts running containers.
- **Proxy and dashboard:** pull this repository, then
  `docker compose up --detach --build --pull always` in `templates/proxy`. Previews
  are unreachable for the few seconds Traefik restarts; their containers keep
  running.
- **Workflow actions:** Dependabot on each project keeps the action versions in
  its copy of `preview.yml` current. In this repository, Dependabot covers the
  dashboard's Go and base images, the proxy template's images and its own CI —
  but not `templates/workflows/`, which it doesn't scan; bump those by hand when
  a project's Dependabot bumps its copy.

## Disk

```sh
docker system df
```

Build cache is pruned weekly by the timer ([03](03-docker.md)). Images of closed
PRs are removed by teardown.

If the host was down when a PR closed, its teardown job waits in the queue, and
GitHub drops queued jobs after 24 hours. The daily run of `preview.yml` catches
these: it lists the PR numbers that have an image or container on the host but
aren't open, and runs the same teardown for each — container, backend preview,
image. Trigger it by hand with Actions → Preview → Run workflow.

A project whose workflow was deleted leaves its previews behind for good. Remove
them by hand, backend previews included.

## Certificates

Traefik renews them 30 days before expiry, with the same DNS token. They live in
the `preview-host_letsencrypt` volume. Losing it isn't a disaster — they are
issued again on demand — but don't delete it casually: Let's Encrypt allows 5
identical certificates a week.

Renewal fails silently on the host if the DNS token expires or is revoked;
`preview-health.yml` reports it three weeks before expiry. Check Traefik's log
after rotating the token anyway.

## Rotating credentials

| Credential | Where it lives | Rotate by |
| --- | --- | --- |
| DNS API token | `templates/proxy/.env` on the host | new token, edit `.env`, `docker compose up -d traefik` |
| Doppler service tokens | GitHub `preview` environments | the pipe command in [08](08-secrets.md), then revoke the old token in Doppler |
| Convex preview deploy key | the backend's preview secrets | regenerate in Convex, update the secret |
| Runner registration | the runner directory | `./config.sh remove`, then register again ([02](02-github-runner.md)) |

## Taking it down

For one project: delete its workflow, then the GitHub environment, DNS records,
and tokens. For the whole host: stop the runners' services and remove them in
GitHub, `docker compose down`, remove the device from the tailnet, and on WSL2
unregister the keepalive task and `wsl --unregister <distro>` — which deletes the
distro's disk.
