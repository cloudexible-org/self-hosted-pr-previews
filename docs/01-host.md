# 01 — The preview host

**Needs:** `host.*`

The preview host is where the GitHub runner builds images, where preview
containers run, and where Traefik serves them. It should be a place of its own.

## Why a dedicated environment

- **It runs pull-request code.** Keep it away from your own dev environment,
  your files and your credentials.
- **It has its own lifecycle.** It must come back after a reboot with nobody
  logged in, and survive you reinstalling your dev tools.
- **It is disposable.** When something goes badly wrong, you can delete it and
  follow these runbooks again.

On Windows that means a dedicated WSL2 distro — not the distro you develop in.
On Linux, a separate machine, VM, or at least a separate user.

## Windows with WSL2 (`host.kind: wsl2`)

Requires WSL 2.4 or newer (`wsl --version`) for `--name` and `--location`.

1. **Create the distro** (in PowerShell, as the Windows user who will own it):

   ```powershell
   wsl --install Ubuntu-24.04 --name <host.wsl_distro> --location C:\wsl\<host.wsl_distro> --no-launch
   ```

2. **Create the user and lock the distro down.** As root inside it, create
   `host.user`, then write [`templates/host/wsl.conf`](../templates/host/wsl.conf)
   to `/etc/wsl.conf`. It enables systemd, sets the default user, and turns off
   Windows drive mounting and Windows interop, so nothing running here can read
   your Windows files or start Windows programs.

   Do this while the drive is still mounted: anything you need from Windows
   afterwards has to be piped in ([pitfalls](pitfalls.md#windows-cant-reach-the-distros-files)).

3. **Restart the distro:** `wsl --terminate <host.wsl_distro>`.

4. **Cap WSL's memory.** Write [`templates/host/wslconfig`](../templates/host/wslconfig)
   to `%USERPROFILE%\.wslconfig`. It applies to every distro and takes effect after
   `wsl --shutdown` — which stops all of them, so pick a moment.

5. **Keep it running from boot.** In an elevated PowerShell, run
   [`templates/host/keepalive-task.ps1`](../templates/host/keepalive-task.ps1) with
   `$distro` set to `host.wsl_distro`.

**Verify:**

```sh
# inside the distro
ps -p 1 -o comm=         # systemd
whoami                   # host.user
ls -A /mnt/c | wc -l     # 0 — no Windows drive mounted
cmd.exe /c ver           # fails — interop is off
```

```powershell
# on Windows, a few minutes after closing every terminal
wsl -l -v                # the distro is still Running
```

## Linux (`host.kind: linux`)

Use a machine or VM with systemd. Create `host.user` as a normal, non-root user.
Skip everything above.

## Base packages

As root on the host:

```sh
apt-get update
apt-get install -y ca-certificates curl jq git dnsutils libatomic1
```

`libatomic1` is not optional: `pnpm/action-setup` fails without it
([pitfalls](pitfalls.md#pnpm-needs-libatomic1)).

**Verify:** `dpkg -l libatomic1 dnsutils | grep ^ii` lists both.
