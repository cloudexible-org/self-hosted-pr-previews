# Instructions for AI agents

You are setting up private, per-pull-request preview environments on the user's
own machine. This repository is the complete recipe: runbooks in `docs/`,
reference files in `templates/`, and one app in `apps/dashboard`. Work through it
in the order below.

## 1. Read before acting

1. [`README.md`](README.md) — what is being built and why.
2. [`docs/pitfalls.md`](docs/pitfalls.md) — the traps this setup has already hit.
   Most failures you will see are one of these; check here before debugging.

## 2. Get the configuration

Everything specific to the user's setup belongs in `config.yaml` (gitignored),
shaped like [`config.example.yaml`](config.example.yaml).

- If `config.yaml` exists, read it and confirm with the user that it is current.
- Otherwise, write it by interviewing the user. Discover what you can yourself,
  read-only, before asking: the OS and WSL version, existing WSL distros, whether
  Docker, Tailscale, `gh` and `doppler` are installed and logged in, which GitHub
  orgs `gh` can see, and where each project's DNS is hosted.
- Ask only for decisions and facts you cannot discover: which machine, which
  projects, which domains, who the testers are, which secrets manager and
  backend each project uses.
- Show the finished `config.yaml` to the user before starting the runbooks.

## 3. Follow the runbooks in order

| Runbook | Skip when |
| --- | --- |
| [01 — Host](docs/01-host.md) | never |
| [02 — GitHub runner](docs/02-github-runner.md) | never |
| [03 — Docker](docs/03-docker.md) | never |
| [04 — Private network](docs/04-private-network.md) | `access.mode: public` |
| [05 — Proxy and certificates](docs/05-proxy-and-certificates.md) | never |
| [06 — DNS](docs/06-dns.md) | never |
| [07 — Project workflow](docs/07-project-workflow.md) | never (once per project) |
| [08 — Secrets](docs/08-secrets.md) | never (once per project) |
| [09 — Backend previews](docs/09-backend-previews.md) | `backend.kind: none` |
| [10 — Auth](docs/10-auth.md) | `auth.provider: none` |
| [11 — Dashboard](docs/11-dashboard.md) | `dashboard.enabled: false` |
| [12 — Operations](docs/12-operations.md) | never |

Each runbook lists the config keys it **needs**, the **steps**, and how to
**verify** them. Do not start the next runbook until the current one verifies.
Show the user the evidence — command output, not "should work".

## 4. Rules

**Ask the user first** before any of these, even when you have the access:

- changing DNS records, Tailscale access policy, or anything in a cloud dashboard
  (Cloudflare, GitHub org settings, Doppler, Convex, WorkOS, Clerk, Vercel);
- anything on a machine other than the preview host, or in a WSL distro other
  than the one this setup creates — the user's own dev environment is off limits;
- restarting or replacing the proxy while previews are running;
- commits, pushes and pull requests in the user's project repositories. Follow
  each project's own contribution rules (`AGENTS.md`, `CONTRIBUTING.md`).

**Secrets never pass through the conversation.** When a token must be created and
stored, give the user a command to run in their own terminal, preferably one that
pipes it straight to its destination:

```sh
doppler configs tokens create gh-preview --project <project> --config preview --plain \
  | gh secret set DOPPLER_TOKEN_WEB --env preview --repo <owner>/<repo>
```

On Windows, tell the user to paste multi-line commands as a single line — a
PowerShell continuation breaks them apart and the command runs with missing
arguments.

**Keep project repositories generic.** A project's workflow and docs describe only
the contract in [`docs/07-project-workflow.md`](docs/07-project-workflow.md) —
network, entrypoint, resolver, name prefix, labels. Never write the host's tailnet
name, IPs, machine names or this repository into a project.

**Templates are references, not copy-paste.** Fill them from `config.yaml`, keep
the comments that explain *why*, and drop the optional sections the config does
not use.

## 5. Done means

- Opening a pull request in each project produces a preview at
  `https://pr-<number>.<preview_domain>` that loads with a valid certificate from a
  tester's device.
- Signing in works on the preview, when the project has auth.
- Closing the pull request removes the container, the image and the backend
  preview.
- Rebooting the preview host brings everything back without anyone logging in.
- The user has a short note of what was set up where — machine, runner services,
  DNS records, tokens and which dashboards hold them.
