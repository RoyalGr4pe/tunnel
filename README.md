# karo

`karo` exposes a local HTTP or HTTPS service through the fixed public address
`https://karo.salkaro.com` using an existing Cloudflare Tunnel.

## Install

Requirements: Go 1.22+ and [`cloudflared`](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/downloads/).

```bash
make build
make install
```

The install target writes `karo` to `~/.local/bin` by default. Set `BINDIR` to
choose another location.

## One-time Cloudflare setup

Run the following while authenticated to the Cloudflare account that owns
`salkaro.com`:

```bash
cloudflared tunnel login
cloudflared tunnel create karo
cloudflared tunnel route dns karo karo.salkaro.com
```

The `create` command prints the tunnel UUID and creates a credentials file at
`~/.cloudflared/<TUNNEL_UUID>.json`. Create `~/.config/karo/config.json` from
the example below, replacing both placeholders. `hostname` is optional, but if
present it must remain `karo.salkaro.com`.

```json
{
  "tunnel_id": "YOUR_TUNNEL_UUID",
  "credentials_file": "/home/USER/.cloudflared/YOUR_TUNNEL_UUID.json",
  "hostname": "karo.salkaro.com"
}
```

The credentials JSON is private. Do not place it in this repository.

## Usage

Start the local service first, then run one of:

```bash
karo http 3000
karo http 5173
karo https 8443
```

Each command makes `https://karo.salkaro.com` route to the requested local
origin. `https` expects an HTTPS service on localhost; karo disables upstream
certificate verification for that loopback connection to support local
development certificates.

Only one invocation should run at a time because they use the same hostname and
tunnel ingress rule. Karo opens a full-screen terminal dashboard rather than
printing Cloudflare logs continuously. Press `q` or `Ctrl-C` to stop the tunnel,
or press `l` to toggle the eight most recent Cloudflare log lines. The temporary
Cloudflare configuration is removed automatically.

## Private Access

To restrict the public hostname, create a self-hosted application for
`karo.salkaro.com` in the Cloudflare Zero Trust dashboard and add a Cloudflare
Access policy for the allowed identities or identity provider. This CLI does
not change Cloudflare account settings.
