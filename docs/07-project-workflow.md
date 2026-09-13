# 07 — Project workflow

**Needs:** `projects[]`, `github.runners[]`, `proxy.*`, `secrets.github_environment`

Once per project. This is the only runbook that changes a project's repository,
so it ends in a pull request the user reviews — and that pull request is also the
first preview.

## The contract

The project knows nothing about the host except this. Keep project docs and
comments to exactly these facts ([AGENTS.md](../AGENTS.md#4-rules)):

| | Value |
| --- | --- |
| Runner | `runs-on: { group: <runner_group>, labels: [self-hosted, linux] }` |
| Docker network | `previews` (`proxy.network`) |
| Entrypoint | `websecure` (`proxy.entrypoint`) |
| Certificate resolver | `acme` (`proxy.cert_resolver`), with `tls.domains[0].main=*.<preview_domain>` |
| Container and router name | `<project>-pr-<n>` |
| Image tag | `<project>-preview:pr-<n>` |
| Hostname | `pr-<n>.<preview_domain>` |
| Dashboard labels (optional) | `preview.project`, `preview.pr`, `preview.repo`, `preview.branch`, `preview.sha`, `preview.run-url` |

Names are global on the host, so the project prefix is not optional
([pitfalls](pitfalls.md#router-and-container-names-are-global)).

## 1. Let the repository use the runner

For an org runner group: org Settings → Actions → Runner groups → the group →
add the repository. Runner groups that allow public repositories are a risk; keep
that box off.

## 2. Make the app build as an image

The project needs a Dockerfile for the previewed app (`app.dockerfile`) that:

- serves on `0.0.0.0:<app.port>`, plain HTTP — Traefik terminates TLS;
- takes public build-time values as `ARG`s (`app.build_args`): the app's own URL,
  the backend URL, publishable keys. Anything in a client bundle is public; never
  pass a secret as a build arg;
- reads runtime secrets from the environment;
- doesn't run the project's `doppler run`-wrapped scripts
  ([pitfalls](pitfalls.md#a-build-script-wrapped-in-doppler-run-breaks-inside-docker)).

Check `.dockerignore` excludes `node_modules`, `.git`, local `.env*` files and
build output — the build context is the whole repository.

A static single-page app can be served by a small web server in the image (Caddy
or nginx) with a fallback to `index.html`. A server-rendered app runs its own
production server.

**Verify** on a dev machine: `docker build` succeeds and the container answers on
its port.

## 3. Add the workflow

Copy [`templates/workflows/preview.yml`](../templates/workflows/preview.yml) to
`.github/workflows/preview.yml` and fill in the `env` block. Then adjust the
marked sections:

- **`secrets.manager`** — with `github` instead of `doppler`, drop the Doppler
  steps and pass variables with `--env NAME` from step `env:` ([08](08-secrets.md)).
- **`backend.kind`** — with `none`, replace the Convex step with a plain
  `docker build` using the same tag, and remove the backend part of teardown
  ([09](09-backend-previews.md)).
- **Package manager and Node version** — match the project. Keep `dest:` on
  `pnpm/action-setup` ([pitfalls](pitfalls.md#two-runners-under-one-user-race-on-setup-pnpm)).
- **Monorepo with several apps** — one workflow per previewed app, each with its
  own `PROJECT` (e.g. `shop-web`, `shop-admin`) and its own hostname scheme, or
  one job that builds and starts several containers.

What the workflow guarantees, and must keep guaranteeing after edits:

- **Forks never run.** The `if:` on `deploy` checks the head repository. Don't
  switch the trigger to `pull_request_target`.
- **One build per PR at a time.** `concurrency` cancels a superseded build.
- **A failed build leaves the previous preview running.** The container is only
  replaced after the image and backend deploy succeed.
- **Branch names are untrusted.** They reach shell only through `env:`.
- **Teardown removes everything** on close or merge — container, backend preview,
  then image.

## 4. Create the GitHub environment

Repository Settings → Environments → **New environment** → `preview`
(`secrets.github_environment`). Don't add required reviewers — every push would
wait for approval. Its secrets come in [08](08-secrets.md).

The environment also gives each PR a **View deployment** button linking to its
preview.

## 5. Stop other hosting from building PR branches

If `other_hosting` is set, a PR now builds twice — and on Vercel, possibly with
production backend credentials
([pitfalls](pitfalls.md#vercel-still-builds-pr-branches)). Make that hosting build
`main` only before opening the first PR.

## 6. Open the pull request

Commit the Dockerfile, workflow and hosting change on a branch and open a PR —
with the user's go-ahead, following the project's own contribution rules.

**Verify:**

- The **Preview** check runs on the host's runner and passes.
- `https://pr-<n>.<preview_domain>` loads from a tester's device.
- A second push replaces the container; the old image is pruned.
- Closing the PR (or merging it) runs **remove preview**, and afterwards
  `docker ps -a --filter name=<project>-pr-<n>` and
  `docker images <project>-preview` are empty.
