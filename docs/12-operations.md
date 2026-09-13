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
  `preview.yml` current.

## Disk

```sh
docker system df
```

Build cache is pruned weekly by the timer ([03](03-docker.md)). Images of closed
PRs are removed by teardown. If the host was down when a PR closed, the teardown
job waits in the queue — GitHub drops queued jobs after 24 hours, which leaves an
orphan. Find them:

```sh
for c in $(docker ps -a --filter label=preview.pr --format '{{.Names}}'); do
  repo=$(docker inspect -f '{{index .Config.Labels "preview.repo"}}' "$c")
  pr=$(docker inspect -f '{{index .Config.Labels "preview.pr"}}' "$c")
  state=$(gh pr view "$pr" --repo "$repo" --json state --jq .state)
  [ "$state" = OPEN ] || echo "$c ($repo#$pr is $state)"
done
```

Then re-run that PR's teardown job from its Actions run, which also deletes its
backend preview — or remove the container and image by hand.

## Certificates

Traefik renews them 30 days before expiry, with the same DNS token. They live in
the `preview-host_letsencrypt` volume. Losing it isn't a disaster — they are
issued again on demand — but don't delete it casually: Let's Encrypt allows 5
identical certificates a week.

Renewal fails silently if the DNS token expires or is revoked. Check Traefik's
log after rotating it.

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
