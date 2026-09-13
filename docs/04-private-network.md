# 04 — Private network (Tailscale)

**Needs:** `access.tailscale.*`, `host.kind`

Previews run unreviewed code with preview credentials. Keeping them off the
internet removes a whole class of worry: no public port on your router, no bots
finding a half-finished sign-up page, no need to put auth in front of every
preview. Testers install Tailscale and sign in; that's the access control.

## Install and join

As root on the host:

```sh
curl -fsSL https://tailscale.com/install.sh | sh
```

If `access.tailscale.userspace_networking` is true — on WSL2 whenever another
distro on the machine runs Tailscale, and it's the safe default there — write
[`templates/host/tailscaled.default`](../templates/host/tailscaled.default) to
`/etc/default/tailscaled` and `systemctl restart tailscaled`
([why](pitfalls.md#wsl-shares-one-network)).

Then join the tailnet. This prints a login URL the user opens:

```sh
tailscale up --hostname=<access.tailscale.node_name> --advertise-tags=tag:preview-host
```

A tag makes the host a server rather than a device owned by whoever logged it
in: it stays in the tailnet if that person leaves, and its key doesn't expire.
The tag must exist in the policy (below) before `tailscale up` can claim it.

In the admin console, under **DNS**, turn on **MagicDNS** and **HTTPS
Certificates**. The dashboard's certificate needs both ([11](11-dashboard.md)).

**Verify:**

```sh
tailscale status            # the host is listed, online, with its tag
tailscale ip -4             # its 100.x address — goes into DNS in 06
```

## Access policy

Tailscale's default policy lets every device reach every other device. Replace
it so testers can open previews and nothing else. Show this to the user and let
them apply it in the admin console — **merge it with their existing policy**
rather than replacing groups and grants they already rely on:

```jsonc
{
  "groups": {
    "group:admins":  ["you@example.com"],
    // access.tailscale.testers
    "group:testers": ["teammate@example.com"]
  },
  "tagOwners": {
    "tag:preview-host": ["group:admins"]
  },
  "grants": [
    // Admins keep full access to everything.
    { "src": ["group:admins"],  "dst": ["*"], "ip": ["*"] },
    // Testers reach previews over HTTPS, and nothing else on the tailnet.
    { "src": ["group:testers"], "dst": ["tag:preview-host"], "ip": ["tcp:443"] }
    // Tagged devices are not users: a tagged machine of yours that must reach
    // the host needs its own grant, e.g.
    // { "src": ["tag:server"], "dst": ["*"], "ip": ["*"] }
  ],
  "tests": [
    { "src": "teammate@example.com", "accept": ["tag:preview-host:443"], "deny": ["tag:preview-host:22"] },
    { "src": "you@example.com",      "accept": ["tag:preview-host:443", "tag:preview-host:22"] }
  ]
}
```

`tests` make the admin console reject a policy change that breaks these
expectations. Add one for every device that must keep working — tagged ones
especially ([pitfalls](pitfalls.md#tagged-tailscale-devices-arent-users)).

## Inviting a tester

1. Invite them to the tailnet (admin console → Users → Invite). A tester outside
   your organisation's identity provider can join with a personal login.
2. Add their login to `group:testers` and to `access.tailscale.testers`.
3. They install Tailscale on the device they test from and sign in.

**Verify**, from a tester's device once DNS and the proxy are up:
`https://pr-<n>.<preview_domain>` loads, and `ssh <node_name>` does not connect.

## Without Tailscale

`access.mode: public` means a public IP (or a tunnel) pointing at port 443, and
DNS records that are public for real. It works with everything else in these
runbooks unchanged, but every preview is then on the internet: put
authentication in front of it (a Traefik `forwardAuth` middleware to an identity
proxy, for example) and expect scanners within minutes of the first certificate
appearing in certificate-transparency logs.
