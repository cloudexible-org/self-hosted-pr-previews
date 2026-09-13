# I replaced paid preview deployments with a desktop and a tailnet

*Private previews for every pull request, on my own domain, with sign-in that
works. I open-sourced the setup as instructions for your AI agent, not as a tool.*

---

Preview deployments are one of those features you don't notice until they're
gone. Open a pull request, get a link, send it to whoever needs to look at it.
Hosted platforms made this effortless, and for a long time it was.

Then sign-in broke it.

## The $100 problem

My apps authenticate through WorkOS. Like every OAuth-style provider, WorkOS only
redirects back to URLs you've registered. A preview gets a new hostname per pull
request, so the only way to register them is a wildcard:
`https://*.preview.myapp.com/callback`.

WorkOS accepts that wildcard, but refuses one on a public-suffix domain like
`*.vercel.app`. Their reasoning is sound — anyone can deploy something under
`vercel.app` — but it means previews on the host's default domain can't sign in.
The fix is a custom domain for preview deployments, which on my plan was a
$100/month add-on.

At the same time, a desktop with far more cores than any preview build needs was
sitting idle next to me for most of the day.

## What I built instead

Every same-repository pull request now gets
`https://pr-<number>.preview.myapp.com`:

- **A self-hosted GitHub Actions runner** picks up the job on my desktop, inside a
  dedicated WSL2 distro that has nothing to do with my own dev environment.
- **Docker** builds the app's image and runs it.
- **Its own backend:** the workflow creates a Convex preview deployment named after
  the PR, seeds it with test accounts, and bakes its URL into the image.
- **Traefik** routes the hostname to the container, using nothing but Docker labels,
  with a wildcard Let's Encrypt certificate per project.
- **Tailscale** makes all of it private. The DNS record points at the machine's
  tailnet address, so a preview link only opens for people on my tailnet.
- **Teardown:** closing or merging the PR removes the container, the image and the
  backend preview.

A small dashboard lists every running preview across projects and streams their
logs in the browser.

It has been serving two products for a few weeks. Previews build faster than they
did on the hosted platform, because the Docker layer cache is warm and the machine
is idle.

## Three ideas that made it work

### 1. Private beats protected

The first instinct with self-hosting is to expose a port and put auth in front of
it. I didn't want to maintain that, or explain to a tester why a preview asks them
to log in twice.

With Tailscale, the preview host has no public port at all. Testers install
Tailscale and sign in once; the access policy lets them reach port 443 on the
preview host and nothing else on the network. Removing someone from the tailnet
removes their access to every preview at once.

### 2. Certificates without a public server

Let's Encrypt can't reach a private machine over HTTP, so certificates use the
DNS-01 challenge: prove you control a name by publishing a TXT record.

That normally means giving the machine an API token that can edit your domain's
DNS. For several projects, possibly in several DNS accounts, that's a lot of
power sitting on a desktop that runs unreviewed code.

Instead, each project's zone gets one CNAME:

```
_acme-challenge.preview.myapp.com  CNAME  myapp-preview.acme.my-infra-domain.net
```

Let's Encrypt follows it, and Traefik writes the challenge into a throwaway zone
that holds nothing else. The only DNS token on the machine can edit only that
zone. My projects live in separate Cloudflare accounts, and none of their
credentials ever touch the preview host.

### 3. Tear down the backend, too

The part hosted previews quietly handle for you is cleanup. Containers are easy.
Backend previews are not: a preview deploy key can create and delete a Convex
deployment, but can't look one up by PR number. So the build labels the image with
the deployment's URL, and the teardown job reads the label and calls the delete
endpoint. The image is removed last, so if the delete fails, re-running the job
retries it.

## The bumps

The architecture took an afternoon. The pitfalls took the rest. A few favourites:

- **All WSL2 distros share one network.** My dev distro already ran Tailscale and
  owned the `tailscale0` device. The preview host's Tailscale runs in userspace
  networking mode instead, and still receives connections fine.
- **WSL stops a distro when nothing on Windows holds it open**, systemd services or
  not. A scheduled task that starts at boot keeps it alive with nobody logged in.
- **Nested `doppler run` leaks.** The outer one exports `DOPPLER_PROJECT`, and the
  inner one — inside a project script — obediently asks for the wrong project with
  the right token. The error says the token has no access, which sends you looking
  in exactly the wrong place.
- **Convex runs the seed only when it creates the preview.** A first deploy that
  fails halfway leaves an unseeded deployment that every retry reuses.
- **Moving to pull requests changes what your old host builds.** A project that
  deployed from `main` suddenly had Vercel building every branch — with whatever
  backend deploy key sat in its preview environment. Check that before opening the
  first PR.
- **Let's Encrypt's secondary validation failed for a day** with every DNS check
  clean. It was on their side. Staging environments exist so that day doesn't burn
  your production rate limit.

Every one of these is written up in the repository, with the symptom first, so
the next person searching the error message finds the fix.

## Why it's instructions, not a tool

When I went to open-source this, I realised almost nothing in it was reusable
as code. My setup is Windows with WSL, Cloudflare, Doppler, Convex and WorkOS.
Yours is probably a Linux box, Route 53, GitHub secrets and Postgres. A script that
handles every combination would be enormous, and wrong for everyone.

What *is* reusable is the knowledge: the architecture, the order things have to
happen in, how to check each step worked, and the traps.

So the repository is a set of runbooks, reference templates, a single
`config.example.yaml` holding every setup-specific value, and an `AGENTS.md` that
tells a coding agent how to use them: interview the user to fill in the config,
work through the runbooks in order, show evidence that each step verifies, and
ask before touching DNS, access policies or cloud dashboards. Secrets are created
by commands the user runs themselves, piped straight to where they're stored, so
they never pass through the conversation.

Clone it, open your agent of choice, and say:

> Read AGENTS.md and set up self-hosted PR previews for my projects.

The one piece of real code is the dashboard — a small Go app that reads Docker
through a socket proxy allowing only `GET` requests, so a page that shows logs can
never start, stop or inspect a container's secrets.

## Is this for you?

It is if you have a small team you trust, a machine that stays on, and a reason
previews need your own domain. It isn't if you accept pull requests from strangers
— never run fork code on a self-hosted runner — or if a preview being down while
your machine reboots is a problem.

The repository: **github.com/cloudexible-org/self-hosted-pr-previews** (MIT).

If your agent hits a trap that isn't in `docs/pitfalls.md` yet, a pull request
adding it is the most useful contribution there is.
