<div align="center">

<img src="./media/omega-banner.png" alt="OMEGA" width="100%">

# OMEGA

**A panel for managing Xray-core servers — built on [3x-ui](https://github.com/MHSanaei/3x-ui) `v3.3.1`, extended with [Resellers (نمایندگی)](#-resellers-نمایندگی), external OpenVPN, and external L2TP/IPsec daemons.**

English · [فارسی](README.fa_IR.md)

[![Release](https://img.shields.io/github/v/release/Dark-Sky07/OMEGA?include_prereleases&label=release&color=blue)](https://github.com/Dark-Sky07/OMEGA/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/Dark-Sky07/OMEGA/release.yml?label=build)](https://github.com/Dark-Sky07/OMEGA/actions)
[![License](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
[![Base](https://img.shields.io/badge/base-3x--ui%20v3.3.1-informational)](https://github.com/MHSanaei/3x-ui/releases/tag/v3.3.1)
[![Go](https://img.shields.io/github/go-mod/go-version/Dark-Sky07/OMEGA)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20windows-lightgrey)](#supported-platforms)

**Install in one line** — on a fresh server, as `root`:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Dark-Sky07/OMEGA/v3.3.18-omega/install-omega.sh) v3.3.18-omega
```

</div>

> [!NOTE]
> OMEGA is a fork based on 3x-ui v3.3.1. It adds reseller controls, an external OpenVPN daemon, and an external
> L2TP/IPsec daemon while keeping the panel service name (`x-ui`), install paths (`/usr/local/x-ui`, `/etc/x-ui`),
> environment variables, and Xray configuration conventions compatible with the upstream project. The OMEGA release
> version is maintained separately from the upstream core version, so the current stable release is `v3.3.18-omega`.

---

## :sparkles: What's different from upstream 3x-ui

| | Change |
| --- | --- |
| ➕ **Added** | **Resellers (نمایندگی)** — sub-accounts with their own login, scoped ownership, quotas and sales/billing reports. |
| ➕ **Added** | **OpenVPN inbounds** — an OpenVPN daemon per inbound binds the port directly; per-client certificates are generated automatically (CN = email), each client gets a ready-to-import `.ovpn` (copy/download from the client info), and per-client traffic + online status flow into the normal stats pipeline. The tagged release installer installs and verifies the host `openvpn` package and `/dev/net/tun`; the panel then renders one daemon config per enabled local OpenVPN inbound. The release archive does not embed an OS package, so use the installer rather than copying only the panel tarball. |
| ➕ **Added** | **L2TP/IPsec inbounds** — one global local-Linux daemon group (strongSwan + xl2tpd/PPP) uses existing client email/password credentials, persists the IPsec PSK, reconciles daemon config and `chap-secrets` synchronously for single and bulk client operations, recovers orphaned daemons after restart, manages UDP 500/4500/1701 plus IPv4 forwarding/FORWARD/MASQUERADE rules, and reports PPP online/traffic state. It is not an Xray inbound, does not generate a profile file, and does not include PPTP; the UI shows native client parameters. |
| 🎨 **Branding** | Panel name shown as **OMEGA** (sidebar, login page, page titles, API docs, translations). UI branding only; service names and install paths remain unchanged. |
| 🛠 **Install** | [`install-omega.sh`](install-omega.sh) installs *this* panel from *this* repository; [`x-ui.sh`](x-ui.sh) updates from here too, so `x-ui update` can never silently swap in vanilla 3x-ui. |
| ✅ **Unchanged** | Everything else — all of 3x-ui v3.3.1 (protocols, transports, nodes, subscriptions, Telegram bot, routing, API, themes, 13 languages). |

### OpenVPN installation and host checklist

OpenVPN is a host daemon, not an Xray component and not a file inside the `x-ui` release archive. The release installer installs the `openvpn` package, validates `/dev/net/tun`, and aborts instead of silently installing a panel that cannot serve an OpenVPN inbound. It intentionally does not enable a generic `openvpn.service`: OMEGA creates `bin/openvpn/<inbound-id>/openvpn.conf` and starts one daemon per enabled local OpenVPN inbound.

To install or repair an existing host with the exact stable release, run as `root`:

```bash
curl -fsSL https://raw.githubusercontent.com/Dark-Sky07/OMEGA/v3.3.18-omega/install-omega.sh \
  -o /tmp/install-omega.sh
env OMEGA_REF=v3.3.18-omega bash /tmp/install-omega.sh v3.3.18-omega
```

Verify the prerequisite before troubleshooting the network:

```bash
command -v openvpn
openvpn --version | head -3
test -c /dev/net/tun && echo "TUN is ready"
systemctl restart x-ui
```

The `x-ui update` script shipped in this release resolves the latest stable release tag and runs its matching installer; it no longer downloads an unpinned, possibly stale `main` installer. For an OpenVPN inbound, allow the configured port on both the host firewall and the VPS/provider firewall using the selected transport (`udp` or `tcp`). When **Redirect gateway** is enabled, the host must also have IPv4 forwarding and NAT/masquerading configured for the generated `10.x.x.0/24` tunnel subnet; the panel does not overwrite an operator's firewall policy. If a profile imports but remains on `Trying to connect`, check `command -v openvpn`, `/dev/net/tun`, the listener with `ss -lunpt`, and `/var/log/x-ui/3xui.log` plus `bin/openvpn/<inbound-id>/openvpn.log`.

### L2TP/IPsec installation and host checklist

L2TP/IPsec is a host daemon, not an Xray protocol. The installer installs and verifies `strongswan`, `xl2tpd`, `ppp`, `iptables`, and `iproute2`, and checks `/dev/ppp`. Only one enabled local L2TP/IPsec inbound is allowed because the daemon group owns UDP 500 (IKE), UDP 4500 (NAT-T), UDP 1701 (L2TP), and ESP protocol 50 for clients without NAT. PPTP is not part of OMEGA.

The panel writes managed runtime files under `bin/l2tp/<inbound-id>/`, enables IPv4 forwarding, opens the three UDP listeners in iptables, and installs `FORWARD` plus `MASQUERADE` rules for the configured pool. Attach existing clients from the normal Clients page; their email is the PPP username and their password is the MS-CHAPv2 credential. The inbound info view shows the PSK, pool, DNS, and fixed ports. Subscription info shows one native parameter set for every attached client with that subscription ID, and the admin Client Information dialog shows the same values with copy buttons. Configure clients with their native L2TP/IPsec settings; no profile file or synthetic Xray link is generated.

Verify a host or manual installation with:

```bash
command -v ipsec xl2tpd pppd iptables sysctl
ipsec --version | head -2
command -v xl2tpd && command -v pppd
# On a host installation:
test -c /dev/ppp && echo "PPP is ready"
ss -lunp | grep -E ':(500|4500|1701)\\b'
```

For Docker, the container needs `NET_ADMIN`, `NET_RAW`, `/dev/ppp`, `/dev/net/tun`, IPv4 forwarding, and published UDP 500, 4500, and 1701. The repository `docker-compose.yml` contains these settings. The host kernel must provide PPP and XFRM/IPsec; a Docker container cannot load a missing host kernel module.

### L2TP/IPsec feature highlights

- **One global listener:** the panel accepts only one L2TP/IPsec inbound per local host. UDP 500 and 4500 are used by IKE/NAT-T and UDP 1701 by L2TP.
- **Existing clients:** attach clients from the normal Clients page; email becomes the PPP username and the existing password becomes the MS-CHAPv2 secret. Enable/disable, password edits, attach, detach, bulk delete, and quota enforcement are reconciled without putting L2TP into the Xray configuration.
- **Managed networking:** OMEGA enables IPv4 forwarding and maintains exact `INPUT`, `FORWARD`, and `POSTROUTING MASQUERADE` rules for the configured pool. Firewall state is persisted so restart and cleanup can reclaim rules safely.
- **Restart-safe lifecycle:** daemon files live under `bin/l2tp/<inbound-id>/`; strongSwan/xl2tpd are started, stopped, reconciled after settings changes, and orphaned processes are reclaimed after a panel restart.
- **No profile files:** the panel displays PSK, pool, DNS, and port values as native connection parameters. PPTP is intentionally out of scope.

---

## :rocket: Installation

### One-line install (recommended)

On a fresh server, as **root**:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Dark-Sky07/OMEGA/v3.3.18-omega/install-omega.sh) v3.3.18-omega
```

Pin a specific release instead (useful before a branch is merged):

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Dark-Sky07/OMEGA/v3.3.18-omega/install-omega.sh) v3.3.18-omega
```

The installer takes care of everything:

1. installs the required packages (`curl`, `tar`, `socat`, `openssl`, `tzdata`, cron …) plus the host `openvpn`, strongSwan, xl2tpd, PPP, iptables, and iproute2 packages,
2. verifies that `/dev/net/tun` and `/dev/ppp` are available; installation stops if the VPN prerequisite is not usable,
3. downloads the packaged release for your architecture (panel **+ Xray-core + geoip/geosite + mtg**),
4. installs it to `/usr/local/x-ui` and registers the unchanged `x-ui` systemd service,
5. keeps an existing database/settings when upgrading, and
6. restarts the panel and prints the access URL.

Defaults: port **2053**, login **admin / admin** — change both right after your first login.

### Managing the panel

```bash
x-ui              # management menu: start / stop / restart / settings / log / update / uninstall
x-ui status       # service status
x-ui settings     # current settings, including the hidden web base path
x-ui log          # panel log
x-ui update       # update to the newest OMEGA release (never downgrades to vanilla 3x-ui)
x-ui uninstall    # full removal (the database in /etc/x-ui is kept; back it up first)
```

### Manual install

Grab `x-ui-linux-<arch>.tar.gz` from the [releases page](https://github.com/Dark-Sky07/OMEGA/releases)
(`amd64`, `arm64`, `armv7`, `armv6`, `386`, `armv5`, `s390x`), then on the server. Manual extraction of the tarball does **not** install host VPN packages; run the following first if you use this path:

```bash
apt-get update && apt-get install -y openvpn strongswan xl2tpd ppp iptables iproute2
modprobe tun 2>/dev/null || true
modprobe ppp_generic 2>/dev/null || true
test -c /dev/net/tun && test -c /dev/ppp
```

```bash
tar zxvf x-ui-linux-amd64.tar.gz
cd x-ui && chmod +x x-ui bin/xray-linux-*
./x-ui                                             # first run creates the database
cp -f x-ui.service /etc/systemd/system/ 2>/dev/null \
  || cp -f x-ui.service.debian /etc/systemd/system/x-ui.service
cp -f x-ui.sh /usr/bin/x-ui && chmod +x /usr/bin/x-ui
systemctl daemon-reload && systemctl enable --now x-ui
```

### Upgrade / backup

```bash
x-ui update                                   # upgrade in place, settings preserved
cp /etc/x-ui/x-ui.db /root/x-ui-backup.db     # or use Settings → Backup inside the panel
```

📖 **Persian step-by-step guide:** [docs/OMEGA-INSTALL.fa.md](docs/OMEGA-INSTALL.fa.md) — نصب، ساخت اولین نمایندگی،
بروزرسانی، بکاپ و نکات امنیتی.

---

## :briefcase: Resellers (نمایندگی)

A **reseller** is a panel sub-account that owns a slice of the panel: some of the inbounds, some of the clients,
and a defined set of quotas — while the admin keeps full control over everything.

### How it works

- **Own login.** A reseller signs in on the same login page with its own username/password. Its session can only
  reach `/panel/api/inbounds/*`, `/panel/api/clients/*`, `/panel/api/reseller/*` and `/panel/api/auth/me` — every
  other endpoint answers `403`, and the panel-wide WebSocket feed is admin-only.
- **Scoped ownership.** An inbound can be handed to a reseller, and individual clients can be assigned directly.
  A reseller sees exactly what it owns; anything else answers `inbound not found` / `client not found`.
  Ownership is stored in mapping tables, so the base inbound/client tables stay pristine.
- **Quotas.** Traffic cap (the sum of the quotas it allocates to clients), maximum client count, maximum inbound
  count, plus an optional expiry date. A quota of `0` means *unlimited*. Limits are enforced on create, update,
  bulk operations and inbound import — not just in the UI.
- **Sales & billing.** Per-client usage report (quota, usage, cost, attached inbounds), a price per GB, a prepaid
  balance, deposit/withdrawal ledger and settlement history. Balance = deposit − (used GB × price per GB).
- **Disable / reset.** Disabling a reseller blocks the login and turns its inbounds off; a password reset
  invalidates every live session of that reseller immediately.

```mermaid
flowchart LR
    A[Admin panel<br/>admin / admin] -->|assign inbounds| B[Reseller: ali]
    A -->|assign single clients| B
    A -->|quotas: traffic · clients · inbounds| B
    A -->|price per GB · deposit| B
    B -->|own login| C[Sees only own inbounds & clients]
    C --> D[Creates / edits / deletes own clients<br/>within its quotas]
    D --> E[Sales report<br/>usage · cost · balance · ledger]
    A --> F[Admin still sees and manages everything]
```

### Panel pages

| Role | Pages |
| --- | --- |
| **Admin** | **Resellers** (list, create/edit, quotas, assign inbounds, assign clients, balance, reset password, report drawer) + everything 3x-ui has |
| **Reseller** | **Report** (usage, cost, balance, per-client rows, ledger), **Profile** (quotas, usage, change password), **Inbounds** and **Clients** (its own only) |

### API

The admin-side management API lives under `/panel/api/resellers` and is documented in the panel's **API Docs** page:

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/panel/api/resellers/list` | Every reseller with a live usage snapshot |
| `GET` | `/panel/api/resellers/get/:id` | One reseller |
| `GET` | `/panel/api/resellers/assignments` | Ownership map (inbounds + explicitly assigned clients) |
| `GET` | `/panel/api/resellers/report/:id` | Usage + per-client rows + money ledger |
| `POST` | `/panel/api/resellers/add` · `update/:id` · `del/:id` | Create / update / delete |
| `POST` | `/panel/api/resellers/setEnable/:id` · `resetPassword/:id` | Enable-disable · password reset |
| `POST` | `/panel/api/resellers/assignInbound` · `unassignInbound` | Hand an inbound over / take it back |
| `POST` | `/panel/api/resellers/assignClient` · `unassignClient` | Assign / unassign a single client |
| `POST` | `/panel/api/resellers/balance` | Deposit (+) or withdraw (−) on the reseller account |

And the reseller's own endpoints (reachable with a reseller session):

| Method | Endpoint | Purpose |
| --- | --- | --- |
| `GET` | `/panel/api/reseller/profile` · `stats` | Own account, quotas and live usage |
| `GET` | `/panel/api/reseller/report` | Own sales report and ledger |
| `POST` | `/panel/api/reseller/password` | Change own password |
| `GET` | `/panel/api/auth/me` | Session role (`admin` or `reseller`) — used by the UI |

---

## :camera: Screenshots

Panel pages inherited from 3x-ui v3.3.1 (the reseller pages follow the same design):

<details>
<summary>Click to expand</summary>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/01-overview-dark.png">
  <img alt="Overview" src="./media/01-overview-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/02-add-inbound-dark.png">
  <img alt="Inbounds" src="./media/02-add-inbound-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/03-add-client-dark.png">
  <img alt="Add client" src="./media/03-add-client-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/05-add-nodes-dark.png">
  <img alt="Configs" src="./media/05-add-nodes-light.png">
</picture>

</details>

---

## Features

Inherited from 3x-ui v3.3.1, untouched:

- **Multi-protocol inbounds** — VLESS, VMess, Trojan, Shadowsocks, WireGuard, Hysteria2, HTTP, SOCKS (Mixed), Dokodemo-door / Tunnel, and TUN.
- **Modern transports & security** — TCP (Raw), mKCP, WebSocket, gRPC, HTTPUpgrade, and XHTTP, secured with TLS, XTLS, and REALITY.
- **Fallbacks** — serve multiple protocols on a single port (e.g. VLESS and Trojan on 443) using Xray's fallback support.
- **Per-client management** — traffic quotas, expiry dates, IP limits, live online status, and one-click share links, QR codes, and subscriptions.
- **Traffic statistics** — per inbound, per client, and per outbound, with reset controls.
- **Multi-node support** — manage and scale across multiple servers from a single panel.
- **Outbound & routing** — WARP, NordVPN, custom routing rules, load balancers, and outbound proxy chaining.
- **Built-in subscription server** with multiple output formats and [custom page templates](docs/custom-subscription-templates.md).
- **Telegram bot** for remote monitoring and management.
- **RESTful API** with in-panel Swagger documentation.
- **Flexible storage** — SQLite (default) or PostgreSQL.
- **13 UI languages** with dark and light themes.
- **Fail2ban integration** for enforcing per-client IP limits.

## Supported Platforms

**Operating systems:** Ubuntu, Debian, Armbian, Fedora, CentOS, RHEL, AlmaLinux, Rocky Linux, Oracle Linux, Amazon Linux, Virtuozzo, Arch, Manjaro, Parch, openSUSE (Tumbleweed / Leap), Alpine, and Windows.

**Architectures:** `amd64` · `386` · `arm64` (aarch64) · `armv7` · `armv6` · `armv5` · `s390x`.

## Database Options

OMEGA/3x-ui supports two backends, chosen during the install:

- **SQLite** (default) — a single file at `/etc/x-ui/x-ui.db`. Zero setup, ideal for small and medium deployments.
- **PostgreSQL** — recommended for high client counts or multi-node setups. The installer can install PostgreSQL locally for you, or accept a DSN to an existing server.

At runtime the backend is selected via environment variables (the installer writes these to `/etc/default/x-ui` for you):

```
XUI_DB_TYPE=postgres
XUI_DB_DSN=postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable
```

### Migrating an existing SQLite install to PostgreSQL

```bash
x-ui migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
# then set XUI_DB_TYPE and XUI_DB_DSN in /etc/default/x-ui and restart:
systemctl restart x-ui
```

The source SQLite file is left untouched; remove it manually once you have verified the new backend.

### Docker

This fork does not publish images; build one from the repository with the bundled `Dockerfile`, then run it:

```bash
git clone https://github.com/Dark-Sky07/OMEGA.git && cd OMEGA
docker build -t omega-panel .

docker run -d --name omega --restart unless-stopped \
  --cap-add=NET_ADMIN --cap-add=NET_RAW \
  --device /dev/net/tun:/dev/net/tun --device /dev/ppp:/dev/ppp \
  --sysctl net.ipv4.ip_forward=1 \
  -p 2053:2053 -p 1194:1194/udp -p 1194:1194/tcp \
  -p 500:500/udp -p 4500:4500/udp -p 1701:1701/udp \
  -v /etc/x-ui:/etc/x-ui omega-panel
```

`docker-compose.yml` in this repository builds the same image and keeps SQLite by default. To run with the bundled PostgreSQL service, uncomment the two `XUI_DB_*` env lines and start with the profile:

```bash
docker compose --profile postgres up -d
```

The image bundles Fail2ban (enabled by default) to enforce per-client **IP limits**. Fail2ban bans offenders with `iptables`, which requires the `NET_ADMIN` capability — `docker-compose.yml` already grants it via `cap_add`, and the `docker run` above passes it explicitly. Without it, bans are logged but never applied.

## Environment Variables

| Variable | Description | Default |
| --- | --- | --- |
| `XUI_DB_TYPE` | Database backend: `sqlite` or `postgres` | `sqlite` |
| `XUI_DB_DSN` | PostgreSQL connection string (when `XUI_DB_TYPE=postgres`) | — |
| `XUI_DB_FOLDER` | Directory for the SQLite database file | `/etc/x-ui` |
| `XUI_DB_MAX_OPEN_CONNS` | Maximum open connections (PostgreSQL pool) | — |
| `XUI_DB_MAX_IDLE_CONNS` | Maximum idle connections (PostgreSQL pool) | — |
| `XUI_INIT_WEB_BASE_PATH` | The initial URI path for the web panel | `/` |
| `XUI_ENABLE_FAIL2BAN` | Enable Fail2ban-based IP-limit enforcement | `true` |
| `XUI_LOG_LEVEL` | Log verbosity (`debug`, `info`, `warning`, `error`) | `info` |
| `XUI_DEBUG` | Enable debug mode | `false` |

## Supported Languages

The panel UI is available in 13 languages:

English · فارسی · العربية · 中文（简体） · 中文（繁體） · Español · Русский · Українська · Türkçe · Tiếng Việt · 日本語 · Bahasa Indonesia · Português (Brasil)

## Contributing

Contributions are welcome. Please read the [Contributing Guide](/CONTRIBUTING.md) before opening an issue or pull request.

For anything that is not about the reseller feature or the OMEGA branding, the upstream
[3x-ui repository](https://github.com/MHSanaei/3x-ui) and its [Wiki](https://github.com/MHSanaei/3x-ui/wiki)
are the authoritative references.

## Security notes

- The panel pins the **exact 3x-ui v3.3.1 dependency set** on purpose (`go.mod` / `frontend/package-lock.json`
  are untouched). Advisories published after that release therefore show up in CI's `govulncheck` and
  `npm audit` steps; those two steps report without failing the build. Fixing them would mean changing the
  pinned dependencies, which is outside the "3x-ui v3.3.1 + resellers only" scope — upstream's newer releases
  carry those fixes.
- Keep the panel on a private port or behind a VPN/HTTPS reverse proxy, change the default `admin/admin` login,
  and rotate reseller passwords from the **Resellers** page whenever one is shared with too many people.

## Credits & License

- **Upstream project:** [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui) — this repository is a fork of
  **3x-ui v3.3.1** and inherits its design, documentation and licence. Thanks to
  [alireza0](https://github.com/alireza0/) and every upstream contributor.
- **Added here:** the reseller (نمایندگی) feature, the OMEGA branding, and the fork-aware installer.
- **Licence:** [GPL-3.0](LICENSE) — same as upstream.

## Acknowledgment

- [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) (License: **GPL-3.0**): _Enhanced v2ray/xray and v2ray/xray-clients routing rules with built-in Iranian domains and a focus on security and adblocking._
- [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat) (License: **GPL-3.0**): _This repository contains automatically updated V2Ray routing rules based on data on blocked domains and addresses in Russia._

## Community Tools

Tools and integrations built by the community around 3x-ui.

- [terraform-provider-3x-ui](https://github.com/batonogov/terraform-provider-threexui) (License: **MIT**): _Manage inbounds, clients, panel settings, and Xray configuration as code with Terraform / OpenTofu._

## Support the upstream project

**If this project is helpful to you, you may wish to give it a** :star2: — donations go to the original 3x-ui author:

<a href="https://www.buymeacoffee.com/MHSanaei" target="_blank">
<img src="./media/default-yellow.png" alt="Buy Me A Coffee" style="height: 70px !important;width: 277px !important;" >
</a>

</br>
<a href="https://nowpayments.io/donation/hsanaei" target="_blank" rel="noreferrer noopener">
   <img src="./media/donation-button-black.svg" alt="Crypto donation button by NOWPayments">
</a>

## Stargazers over Time

[![Stargazers over time](https://starchart.cc/Dark-Sky07/OMEGA.svg?variant=adaptive)](https://starchart.cc/Dark-Sky07/OMEGA)
