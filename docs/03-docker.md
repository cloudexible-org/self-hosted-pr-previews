# 03 — Docker Engine

**Needs:** `host.user`

Install Docker Engine inside the preview host.

On Windows, not Docker Desktop's WSL integration:

- Docker Desktop starts only after someone logs into Windows, so previews would be
  down after every reboot until then.
- Its engine is shared with your own development, so cleanup on one side deletes
  the other's containers and images.
- Its Docker socket reaches the Windows filesystem. Traefik holds that socket.

## Install

As root, following Docker's apt repository instructions for the host's Ubuntu
release:

```sh
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
. /etc/os-release
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
  > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker containerd
usermod -aG docker <host.user>
```

Membership of the `docker` group is effectively root on the host. That is
acceptable here because the host exists only for this.

## Keep it from filling the disk

1. **Log rotation:** write [`templates/host/daemon.json`](../templates/host/daemon.json)
   to `/etc/docker/daemon.json` and restart Docker. It applies only to containers
   created afterwards.
2. **Build cache:** install
   [`docker-builder-prune.service`](../templates/host/docker-builder-prune.service)
   and [`.timer`](../templates/host/docker-builder-prune.timer) into
   `/etc/systemd/system/`, then `systemctl enable --now docker-builder-prune.timer`.

**Verify:**

```sh
sudo -u <host.user> docker run --rm hello-world
systemctl is-enabled docker docker-builder-prune.timer
docker info --format '{{.LoggingDriver}}'        # json-file
```
