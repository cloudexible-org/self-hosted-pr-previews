# 09 — Backend previews

**Needs:** `projects[].backend`

A frontend preview talking to a shared staging backend is only half a preview:
schema changes and new functions in the PR aren't there, and every PR's test data
lands in the same place. Each PR gets its own backend, created on the first
push, updated on every push, deleted when the PR closes.

## The generic shape

Whatever the backend, the workflow needs four things:

1. **Create or reuse** a backend named after the PR (`pr-<n>`), idempotently.
2. **Hand its URL to the image build**, because the frontend bakes it in.
3. **Record how to find it again** — the workflow labels the image with it, since
   the runner keeps nothing else between the deploy and the teardown.
4. **Delete it on close,** tolerating one that's already gone.

Neon and Supabase branches, PlanetScale branches, or a database container on the
host fit the same shape.

## Convex (`backend.kind: convex`)

### One-time project setup

The user, in the Convex dashboard for the project:

1. **Preview deploy key:** Project Settings → **Generate Preview Deploy Key**.
   Store it as `CONVEX_DEPLOY_KEY` in the backend's preview secrets
   ([08](08-secrets.md)). Never use the production deploy key here.
2. **Default environment variables for preview deployments:** Project Settings →
   Environment Variables. Every variable the backend reads, with preview values —
   the auth provider's staging credentials, a flag allowing the seed to run.
   Variables `convex.config.ts` declares as required must be here, even as
   placeholders, or the first push fails
   ([pitfalls](pitfalls.md#required-environment-variables-block-the-first-push)).

Check the project's Convex plan includes preview deployments.

### Seeding

`backend.seed_function` (for example `seed/preview:apply`) runs once, when the
preview deployment is created. Make it:

- **internal** (an `internalMutation` or `internalAction`), so the preview's
  public API can't call it;
- **guarded** by a preview-only environment variable, so it refuses to run
  anywhere else;
- **idempotent**, so running it by hand again is harmless.

Seed the accounts testers sign in with, when users normally arrive through an
auth provider's webhook: private previews can't receive webhooks
([10](10-auth.md)).

### The deploy step

```sh
doppler run -- npx convex deploy \
  --preview-name "pr-$PR" \
  --preview-run seed/preview:apply \
  --cmd-url-env-var-name PUBLIC_BACKEND_URL \
  --cmd 'docker build ... --label preview.convex-url="$PUBLIC_BACKEND_URL" --build-arg PUBLIC_BACKEND_URL ...'
```

- With a preview deploy key, `convex deploy` claims the preview named `pr-<n>`, or
  reuses it.
- It runs `--cmd` with the deployment URL in `--cmd-url-env-var-name`
  (`backend.url_env_var`), and only then pushes functions — so a broken image
  build doesn't push.
- `--preview-run` runs only when this deploy created the preview
  ([pitfalls](pitfalls.md#seeds-run-only-when-the-preview-is-created)).

If a first deploy fails after Convex created the deployment, the preview exists
without its seed or defaults. Delete that deployment in the dashboard and re-run
the job.

### Teardown

A preview deploy key can't look a deployment up by its preview name, but it can
delete one by deployment name. So the workflow reads the URL from the image label,
takes the deployment name from it, and calls the Management API:

```sh
curl --fail-with-body -H "Authorization: Bearer $CONVEX_DEPLOY_KEY" \
  https://api.convex.dev/v1/deployments/<name>            # read first: fail before deleting
curl --fail-with-body -X POST -H "Authorization: Bearer $CONVEX_DEPLOY_KEY" \
  https://api.convex.dev/v1/deployments/<name>/delete
```

The image is removed last, so a failed delete can be retried by re-running the
job. Images built before the label existed leave their deployment to Convex's own
expiry of preview deployments.

**Verify:** after closing a PR, the deployment named in the teardown log is gone
from the Convex dashboard's deployment list.
