# 08 — Secrets

**Needs:** `secrets.*`, `projects[].secrets`

A preview needs two kinds of values:

- **Build-time, public:** baked into the image as build args — the preview's own
  URL, the backend URL, publishable keys.
- **Runtime, secret:** given to the container as environment variables — API keys,
  auth client secrets, webhook secrets — plus whatever the backend deploy needs
  (a preview deploy key).

Previews should get **their own credentials**, never production's: a staging auth
environment, test-mode payment keys, a preview deploy key. A preview runs code
nobody has reviewed yet.

## Doppler (`secrets.manager: doppler`)

### Configs

Give each Doppler project a `preview` config (usually branched from `dev`) holding
exactly what a preview needs. With separate Doppler projects for the web app and
the backend, each gets its own `preview` config.

Everything in the web app's `preview` config becomes the container's environment,
so keep build-only or unrelated values out of it.

### Service tokens

A Doppler service token reads one config of one project. Create one per config
and store it straight into the GitHub environment, without it passing through the
conversation or a file. The user runs, once per token, as **one line**:

```sh
doppler configs tokens create gh-preview --project myapp_web --config preview --plain | gh secret set DOPPLER_TOKEN_WEB --env preview --repo my-org/myapp
doppler configs tokens create gh-preview --project myapp_api --config preview --plain | gh secret set DOPPLER_TOKEN_BACKEND --env preview --repo my-org/myapp
```

Service tokens start with `dp.st.`. A personal (`dp.pt.`) or CLI token works too,
until the person leaves.

**Verify** without printing secrets — the workflow step fails on a bad token, but
this checks first:

```sh
gh secret list --env preview --repo my-org/myapp       # both names present
```

### In the workflow

- `doppler run -- <cmd>` for the backend deploy: the command sees the backend's
  preview values.
- `doppler secrets download --no-file --format docker` for the container, filtered
  to drop Doppler's own `DOPPLER_*` metadata, into `--env-file`. Downloading into a
  variable first makes a bad token fail the step instead of starting a container
  without secrets.

Nesting `doppler run` inside another `doppler run` (a project's own scripts often
do) needs `--project` and `--config` on the inner one
([pitfalls](pitfalls.md#nested-doppler-run-leaks-the-outer-project)).

## GitHub secrets only (`secrets.manager: github`)

Store each value as a secret of the `preview` environment and pass them
explicitly — secrets aren't exposed to steps unless named:

```yaml
      - name: Start the preview container
        env:
          AUTH_CLIENT_SECRET: ${{ secrets.AUTH_CLIENT_SECRET }}
          API_KEY: ${{ secrets.API_KEY }}
        run: |
          docker run --detach ... \
            --env AUTH_CLIENT_SECRET \
            --env API_KEY \
            ...
```

`--env NAME` without a value copies it from the step's environment, so the
secret never appears in the command line or the logs.

## Where secrets end up on the host

- In the container's configuration: anyone with Docker access on the host can read
  them with `docker inspect`. The dashboard deliberately cannot
  ([11](11-dashboard.md)).
- Never in the image: runtime values aren't build args, and `--env-file` reads a
  process substitution, not a file on disk.
- In the runner's job logs only if a step prints them. Doppler and GitHub mask
  values they know; mask anything you fetch yourself with `::add-mask::`.
