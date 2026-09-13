# 10 — Auth

**Needs:** `projects[].auth`, `projects[].preview_domain`

Sign-in is usually the reason previews need your own domain. An auth provider
redirects back only to URLs registered in advance, and a new hostname per PR can
only be registered as a wildcard.

## General rules

- Use the provider's **staging or development environment** for previews, with its
  own client ID and secret in the preview secrets. Never add preview URLs to
  production.
- Register the **wildcard at exactly one level**: `https://*.preview.myapp.com/...`
  matches `pr-12.preview.myapp.com`, and nothing deeper or shallower.
- **Private previews can't receive webhooks.** A provider's servers aren't on your
  tailnet, so `user.created`-style webhooks never arrive. If the backend learns
  about users from webhooks, seed the users testers sign in as
  ([09](09-backend-previews.md#seeding)).

## WorkOS (`auth.provider: workos`)

In the WorkOS dashboard, **staging** environment → Redirects:

- **Redirect URIs:** add `https://*.preview.myapp.com/callback` (the project's
  callback path).
- **Sign-out redirect** and, for AuthKit, the **app homepage** and allowed
  origins: add the same wildcard where the app uses them.

WorkOS refuses wildcards on public-suffix domains such as `*.vercel.app`, which is
why previews on a hosting provider's domain can't sign in
([pitfalls](pitfalls.md#workos-rejects-wildcards-on-public-suffix-domains)).

The preview secrets hold the staging `WORKOS_CLIENT_ID` and API key, and the app
builds its redirect URI from the preview's own URL (the `PUBLIC_APP_URL` build
arg in the template).

## Clerk (`auth.provider: clerk`)

Use the **development instance**. A production instance is tied to its domain and
won't serve other hostnames, while a development instance works from any origin,
with its usual development banner and limits
([pitfalls](pitfalls.md#clerk-previews-use-the-development-instance)).

The preview secrets hold the development publishable and secret keys.

## Others

Auth0, Okta, Cognito and most OAuth providers accept a subdomain wildcard in
allowed callback URLs, sometimes behind a setting. Providers that don't need a
fixed callback host instead — for example, one stable `auth.preview.myapp.com`
route that redirects back to the originating PR after sign-in.

**Verify:** on a preview, sign in as a seeded tester, sign out, and sign in again.
