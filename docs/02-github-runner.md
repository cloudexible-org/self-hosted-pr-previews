# 02 — GitHub runner

**Needs:** `github.runners[]`, `host.user`

Each preview workflow runs on a self-hosted runner on the preview host. A
runner belongs to exactly one org (or one repo), so a host serving several orgs
runs one runner per org — side by side, each as its own service.

## Decide the scope

- **Org runner in a runner group (preferred).** Create a runner group (org
  Settings → Actions → Runner groups), for example `previews`, limited to the
  repositories that get previews, with **public repositories not allowed**.
  Workflows target it with `runs-on: { group: previews, labels: [self-hosted, linux] }`.
- **Repo runner.** When the org's plan has no runner groups, or you lack org
  admin. Workflows target labels only:
  `runs-on: [self-hosted, linux, previews]`.

Either way, the workflow itself refuses pull requests from forks
([07](07-project-workflow.md)). A self-hosted runner must never run code from
someone outside the team.

## Install

As `host.user`, once per runner (`github.runners[]`):

```sh
V=$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest | jq -r .tag_name | sed 's/^v//')
mkdir -p ~/actions-runner-<owner> && cd ~/actions-runner-<owner>
curl -fsSL "https://github.com/actions/runner/releases/download/v${V}/actions-runner-linux-x64-${V}.tar.gz" | tar xz
sudo ./bin/installdependencies.sh      # once per host
```

## Register

A registration token expires after an hour. Get it from the org's (or repo's)
Settings → Actions → Runners → **New self-hosted runner**, or with
`gh api -X POST orgs/<owner>/actions/runners/registration-token --jq .token`
(needs the `admin:org` scope).

Keep the token out of the conversation: the user runs this in their own
terminal, as **one line** — a multi-line paste in PowerShell arrives in bash as
separate commands, and `<TOKEN>` left in place reads as a file redirect.

```powershell
$t = Read-Host "Runner token"
wsl -d <distro> -u <user> -- bash -c "cd ~/actions-runner-<owner> && ./config.sh --unattended --url https://github.com/<owner> --token $t --name <runner_name> --runnergroup <runner_group> --labels <labels> --work _work"
```

Then install and start it as a service:

```sh
cd ~/actions-runner-<owner>
sudo ./svc.sh install <host.user>
sudo ./svc.sh start
```

**Verify:**

```sh
systemctl list-units 'actions.runner.*'          # active (running)
gh api orgs/<owner>/actions/runners --jq '.runners[] | "\(.name) \(.status)"'   # online
```
