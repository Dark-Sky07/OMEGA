<div align="center">

<img src="./docs/assets/phoenix-medallion.png" alt="PHOENIX Medallion" width="220">

# 🔥 PHOENIX (ققنوس)

**Reborn from Fire & Ash — Unified Multi-Protocol Proxy & VPN Sovereignty Panel**  
**Xray-core · OpenVPN · L2TP/IPsec · AmneziaWG · TUIC v5 · MASQUE · XDRIVE · Multi-Reseller Engine**

English · [فارسی](README.fa_IR.md)

[![Release](https://img.shields.io/github/v/release/Dark-Sky07/PHOENIX?include_prereleases&label=release&color=ff5500)](https://github.com/Dark-Sky07/PHOENIX/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/Dark-Sky07/PHOENIX/release.yml?label=build&color=f97316)](https://github.com/Dark-Sky07/PHOENIX/actions)
[![License](https://img.shields.io/badge/license-GPL--3.0-fbbf24.svg)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/Dark-Sky07/PHOENIX?color=ea580c)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20windows-lightgrey)](#supported-platforms)

<img src="./docs/assets/phoenix-emblem.jpg" alt="PHOENIX Armored Emblem" width="640">

**Install the latest PHOENIX release in one line** — on a fresh server, as `root`:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Dark-Sky07/PHOENIX/main/install-phoenix.sh)
```

This is the canonical automatic installer. It resolves the newest PHOENIX release from this repository, downloads the matching panel archive and release-managed scripts, and installs the `phoenix` CLI management menu. For an existing installation, run `phoenix update`.

</div>

> [!NOTE]
> Inspired by the myth of the **Phoenix (ققنوس)** — the immortal bird that ignites in solar flame and rises anew from obsidian ash — **PHOENIX** unifies Xray-core, OpenVPN, L2TP/IPsec, AmneziaWG, TUIC v5, MASQUE, and Reseller (نمایندگی) governance in one self-healing control plane while preserving existing database (`/etc/x-ui/x-ui.db`) and service paths for seamless production upgrades.

---

## :sparkles: PHOENIX Architecture & Capabilities (`v5.0.0-phoenix`)

| Category | Capability |
| --- | --- |
| 🔥 **PHOENIX Identity & Theme** | **Solar Flame & Obsidian Ash Design System** — Three custom themes (**Solar Dawn** Light, **Obsidian Ash** Dark, **Eternal Ember** Ultra-Dark OLED), custom PHOENIX golden-flame crest & armored emblem, interactive two-column Login Showcase with animated rising embers, installable **PWA**, and **`Ctrl+K` / `⌘K` Command Palette**. |
| 💻 **`phoenix` CLI Menu** | **`phoenix` Management Command** (`phoenix`, `phoenix status`, `phoenix settings`, `phoenix update`, `phoenix log`, `phoenix restart`) installed to `/usr/bin/phoenix` alongside backward-compatible `/usr/bin/x-ui`, with automatic startup self-healing (`ensurePhoenixCLI`). |
| 🌐 **Unified Multi-Protocol Core** | **Xray-core + Native Daemons** — `VLESS` (REALITY / XHTTP), `VMess`, `Trojan`, `Shadowsocks 2022`, `Hysteria2`, `WireGuard`, **XDRIVE**, **MASQUE**, embedded **TUIC v5** (QUIC), embedded **AmneziaWG 3.1**, host **OpenVPN** daemons (`.ovpn` auto-PKI), host **L2TP/IPsec** (`strongSwan` + `xl2tpd`), and **MTProto (`mtg`)**. |
| 🔀 **OpenVPN & L2TP Xray Relay** | **Loopback DNAT + Dual TCP/DNS `dokodemo-door` Relay** — Per-inbound *Route through Xray* toggle + direct **Target Outbound (`outboundTag`)** selector. Routes OpenVPN and L2TP client TCP traffic and UDP/TCP DNS queries through any Xray outbound (WARP, Windscribe, proxies, balancers) using universal `nat PREROUTING DNAT` + `SO_ORIGINAL_DST` (no `xt_TPROXY` kernel module required), while preserving PKI certificates and client attachments on inbound edits. |
| 👥 **Multi-Reseller Engine (نمایندگی)** | **Shared Inbounds, Wallets, Vouchers & Templates** — Assign any inbound simultaneously to **multiple resellers** (`inbound_resellers`) with strict per-client isolation (`reseller_clients`), reseller credit wallets & traffic tariffs, **Gift/Voucher Codes (`PHOENIX-XXXX-XXXX-XXXX`)**, **Client Templates** with fixed pricing (`price`), and full sales/ledger reporting. |
| 📱 **Subscriptions & Device Limits** | **HWID Limits, On-Hold & Multi-Format Subs** — Per-client **HWID device limits**, **On-Hold expiration** (`onHoldExpireDuration`), **Calendar renewal strategies**, **Sing-box JSON**, **Clash Meta / Mihomo YAML**, **Outline JSON**, **Fake Status/Info Nodes** (`subStatusNode`, `subExpiredNotice`), one-click **Subscription Revocation**, **Live Sub Pull Tracking** (`subLastUpdatedAt`, `subLastUserAgent`), and **Smart Host/SNI Pools** (`*.domain`, `{RAND}`, `{USERNAME}`). |
| 🛡️ **Outbound Intelligence & Iran Check-Host** | **Watchdog, Multi-Windscribe, GeoIP & Iran Check-Host** — **Smart Outbound Auto-Failover Watchdog** (automatic primary/backup outbound switching), **Multiple Windscribe WireGuard Outbounds**, **Outbound Egress IP & Country GeoIP** on test, and built-in **Iran Check-Host (`ir1`..`ir7`)** live Ping & TCP port reachability testing from Tehran, Isfahan, Shiraz, and Qom. |
| ☁️ **Scheduled Cloud Backup** | **Google Drive, S3 / Cloudflare R2 / MinIO & WebDAV** — Dedicated **Cloud Backup** Settings tab with native **Google Drive v3 (`gdrive` — Service Account JSON & OAuth2 Refresh Token)**, S3 SigV4, and WebDAV uploads, configurable schedule interval, one-click **Backup Now**, and automatic retention pruning. |
| 🔐 **Enterprise Security & Bots** | **RBAC, Scoped API Tokens, Audit Log, Auto-SSL & Bots** — Multi-admin **RBAC** (`super_admin`, `admin`, `auditor`), **Scoped API Tokens**, **Comprehensive Admin & Reseller Security Audit Log**, dedicated **LDAP** Settings tab, one-click **Let's Encrypt Auto-SSL**, **Telegram Bot Suite** (advance warnings, broadcast, invite links, personal digest, support tickets), and **Discord Webhook Notifications**. |

### OpenVPN installation and host checklist

OpenVPN is a host daemon, not an Xray component and not a file inside the `phoenix` release archive. The release installer installs the `openvpn` package, validates `/dev/net/tun`, and aborts instead of silently installing a panel that cannot serve an OpenVPN inbound. It intentionally does not enable a generic `openvpn.service`: PHOENIX creates `bin/openvpn/<inbound-id>/openvpn.conf` and starts one daemon per enabled local OpenVPN inbound.

To install or repair an existing host with the exact stable release, run as `root`:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Dark-Sky07/PHOENIX/main/install-phoenix.sh)
```

Verify the prerequisite before troubleshooting the network:

```bash
command -v openvpn
openvpn --version | head -3
test -c /dev/net/tun && echo "TUN is ready"
systemctl restart x-ui
```

The `phoenix update` script shipped in this release resolves the latest stable release tag and runs its matching installer; it no longer downloads an unpinned, possibly stale `main` installer. For an OpenVPN inbound, allow the configured port on the VPS/provider firewall using the selected transport (`udp` or `tcp`). PHOENIX enables IPv4 forwarding and maintains exact `INPUT`, `FORWARD`, and `POSTROUTING MASQUERADE` rules for the generated `10.x.x.0/24` tunnel subnet, while preserving unrelated firewall rules. If a profile imports but remains on `Trying to connect`, check `command -v openvpn`, `command -v iptables`, `/dev/net/tun`, the listener with `ss -lunpt`, and `/var/log/x-ui/phoenix.log` plus `bin/openvpn/<inbound-id>/openvpn.log`.

### L2TP/IPsec installation and host checklist

L2TP/IPsec is a host daemon, not an Xray protocol. The installer installs and verifies `strongswan`, `xl2tpd`, `ppp`, `iptables`, and `iproute2`, and checks `/dev/ppp`. Only one enabled local L2TP/IPsec inbound is allowed because the daemon group owns UDP 500 (IKE), UDP 4500 (NAT-T), UDP 1701 (L2TP), and ESP protocol 50 for clients without NAT. PPTP is not part of PHOENIX.

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

### Routing rules for OpenVPN and L2TP/IPsec clients

OpenVPN and L2TP/IPsec are served by host daemons, so their clients' packets used to be forwarded and NATed by the kernel without ever passing through Xray — nothing on the **Xray → Routing** page applied to them. PHOENIX now relays that traffic into the core: every enabled local OpenVPN/L2TP inbound gets a loopback `dokodemo-door` TPROXY listener in the generated Xray config, tagged with the inbound's own tag, and the daemons' network managers add `mangle PREROUTING` TPROXY rules (`-i tun<id>` / `-i ppp+`, matched on the pool subnet) that hand new TCP connections and all UDP datagrams to it, plus a `filter INPUT` accept for the marked packets so default-deny firewalls (ufw) do not drop them. Xray sniffs the destination like it does for its native inbounds, so basic and custom rules (domain, IP, port, protocol, `inboundTag` = the inbound's tag, balancers) apply exactly as they do for Xray inbounds. AmneziaWG already enters Xray through its embedded relay and is unaffected.

- **Opt-in, per inbound.** The OpenVPN/L2TP inbound forms have a *Route through Xray* switch that is **off by default** (existing inbounds stay on the direct path until you turn it on). Enable it on one inbound first and verify with `iptables -t mangle -S PREROUTING | grep 0x2e01` and a client; turning it off restores the direct `FORWARD`/`MASQUERADE` path. The switch only changes iptables rules and the generated Xray config, never the daemon config, so it does not restart the OpenVPN daemon (saving the L2TP form restarts the L2TP daemon, as any L2TP edit does).
- **Fail-open.** The divert rules exist only while the running core actually listens on the relay port; when Xray is stopped, restarting, or cannot start transparent sockets, the pool falls back to the direct path within one reconcile round (~10 s) and a warning is logged. Should Xray die right after starting twice in a row with relay listeners configured, they are suspended until the next forced restart.
- **Host requirements.** Linux with `iptables`, `iproute2`, `CAP_NET_ADMIN`, and the `xt_TPROXY`/`xt_socket` netfilter modules (standard on host installs; a Docker container relies on the host kernel providing them). Policy routing uses fwmark `0x2e01`, table `2601` and rule priorities `2600`/`2601`; the `2600` guard rule and a `throw` route for the pool keep the kernel's reverse-path check (which honours the fwmark on hosts where wg-quick set `net.ipv4.conf.all.src_valid_mark=1`, e.g. WireGuard/WARP tunnels) from discarding diverted packets as martians. Hosts running **firewalld** are excluded on purpose: its nftables input chain would reject the diverted packets, so while `firewalld` is active the relay stays off (direct path, one warning in the log) and resumes automatically once it is stopped.
- **Scope.** TCP and UDP only — ICMP (ping) still takes the direct path. Connections that were already established before the relay activated keep the direct path until they close. Traffic keeps being accounted by the daemons; the core's counters for the relay tags are discarded so nothing is counted twice.

### L2TP/IPsec feature highlights

- **One global listener:** the panel accepts only one L2TP/IPsec inbound per local host. UDP 500 and 4500 are used by IKE/NAT-T and UDP 1701 by L2TP.
- **Existing clients:** attach clients from the normal Clients page; email becomes the PPP username and the existing password becomes the MS-CHAPv2 secret. Enable/disable, password edits, attach, detach, bulk delete, and quota enforcement are reconciled without putting L2TP into the Xray configuration.
- **Managed networking:** PHOENIX enables IPv4 forwarding and maintains exact `INPUT`, `FORWARD`, and `POSTROUTING MASQUERADE` rules for the configured pool. Firewall state is persisted so restart and cleanup can reclaim rules safely.
- **Restart-safe lifecycle:** daemon files live under `bin/l2tp/<inbound-id>/`; strongSwan/xl2tpd are started, stopped, reconciled after settings changes, and orphaned processes are reclaimed after a panel restart.
- **No profile files:** the panel displays PSK, pool, DNS, and port values as native connection parameters. PPTP is intentionally out of scope.

---

## :shield: AmneziaWG

PHOENIX's AmneziaWG support is a native, end-to-end protocol path. It is not limited to adding an option to the
inbound form:

- **Server and peer lifecycle:** server keys, client keys, allowed-IP allocation, pre-shared keys, keepalive,
  enable/disable, add/edit/delete, attach/detach, bulk operations, and restart-safe reconciliation are handled by
  the same client/inbound services used by the rest of PHOENIX.
- **AWG parameters:** `Jc`, `Jmin`, `Jmax`, `S1`–`S4`, `H1`–`H4`, `I1`–`I5`, and the AWG 3.1 fields such as
  `HeaderProtectionKey`, `ContentPaddingAddition`, rekey/reject timing, random trailers, and cookie controls are
  validated and passed to the running AmneziaWG engine.
- **Panel features:** per-client status, handshake and traffic, quota/expiry enforcement, inbound-specific tunnel
  addresses, subscriptions, reseller visibility, client transfer/import/export, and diagnostics remain available.
- **Client profiles:** the panel generates native `[Interface]`/`[Peer]` `.conf` files. The client-details QR code
  contains the plain configuration expected by the standalone **AmneziaWG Android** app; `vpn://` is kept only for
  clients that explicitly support the AmneziaVPN URI format. The downloaded `.conf` file and QR payload carry the
  same obfuscation values.

For Android, use **Client information → AmneziaWG → QR**, or download the `.conf` file and choose **Import from file
or archive** in the AmneziaWG app. Do not paste an AmneziaVPN `vpn://` URI into the standalone AmneziaWG importer.

## :arrows_counterclockwise: Safe import/export and updates

- **Client transfer:** export includes client configuration and portable inbound-attachment identities, not destination
  traffic counters or database IDs. Import validates the complete envelope before making changes, matches clients by
  email, keeps an existing destination client authoritative, and adds missing attachments by default.
- **Inbound import:** imported inbounds receive local identities while their protocol settings, clients, enabled state,
  and relevant configuration are preserved. Existing data is not replaced or deleted implicitly; destructive attachment
  replacement requires the explicit `replaceAttachments` option.
- **Release/update path:** `dynamic_latest` resolves the newest PHOENIX tag, downloads the matching Xray-core archive,
  validates the archive and binary before activation, and retains the previous working binary for rollback. Existing
  users are not disconnected until a new binary is ready and validated and a restart is required.

---

## :rocket: Installation

### One-line install (recommended)

On a fresh server, as **root**:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Dark-Sky07/PHOENIX/main/install-phoenix.sh)
```

The installer takes care of everything:

1. installs the required packages (`curl`, `tar`, `socat`, `openssl`, `tzdata`, cron …) plus the host `openvpn`, strongSwan, xl2tpd, PPP, iptables, and iproute2 packages,
2. verifies that `/dev/net/tun` and `/dev/ppp` are available; installation stops if the VPN prerequisite is not usable,
3. downloads the packaged release for your architecture (panel **+ Xray-core + geoip/geosite + mtg**),
4. installs it to `/usr/local/x-ui` and registers the unchanged `phoenix` systemd service,
5. keeps an existing database/settings when upgrading, and
6. restarts the panel and prints the access URL.

Defaults: port **2053**, login **admin / admin** — change both right after your first login.

### Managing the panel

```bash
phoenix           # management menu: start / stop / restart / settings / log / update / uninstall
phoenix status       # service status
phoenix settings     # current settings, including the hidden web base path
phoenix log          # panel log
phoenix update       # update to the newest PHOENIX release (never downgrades to vanilla PHOENIX)
phoenix uninstall    # full removal (the database in /etc/x-ui is kept; back it up first)
```

### Manual install

Grab `x-ui-linux-<arch>.tar.gz` from the [releases page](https://github.com/Dark-Sky07/PHOENIX/releases)
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
phoenix update                                   # upgrade in place, settings preserved
cp /etc/x-ui/x-ui.db /root/x-ui-backup.db     # or use Settings → Backup inside the panel
```

📖 **Persian step-by-step guide:** [docs/PHOENIX-INSTALL.fa.md](docs/PHOENIX-INSTALL.fa.md) — نصب، ساخت اولین نمایندگی،
بروزرسانی، بکاپ و نکات امنیتی.

---

## :briefcase: Resellers (نمایندگی)

A **reseller** is a panel sub-account that owns a slice of the panel: some of the inbounds, some of the clients,
and a defined set of quotas — while the admin keeps full control over everything.

### How it works

- **Own login.** A reseller signs in on the same login page with its own username/password. Its session can only
  reach `/panel/api/inbounds/*`, `/panel/api/clients/*`, `/panel/api/reseller/*` and `/panel/api/auth/me` — every
  other endpoint answers `403`, and the panel-wide WebSocket feed is admin-only.
- **Scoped ownership.** Inbounds and clients have separate attachment relationships. An inbound association alone never exposes all of that inbound's clients: a reseller sees only clients explicitly attached to it; anything else answers `inbound not found` / `client not found`.
  The admin attachment picker supports multi-select/bulk attach and detach, while individual client assignments remain available for shared inbounds.
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
| **Admin** | **Resellers** (list, create/edit, quotas, assign inbounds, assign clients, balance, reset password, report drawer) + everything PHOENIX has |
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

Panel pages inherited from PHOENIX v3.3.1 (the reseller pages follow the same design):

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
  <source media="(prefers-color-scheme: dark)" srcset="./media/09-add-reseller-dark.png">
  <img alt="Add reseller" src="./media/09-add-reseller-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/05-add-nodes-dark.png">
  <img alt="Configs" src="./media/05-add-nodes-light.png">
</picture>

</details>

---

## Features

Inherited from PHOENIX v3.3.1, untouched:

- **Multi-protocol inbounds** — VLESS, VMess, Trojan, Shadowsocks, WireGuard, Hysteria2, HTTP, SOCKS (Mixed), Dokodemo-door / Tunnel, and TUN. Native AmneziaWG is documented above as an PHOENIX addition.
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

PHOENIX/PHOENIX supports two backends, chosen during the install:

- **SQLite** (default) — a single file at `/etc/x-ui/x-ui.db`. Zero setup, ideal for small and medium deployments.
- **PostgreSQL** — recommended for high client counts or multi-node setups. The installer can install PostgreSQL locally for you, or accept a DSN to an existing server.

At runtime the backend is selected via environment variables (the installer writes these to `/etc/default/x-ui` for you):

```
XUI_DB_TYPE=postgres
XUI_DB_DSN=postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable
```

### Migrating an existing SQLite install to PostgreSQL

```bash
phoenix migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
# then set XUI_DB_TYPE and XUI_DB_DSN in /etc/default/x-ui and restart:
systemctl restart x-ui
```

The source SQLite file is left untouched; remove it manually once you have verified the new backend.

### Docker

PHOENIX publishes multi-architecture images to [GHCR](https://ghcr.io/dark-sky07/phoenix) for release tags. To build the same image locally instead, use the bundled `Dockerfile`:

```bash
git clone https://github.com/Dark-Sky07/PHOENIX.git && cd phoenix
docker build -t phoenix-panel .

docker run -d --name phoenix --restart unless-stopped \
  --cap-add=NET_ADMIN --cap-add=NET_RAW \
  --device /dev/net/tun:/dev/net/tun --device /dev/ppp:/dev/ppp \
  --sysctl net.ipv4.ip_forward=1 \
  -p 2053:2053 -p 1194:1194/udp -p 1194:1194/tcp \
  -p 500:500/udp -p 4500:4500/udp -p 1701:1701/udp \
  -v /etc/x-ui:/etc/x-ui phoenix-panel
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

For anything that is not about the reseller feature or the PHOENIX branding, the upstream
[PHOENIX repository](https://github.com/Dark-Sky07/PHOENIX) and its [Wiki](https://github.com/Dark-Sky07/PHOENIX/wiki)
are the authoritative references.

## Security notes

- The panel pins the **exact PHOENIX v3.3.1 dependency set** on purpose (`go.mod` / `frontend/package-lock.json`
  are untouched). Advisories published after that release therefore show up in CI's `govulncheck` and
  `npm audit` steps; those two steps report without failing the build. Fixing them would mean changing the
  pinned dependencies, which is outside the "PHOENIX v3.3.1 + resellers only" scope — upstream's newer releases
  carry those fixes.
- Keep the panel on a private port or behind a VPN/HTTPS reverse proxy, change the default `admin/admin` login,
  and rotate reseller passwords from the **Resellers** page whenever one is shared with too many people.

## Credits & License

- **Upstream project:** [Dark-Sky07/PHOENIX](https://github.com/Dark-Sky07/PHOENIX) — this repository is a fork of
  **PHOENIX v3.3.1** and inherits its design, documentation and licence. Thanks to
  [alireza0](https://github.com/alireza0/) and every upstream contributor.
- **Added here:** reseller (نمایندگی) controls, native AmneziaWG, OpenVPN and L2TP/IPsec integrations, traffic/quota lifecycle support, safe release/update handling, PHOENIX branding, and the fork-aware latest-release installer.
- **Licence:** [GPL-3.0](LICENSE) — same as upstream.

## Acknowledgment

- [Iran v2ray rules](https://github.com/chocolate4u/Iran-v2ray-rules) (License: **GPL-3.0**): _Enhanced v2ray/xray and v2ray/xray-clients routing rules with built-in Iranian domains and a focus on security and adblocking._
- [Russia v2ray rules](https://github.com/runetfreedom/russia-v2ray-rules-dat) (License: **GPL-3.0**): _This repository contains automatically updated V2Ray routing rules based on data on blocked domains and addresses in Russia._

## Community Tools

Tools and integrations built by the community around PHOENIX.

- [terraform-provider-PHOENIX](https://github.com/batonogov/terraform-provider-threexui) (License: **MIT**): _Manage inbounds, clients, panel settings, and Xray configuration as code with Terraform / OpenTofu._


## Stargazers over Time

[![Stargazers over time](https://starchart.cc/Dark-Sky07/PHOENIX.svg?variant=adaptive)](https://starchart.cc/Dark-Sky07/PHOENIX)
