[English](/README.md) | فارسی | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

<div align="center">

<img src="./docs/assets/phoenix-medallion.png" alt="PHOENIX Medallion" width="220">

# 🔥 PHOENIX (ققنوس)

**برخاسته از آتش و خاکستر — پنل یکپارچه و جاودان مدیریت سرورهای پراکسی و VPN**  
**Xray-core · OpenVPN · L2TP/IPsec · AmneziaWG · TUIC v5 · MASQUE · XDRIVE · سامانه جامع نمایندگی**

[English](README.md) · فارسی

[![Release](https://img.shields.io/github/v/release/Dark-Sky07/phoenix?include_prereleases&label=release&color=ff5500)](https://github.com/Dark-Sky07/phoenix/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/Dark-Sky07/phoenix/release.yml?label=build&color=f97316)](https://github.com/Dark-Sky07/phoenix/actions)
[![License](https://img.shields.io/badge/license-GPL--3.0-fbbf24.svg)](LICENSE)

<img src="./docs/assets/phoenix-emblem.jpg" alt="PHOENIX Armored Emblem" width="640">

**نصب آخرین نسخه‌ی PHOENIX با یک دستور** — روی سرور تازه، با کاربر `root`:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Dark-Sky07/phoenix/main/install-phoenix.sh)
```

این لینک، نصب‌کننده‌ی رسمی و خودکار **PHOENIX** است: جدیدترین نسخه‌ی همین مخزن را دریافت و نصب می‌کند و دستور منوی مدیریت **`phoenix`** را در سیستم ثبت می‌نماید. برای آپدیت سرورهای موجود کافیست دستور `phoenix update` را اجرا کنید.

</div>

> [!NOTE]
> با الهام از اسطوره‌ی **ققنوس (PHOENIX)** — پرنده‌ی جاودانه‌ای که از دل آتش و خاکستر سر برمی‌آورد — پنل **PHOENIX** هسته‌های Xray-core، OpenVPN، L2TP/IPsec، AmneziaWG، TUIC v5، MASQUE و سامانه‌ی نمایندگی را در یک دژ یکپارچه و خودترمیم‌گر متحد می‌سازد و در عین حال مسیر دیتابیس (`/etc/x-ui/x-ui.db`) و سرویس‌های موجود را بدون کوچک‌ترین اختلال حفظ می‌کند.

---

## :sparkles: معماری و قابلیت‌های PHOENIX (`v5.0.0-phoenix`)

| دسته‌بندی | قابلیت‌ها |
| --- | --- |
| 🔥 **هویت بصری و تم ققنوس (`PHOENIX`)** | **سیستم طراحی Solar Flame & Obsidian Ash** — سه تم اختصاصی (**Solar Dawn** روشن، **Obsidian Ash** تیره و **Eternal Ember** اولترا-دارک OLED)، نشان طلایی-آتشین و قاب زره‌پوش ققنوس، صفحه‌ی ورود دو-ستونه با اخگرهای متحرک، وب‌اپلیکیشن قابل نصب (**PWA**) و **Command Palette (`Ctrl+K` / `⌘K`)**. |
| 💻 **منوی خط فرمان `phoenix`** | **دستور مدیریت `phoenix`** (`phoenix`, `phoenix status`, `phoenix settings`, `phoenix update`, `phoenix log`, `phoenix restart`) در مسیر `/usr/bin/phoenix` در کنار حفظ سازگاری کامل با `/usr/bin/x-ui` و خودترمیمی خودکار در لحظه‌ی بالا آمدن پنل (`ensurePhoenixCLI`). |
| 🌐 **هسته‌ی چندپروتکلی یکپارچه** | **Xray-core + دیمون‌های بومی** — پشتیبانی کامل از `VLESS` (REALITY / XHTTP)، `VMess`، `Trojan`، `Shadowsocks 2022`، `Hysteria2`، `WireGuard`، **XDRIVE**، **MASQUE**، پروتکل توکار **TUIC v5** (QUIC)، پروتکل توکار **AmneziaWG 3.1**، دیمون **OpenVPN** (با تولید خودکار گواهی و `.ovpn`)، دیمون **L2TP/IPsec** (`strongSwan` + `xl2tpd`) و **MTProto (`mtg`)**. |
| 🔀 **عبور OpenVPN و L2TP از Xray** | **رله‌ی Loopback DNAT + دوگانه‌ی TCP/DNS `dokodemo-door`** — سوئیچ *عبور ترافیک از مسیر Xray* به همراه انتخابگر مستقیم **خروجی مقصد (`outboundTag`)** در فرم اینباند؛ هدایت ترافیک TCP و درخواست‌های DNS کلاینت‌های OpenVPN و L2TP از طریق هر Outbound دلخواه (WARP، Windscribe، پراکسی یا Balancer) با استفاده از `nat PREROUTING DNAT` و `SO_ORIGINAL_DST` (بدون نیاز به ماژول کرنل `xt_TPROXY`) و حفظ کامل گواهی‌های PKI و کلاینت‌ها هنگام ویرایش اینباند. |
| 👥 **سامانه جامع نمایندگی (Resellers)** | **اینباندهای اشتراکی، کیف پول، کد هدیه و قالب کلاینت** — امکان اشتراک‌گذاری هم‌زمان یک اینباند بین **چندین نماینده** (`inbound_resellers`) با تفکیک کامل کلاینت‌ها (`reseller_clients`)، کیف پول اعتباری و تعرفه‌ی ترافیک، **کدهای هدیه و ووچر (`PHOENIX-XXXX-XXXX-XXXX`)**، **قالب‌های آماده کلاینت (Templates)** با قیمت ثابت (`price`) و گزارش‌های جامع فروش و تراکنش‌ها. |
| 📱 **مدیریت پیشرفته کلاینت و اشتراک (Sub)** | **محدودیت دستگاه (HWID)، اشتراک زمان‌دار از اولین اتصال و فرمت‌های متنوع** — محدودیت تعداد دستگاه (**HWID Device Limit**)، انقضای **On-Hold** (`onHoldExpireDuration`)، استراتژی‌های تمدید تقویمی، خروجی‌های **Sing-box JSON**، **Clash Meta / Mihomo YAML**، **Outline JSON**، **کانفیگ نمایشی وضعیت اشتراک (`subStatusNode`, `subExpiredNotice`)**، ابطال آنی لینک اشتراک (**Sub Revoke**)، ثبت **آخرین زمان و نرم‌افزار دریافت اشتراک** (`subLastUpdatedAt`, `subLastUserAgent`) و **قالب‌های هوشمند دامنه/SNI** (`*.domain`, `{RAND}`, `{USERNAME}`). |
| 🛡️ **هوشمندی Outbound و تست پینگ/فیلترینگ ایران** | **واچ‌داگ خودکار، چند خروجی Windscribe، تشخیص IP/کشور و Check-Host ایران** — **واچ‌داگ هوشمند جایگزینی خودکار Outbound** (سوییچ خودکار به خروجی پشتیبان در صورت قطعی)، امکان ساخت **چندین خروجی WireGuard از Windscribe**، نمایش **IP خروجی و کشور (GeoIP)** در تست Outbound و ابزار داخلی **تست پینگ و پورت ایران (`Check-Host` روی نودهای تهران، اصفهان، شیراز و قم)**. |
| ☁️ **بکاپ ابری خودکار (Cloud Backup)** | **پشتیبانی از Google Drive، S3 / Cloudflare R2 / MinIO و WebDAV** — تب اختصاصی **بکاپ ابری** در تنظیمات با پشتیبانی بومی از **Google Drive v3 (`gdrive` با Service Account JSON یا OAuth2 Refresh Token)**، S3 SigV4 و WebDAV، زمان‌بندی خودکار، دکمه‌ی **بکاپ آنی** و پاکسازی خودکار بکاپ‌های قدیمی بر اساس سقف نگهداری (`Retention`). |
| 🔐 **امنیت سازمانی، ربات‌ها و اتوماسیون** | **سطوح دسترسی (RBAC)، توکن‌های API محدودشده، لاگ ممیزی، Auto-SSL و ربات‌ها** — مدیریت چند ادمین با نقش‌های **`super_admin` / `admin` / `auditor`**، توکن‌های API با اسکوپ مجزا، **لاگ جامع ممیزی امنیتی (Audit Log)**، تب اختصاصی **LDAP**، دریافت خودکار گواهی **Let's Encrypt SSL** با یک کلیک، **مجموعه کامل ربات تلگرام** (هشدار پیش از اتمام حجم/زمان، پیام همگانی، لینک دعوت، گزارش شخصی، تیکت پشتیبانی) و **اعلان‌های وب‌هوک دیسکورد**. |

### نصب OpenVPN و چک‌لیست سمت سرور

OpenVPN یک daemon سیستم‌عامل است، نه بخشی از Xray و نه فایلی داخل tar پنل. installer نسخه‌ی release بسته‌ی `openvpn` را نصب، وجود `/dev/net/tun` را بررسی و در صورت شکست متوقف می‌شود؛ بنابراین پنل بی‌صدا بدون قابلیت VPN نصب نمی‌شود. پنل عمداً سرویس عمومی `openvpn.service` را فعال نمی‌کند؛ برای هر اینباند محلی فعال، فایل `bin/openvpn/<inbound-id>/openvpn.conf` و یک daemon مستقل می‌سازد.

برای نصب یا تعمیر سرور موجود با آخرین release پایدار، با کاربر `root` اجرا کنید:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Dark-Sky07/phoenix/main/install-phoenix.sh)
```

پیش‌نیاز را قبل از بررسی شبکه verify کنید:

```bash
command -v openvpn
openvpn --version | head -3
test -c /dev/net/tun && echo "TUN آماده است"
systemctl restart x-ui
```

اسکریپت `phoenix update` که در این release قرار دارد، آخرین tag پایدار را پیدا و installer همان tag را اجرا می‌کند و دیگر installer قدیمی و بدون tag را از `main` نمی‌گیرد. برای اینباند OpenVPN، پورت تنظیم‌شده را با transport انتخابی (`udp` یا `tcp`) در فایروال ارائه‌دهندهٔ VPS باز کنید. PHOENIX forwarding IPv4 را فعال می‌کند و ruleهای دقیق `INPUT`، `FORWARD` و `POSTROUTING MASQUERADE` را برای subnet تونل `10.x.x.0/24` مدیریت می‌کند و ruleهای نامرتبط فایروال را دست نمی‌زند. اگر profile import می‌شود اما روی `Trying to connect` می‌ماند، این موارد را بررسی کنید: `command -v openvpn`، `command -v iptables`، وجود `/dev/net/tun`، خروجی `ss -lunpt`، فایل `/var/log/x-ui/phoenix.log` و فایل `bin/openvpn/<inbound-id>/openvpn.log`.

### نصب و چک‌لیست L2TP/IPsec

L2TP/IPsec daemon سیستم‌عامل است و بخشی از Xray نیست. installer بسته‌های `strongswan`، `xl2tpd`، `ppp`، `iptables` و `iproute2` را نصب و وجود `/dev/ppp` را بررسی می‌کند. فقط یک اینباند محلی فعال مجاز است، چون گروه daemon پورت‌های UDP 500، 4500 و 1701 و همچنین پروتکل ESP با شمارهٔ 50 را در اختیار می‌گیرد. PPTP در PHOENIX پیاده‌سازی نشده است.

فایل‌های runtime زیر `bin/l2tp/<inbound-id>/` ساخته می‌شوند؛ forwarding IPv4، ruleهای `FORWARD` و `MASQUERADE` و listenerهای ثابت هم توسط پنل مدیریت می‌شوند. کلاینت‌های موجود را از صفحهٔ Clients متصل کنید: email نام کاربری PPP و password رمز MS-CHAPv2 است. در صفحهٔ اطلاعات اینباند PSK، pool، DNS و پورت‌ها دیده می‌شود. صفحهٔ Subscription info برای هر کلاینت متصل با همان subscription ID یک مجموعه پارامتر native نشان می‌دهد و پنجرهٔ Client Information مدیر نیز همین مقادیر را با دکمهٔ کپی نمایش می‌دهد. کلاینت را با تنظیمات native L2TP/IPsec پیکربندی کنید؛ profile فایل یا لینک جعلی Xray تولید نمی‌شود.

برای Docker باید `NET_ADMIN`، `NET_RAW`، دستگاه‌های `/dev/ppp` و `/dev/net/tun`، sysctl forwarding و publish کردن UDPهای 500، 4500 و 1701 فراهم باشد؛ تنظیمات آن در `docker-compose.yml` قرار دارد. kernel میزبان باید PPP و XFRM/IPsec را پشتیبانی کند.

### اعمال قوانین Routing روی کلاینت‌های OpenVPN و L2TP/IPsec

OpenVPN و L2TP/IPsec توسط daemonهای سیستم‌عامل سرویس داده می‌شوند؛ پیش‌تر بسته‌های کلاینت‌های آن‌ها مستقیماً توسط kernel forward و NAT می‌شد و هیچ‌وقت از Xray عبور نمی‌کرد، بنابراین قوانین صفحهٔ **Xray → Routing** روی آن‌ها اثری نداشت. حالا PHOENIX این ترافیک را به داخل هسته هدایت می‌کند: برای هر inbound محلیِ فعال OpenVPN/L2TP یک listener از نوع `dokodemo-door` (TPROXY) روی loopback با همان tag خود inbound در کانفیگ تولیدشدهٔ Xray قرار می‌گیرد و مدیر شبکهٔ daemonها ruleهای `mangle PREROUTING` TPROXY (روی `tun<id>` / `ppp+` و subnet همان pool) را اضافه می‌کند تا اتصال‌های TCP جدید و همهٔ datagramهای UDP به آن listener تحویل شوند؛ یک rule `filter INPUT` هم بسته‌های mark‌شده را می‌پذیرد تا فایروال‌های default-deny (مثل ufw) آن‌ها را drop نکنند. Xray مقصد را مثل inboundهای native خودش sniff می‌کند، پس قوانین پایه و سفارشی (domain، IP، port، protocol، `inboundTag` برابر با tag همان inbound و balancerها) دقیقاً مثل inboundهای Xray اعمال می‌شوند. AmneziaWG از قبل از طریق relay داخلی خودش وارد Xray می‌شد و تغییری نکرده است.

- **Opt-in، برای هر inbound جداگانه.** فرم inboundهای OpenVPN/L2TP یک سوئیچ *مسیریابی از طریق Xray* دارد که **به‌صورت پیش‌فرض خاموش** است (inboundهای قبلی تا وقتی خودتان روشنش نکنید روی مسیر مستقیم می‌مانند). اول روی یک inbound فعالش کنید و با `iptables -t mangle -S PREROUTING | grep 0x2e01` و یک کلاینت واقعی بررسی کنید؛ خاموش کردن آن مسیر مستقیم `FORWARD`/`MASQUERADE` را برمی‌گرداند. این سوئیچ فقط ruleهای iptables و کانفیگ تولیدشدهٔ Xray را عوض می‌کند و به کانفیگ daemon دست نمی‌زند، بنابراین daemon OpenVPN را restart نمی‌کند (ذخیرهٔ فرم L2TP مثل هر ویرایش L2TP، daemon آن را restart می‌کند).
- **Fail-open.** ruleهای هدایت فقط وقتی وجود دارند که هستهٔ در حال اجرا واقعاً روی پورت relay گوش می‌دهد؛ وقتی Xray متوقف است، در حال restart است یا نمی‌تواند سوکت transparent باز کند، pool ظرف یک دور reconcile (حدود ۱۰ ثانیه) به مسیر مستقیم برمی‌گردد و یک warning لاگ می‌شود. اگر Xray دو بار پشت سر هم بلافاصله بعد از start با listenerهای relay crash کند، relay تا restart اجباری بعدی معلق می‌شود.
- **نیازمندی‌های host.** لینوکس با `iptables`، `iproute2`، `CAP_NET_ADMIN` و ماژول‌های netfilter `xt_TPROXY`/`xt_socket` (روی نصب مستقیم استاندارد است؛ در Docker به kernel میزبان وابسته است). policy routing از fwmark `0x2e01`، جدول `2601` و ruleهای با اولویت `2600`/`2601` استفاده می‌کند؛ rule محافظ `2600` و یک route از نوع `throw` برای pool باعث می‌شوند بررسی reverse-path هستهٔ لینوکس (که روی hostهایی که wg-quick مقدار `net.ipv4.conf.all.src_valid_mark=1` را تنظیم کرده — مثل تونل‌های WireGuard/WARP — fwmark بسته را هم در نظر می‌گیرد) بسته‌های هدایت‌شده را به‌عنوان martian دور نریزد. hostهایی که **firewalld** دارند عمداً مستثنا هستند: زنجیرهٔ input آن در nftables بسته‌های هدایت‌شده را reject می‌کند، بنابراین تا وقتی `firewalld` فعال است relay خاموش می‌ماند (مسیر مستقیم، یک warning در لاگ) و به‌محض متوقف شدن آن خودکار برمی‌گردد.
- **محدودهٔ اثر.** فقط TCP و UDP؛ ICMP (ping) همچنان مسیر مستقیم را می‌رود. اتصال‌هایی که قبل از فعال شدن relay برقرار شده‌اند تا بسته شدن مسیر مستقیم را نگه می‌دارند. حساب ترافیک همچنان توسط daemonها انجام می‌شود و شمارنده‌های هسته برای tagهای relay دور ریخته می‌شوند تا چیزی دو بار شمرده نشود.

### خلاصه‌ی قابلیت‌های L2TP/IPsec

- **یک listener سراسری:** در هر host محلی فقط یک inbound از نوع L2TP/IPsec مجاز است؛ UDPهای 500 و 4500 برای IKE/NAT-T و UDP 1701 برای L2TP هستند.
- **کلاینت‌های موجود:** از صفحه‌ی عادی Clients کلاینت را متصل کنید؛ email نام کاربری PPP و password موجود رمز MS-CHAPv2 می‌شود. enable/disable، تغییر password، attach، detach، حذف گروهی و enforcement سهمیه‌ها بدون وارد کردن L2TP به کانفیگ Xray reconcile می‌شوند.
- **مدیریت شبکه:** PHOENIX IPv4 forwarding را فعال و ruleهای دقیق `INPUT`، `FORWARD` و `POSTROUTING MASQUERADE` را برای pool تنظیم‌شده مدیریت می‌کند. وضعیت firewall برای restart و cleanup پایدار می‌ماند.
- **چرخه‌ی امن پس از restart:** فایل‌های daemon زیر `bin/l2tp/<inbound-id>/` قرار می‌گیرند؛ strongSwan و xl2tpd هنگام تغییر تنظیمات reconcile می‌شوند و processهای orphan بعد از restart جمع می‌شوند.
- **بدون profile فایل:** PSK، pool، DNS و پورت‌ها به‌صورت پارامترهای native اتصال نمایش داده می‌شوند. PPTP عمداً خارج از scope است.

---

## :shield: AmneziaWG

پشتیبانی AmneziaWG در PHOENIX یک مسیر native و کامل است و فقط به اضافه کردن یک گزینه به فرم محدود نمی‌شود:

- **چرخه‌ی سرور و peer:** کلیدهای سرور و کلاینت، allowed IP، pre-shared key، keepalive، فعال/غیرفعال‌سازی، ساخت/ویرایش/حذف، attach/detach، عملیات bulk و reconcile امن پس از restart.
- **پارامترهای AWG:** `Jc`، `Jmin`، `Jmax`، `S1` تا `S4`، `H1` تا `H4`، `I1` تا `I5` و گزینه‌های AWG 3.1 مانند `HeaderProtectionKey`، padding، زمان‌بندی rekey/reject، random trailers و cookieها validate و به engine فعال منتقل می‌شوند.
- **امکانات پنل:** status، handshake و traffic هر کلاینت، quota و expiry، آدرس tunnel به‌تفکیک inbound، subscription، reseller visibility، import/export و diagnostics.
- **فایل و QR:** پنل فایل native با بخش‌های `[Interface]` و `[Peer]` می‌سازد. QR بخش اطلاعات کلاینت متن خام مورد انتظار اپ مستقل **AmneziaWG Android** را دارد؛ `vpn://` فقط برای کلاینت‌هایی است که فرمت URI خود AmneziaVPN را پشتیبانی می‌کنند.

در اندروید از مسیر **Client information → AmneziaWG → QR** استفاده کنید یا فایل `.conf` را دانلود کرده و در AmneziaWG گزینه‌ی **Import from file or archive** را بزنید. `vpn://` مخصوص AmneziaVPN را داخل importer مستقل AmneziaWG وارد نکنید.

## :arrows_counterclockwise: import/export و آپدیت امن

- **انتقال کلاینت:** export تنظیمات کلاینت و identity قابل‌حمل attachmentهای inbound را دارد، اما counterهای ترافیک مقصد و IDهای دیتابیس را overwrite نمی‌کند. import ابتدا کل envelope را validate می‌کند، کلاینت را با email پیدا می‌کند، رکورد موجود مقصد را authoritative نگه می‌دارد و به‌صورت پیش‌فرض فقط attachmentهای کمبود را اضافه می‌کند.
- **Import inbound:** inbound جدید identity محلی می‌گیرد، اما تنظیمات پروتکل، clientها و وضعیت فعال/غیرفعال حفظ می‌شوند. داده‌ی موجود بدون درخواست صریح جایگزین یا حذف نمی‌شود؛ جایگزینی attachmentها فقط با گزینه‌ی explicit `replaceAttachments` ممکن است.
- **مسیر update:** سیاست `dynamic_latest` جدیدترین tag PHOENIX را resolve می‌کند، archive و binary جدید Xray-core را قبل از فعال‌سازی validate می‌کند و binary قبلی را برای rollback نگه می‌دارد. کاربران فعال تا آماده و validate شدن binary جدید و ضرورت restart قطع نمی‌شوند.

---

## :rocket: نصب

### نصب سریع (پیشنهادی)

روی سرور تازه، با کاربر **root**:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Dark-Sky07/phoenix/main/install-phoenix.sh)
```

نصب‌کننده خودش این کارها را انجام می‌دهد:

1. نصب پیش‌نیازها (`curl`، `tar`، `socat`، `openssl`، `tzdata`، کرون و …) به‌علاوه‌ی بسته‌های `openvpn`، `strongswan`، `xl2tpd`، `ppp`، `iptables` و `iproute2`
2. بررسی وجود `/dev/net/tun` و `/dev/ppp`؛ اگر پیش‌نیاز daemonهای VPN قابل استفاده نباشد نصب متوقف می‌شود
3. دانلود پکیج آماده‌ی معماری سرور (پنل **+ Xray-core + geoip/geosite + mtg**)
4. نصب در `/usr/local/x-ui` و ساخت سرویس systemd با همان نام `phoenix`
5. در آپدیت، **دیتابیس و تنظیمات قبلی حفظ می‌شود**
6. ری‌استارت پنل و نمایش آدرس ورود

پیش‌فرض‌ها: پورت **2053**، یوزر/پس **admin / admin** — بلافاصله بعد از اولین ورود هر دو را عوض کنید.

### مدیریت پنل

```bash
phoenix           # منوی مدیریت: شروع / توقف / ری‌استارت / تنظیمات / لاگ / آپدیت / حذف
phoenix status       # وضعیت سرویس
phoenix settings     # تنظیمات فعلی و مسیر مخفی ورود
phoenix log          # لاگ پنل
phoenix update       # آپدیت به آخرین نسخه‌ی PHOENIX (هرگز به نسخه‌ی خام PHOENIX برنمی‌گردد)
phoenix uninstall    # حذف کامل (دیتابیس در /etc/x-ui می‌ماند؛ قبلش بکاپ بگیرید)
```

### نصب دستی

از [صفحه‌ی ریلیزها](https://github.com/Dark-Sky07/phoenix/releases) فایل `x-ui-linux-<arch>.tar.gz` معماری خودتان را
بگیرید (`amd64`، `arm64`، `armv7`، `armv6`، `386`، `armv5`، `s390x`) و روی سرور. استخراج دستی tar به‌تنهایی بسته‌ی OpenVPN سیستم‌عامل را نصب نمی‌کند؛ اگر این روش را استفاده می‌کنید ابتدا اجرا کنید:

```bash
apt-get update && apt-get install -y openvpn
modprobe tun 2>/dev/null || true
test -c /dev/net/tun
```

```bash
tar zxvf x-ui-linux-amd64.tar.gz
cd x-ui && chmod +x x-ui bin/xray-linux-*
./x-ui                                             # اجرای اول: ساخت دیتابیس
cp -f x-ui.service /etc/systemd/system/ 2>/dev/null \
  || cp -f x-ui.service.debian /etc/systemd/system/x-ui.service
cp -f x-ui.sh /usr/bin/x-ui && chmod +x /usr/bin/x-ui
systemctl daemon-reload && systemctl enable --now x-ui
```

### آپدیت و بکاپ

```bash
phoenix update                                   # آپدیت درجا، تنظیمات حفظ می‌شود
cp /etc/x-ui/x-ui.db /root/x-ui-backup.db     # یا از بخش تنظیمات → بکاپ در خود پنل
```

📖 **راهنمای فارسی گام‌به‌گام:** [docs/PHOENIX-INSTALL.fa.md](docs/PHOENIX-INSTALL.fa.md)

---

## :briefcase: نمایندگی‌ها

**نماینده** یک زیرحساب پنل است که بخشی از پنل را در اختیار دارد: تعدادی ورودی، تعدادی کلاینت و سهمیه‌های مشخص —
در حالی که کنترل کامل پنل همچنان دست مدیر است.

### چطور کار می‌کند

- **ورود مستقل.** نماینده با یوزر/پس خودش در همان صفحه‌ی ورود لاگین می‌کند. نشست او فقط به
  `/panel/api/inbounds/*`، `/panel/api/clients/*`، `/panel/api/reseller/*` و `/panel/api/auth/me` دسترسی دارد؛
  بقیه‌ی آدرس‌ها `403` می‌دهند و فید وب‌سوکت پنل هم فقط برای مدیر است.
- **مالکیت محدود.** رابطه‌ی attach ورودی و کلاینت جداست؛ association با inbound به‌تنهایی همه‌ی clientهای آن را برای نماینده قابل مشاهده نمی‌کند و فقط clientهای صراحتاً attach‌شده دیده می‌شوند. دسترسی به دارایی دیگران با `inbound not found` / `client not found` رد می‌شود. منوی attachment از انتخاب چندتایی و attach/detach گروهی پشتیبانی می‌کند و برای inbound مشترک، واگذاری تک‌کلاینت هم ممکن است. مالکیت در جدول‌های نگاشت ذخیره می‌شود و جداول اصلی ورودی/کلاینت دست‌نخورده می‌مانند.
- **سهمیه‌ها.** سقف ترافیک (مجموع سهمیه‌هایی که به کلاینت‌ها می‌دهد)، سقف تعداد کلاینت، سقف تعداد ورودی و
  تاریخ انقضای اختیاری. مقدار `0` یعنی *نامحدود*. سهمیه‌ها فقط در ظاهر نیستند؛ روی ساخت، ویرایش،
  عملیات گروهی و ایمپورت ورودی اعمال می‌شوند.
- **فروش و حساب.** گزارش مصرف به‌تفکیک کلاینت (سهمیه، مصرف، هزینه، ورودی‌های متصل)، قیمت هر گیگابایت،
  موجودی پیش‌پرداخت، دفتر واریز/برداشت و تاریخچه‌ی تسویه.
  موجودی = شارژ − (گیگابایت مصرف‌شده × قیمت هر گیگ).
- **غیرفعال‌سازی و بازنشانی.** غیرفعال کردن نماینده ورودش را می‌بندد و ورودی‌هایش را خاموش می‌کند؛
  بازنشانی رمز هم همه‌ی نشست‌های فعال آن نماینده را فوراً باطل می‌کند.

```mermaid
flowchart LR
    A[پنل مدیر<br/>admin] -->|واگذاری ورودی| B[نماینده: علی]
    A -->|واگذاری کلاینت مشخص| B
    A -->|سهمیه: ترافیک · کلاینت · ورودی| B
    A -->|قیمت هر گیگ · شارژ| B
    B -->|ورود مستقل| C[فقط ورودی و کلاینت‌های خودش]
    C --> D[ساخت / ویرایش / حذف کلاینت<br/>در محدوده‌ی سهمیه]
    D --> E[گزارش فروش<br/>مصرف · هزینه · موجودی · دفتر حساب]
    A --> F[مدیر همچنان همه‌چیز را می‌بیند و مدیریت می‌کند]
```

### صفحه‌های پنل

| نقش | صفحه‌ها |
| --- | --- |
| **مدیر** | **نمایندگی‌ها** (لیست، ساخت/ویرایش، سهمیه‌ها، واگذاری ورودی، واگذاری کلاینت، موجودی، بازنشانی رمز، گزارش) + همه‌ی صفحه‌های PHOENIX |
| **نماینده** | **گزارش** (مصرف، هزینه، موجودی، ردیف هر کلاینت، دفتر حساب)، **پروفایل** (سهمیه‌ها، مصرف، تغییر رمز)، **ورودی‌ها** و **کلاینت‌ها** (فقط دارایی خودش) |

### API

API مدیریتی سمت مدیر زیر `/panel/api/resellers` است و در صفحه‌ی **API Docs** خود پنل مستند شده:

| متد | مسیر | کار |
| --- | --- | --- |
| `GET` | `/panel/api/resellers/list` | همه‌ی نماینده‌ها با آمار لحظه‌ای مصرف |
| `GET` | `/panel/api/resellers/get/:id` | یک نماینده |
| `GET` | `/panel/api/resellers/assignments` | نقشه‌ی مالکیت (ورودی‌ها + کلاینت‌های واگذارشده) |
| `GET` | `/panel/api/resellers/report/:id` | گزارش مصرف + ردیف کلاینت‌ها + دفتر حساب |
| `POST` | `/panel/api/resellers/add` · `update/:id` · `del/:id` | ساخت / ویرایش / حذف |
| `POST` | `/panel/api/resellers/setEnable/:id` · `resetPassword/:id` | فعال-غیرفعال · بازنشانی رمز |
| `POST` | `/panel/api/resellers/assignInbound` · `unassignInbound` | واگذاری ورودی / پس‌گرفتن آن |
| `POST` | `/panel/api/resellers/assignClient` · `unassignClient` | واگذاری / لغو واگذاری یک کلاینت |
| `POST` | `/panel/api/resellers/balance` | واریز (+) یا برداشت (−) روی حساب نماینده |

و API مخصوص خود نماینده (با نشست نماینده):

| متد | مسیر | کار |
| --- | --- | --- |
| `GET` | `/panel/api/reseller/profile` · `stats` | حساب، سهمیه‌ها و مصرف لحظه‌ای |
| `GET` | `/panel/api/reseller/report` | گزارش فروش و دفتر حساب خودش |
| `POST` | `/panel/api/reseller/password` | تغییر رمز خودش |
| `GET` | `/panel/api/auth/me` | نقش نشست (`admin` یا `reseller`) — مورد استفاده‌ی رابط کاربری |

---

## :camera: تصاویر

صفحه‌های ارث‌بری‌شده از PHOENIX نسخه‌ی 3.3.1 (صفحه‌های نمایندگی با همان طراحی ساخته شده‌اند):

<details>
<summary>برای دیدن کلیک کنید</summary>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/01-overview-dark.png">
  <img alt="نمای کلی" src="./media/01-overview-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/02-add-inbound-dark.png">
  <img alt="ورودی‌ها" src="./media/02-add-inbound-light.png">
</picture>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./media/03-add-client-dark.png">
  <img alt="افزودن کلاینت" src="./media/03-add-client-light.png">
</picture>

</details>

---

## امکانات (ارث‌بری از PHOENIX، دست‌نخورده)

- **ورودی‌های چندپروتکلی** — VLESS، VMess، Trojan، Shadowsocks، WireGuard، Hysteria2، HTTP، SOCKS (Mixed)، Dokodemo-door / Tunnel و TUN. AmneziaWG native به‌عنوان قابلیت اضافه‌شده‌ی PHOENIX در بخش بالا مستند شده است.
- **ترنسپورت و امنیت مدرن** — TCP (Raw)، mKCP، WebSocket، gRPC، HTTPUpgrade و XHTTP همراه با TLS، XTLS و REALITY.
- **فالبک** — اجرای چند پروتکل روی یک پورت (مثلاً VLESS و Trojan روی ۴۴۳) با پشتیبانی fallback در Xray.
- **مدیریت هر کلاینت** — سهمیه‌ی ترافیک، تاریخ انقضا، محدودیت IP، وضعیت آنلاین زنده و لینک/QR/اشتراک یک‌کلیکی.
- **آمار ترافیک** — به‌تفکیک ورودی، کلاینت و خروجی، همراه با ریست.
- **پشتیبانی چند نود** — مدیریت و مقیاس‌دهی چند سرور از یک پنل.
- **خروجی و روتینگ** — WARP، NordVPN، قوانین روتینگ سفارشی، لودبالانسر و زنجیره‌ی پروکسی.
- **سرور اشتراک داخلی** با چند فرمت خروجی و [قالب‌های سفارشی](docs/custom-subscription-templates.md).
- **ربات تلگرام** برای مانیتورینگ و مدیریت از راه دور.
- **API استاندارد REST** با مستندات Swagger داخل پنل.
- **دو بک‌اند ذخیره‌سازی** — SQLite (پیش‌فرض) یا PostgreSQL.
- **۱۳ زبان رابط کاربری** با تم روشن و تیره.
- **یکپارچگی با Fail2ban** برای اعمال محدودیت IP هر کلاینت.

## پلتفرم‌های پشتیبانی‌شده

**سیستم‌عامل:** Ubuntu، Debian، Armbian، Fedora، CentOS، RHEL، AlmaLinux، Rocky Linux، Oracle Linux، Amazon Linux، Virtuozzo، Arch، Manjaro، Parch، openSUSE (Tumbleweed / Leap)، Alpine و Windows.

**معماری‌ها:** `amd64` · `386` · `arm64` (aarch64) · `armv7` · `armv6` · `armv5` · `s390x`.

## دیتابیس

- **SQLite** (پیش‌فرض) — یک فایل در `/etc/x-ui/x-ui.db`؛ بدون تنظیمات، مناسب کارهای کوچک و متوسط.
- **PostgreSQL** — برای تعداد کلاینت بالا یا چند نود؛ نصب‌کننده می‌تواند PostgreSQL را لوکال نصب کند یا DSN بگیرد.

```
XUI_DB_TYPE=postgres
XUI_DB_DSN=postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable
```

انتقال از SQLite به PostgreSQL:

```bash
phoenix migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
systemctl restart x-ui
```

فایل SQLite دست‌نخورده می‌ماند؛ بعد از اطمینان از سالم بودن بک‌اند جدید خودتان حذفش کنید.

## متغیرهای محیطی

| متغیر | توضیح | پیش‌فرض |
| --- | --- | --- |
| `XUI_DB_TYPE` | بک‌اند دیتابیس: `sqlite` یا `postgres` | `sqlite` |
| `XUI_DB_DSN` | رشته‌ی اتصال PostgreSQL (وقتی `XUI_DB_TYPE=postgres`) | — |
| `XUI_DB_FOLDER` | مسیر فایل SQLite | `/etc/x-ui` |
| `XUI_DB_MAX_OPEN_CONNS` | حداکثر اتصال باز (استخر PostgreSQL) | — |
| `XUI_DB_MAX_IDLE_CONNS` | حداکثر اتصال بی‌کار (استخر PostgreSQL) | — |
| `XUI_INIT_WEB_BASE_PATH` | مسیر اولیه‌ی ورود به پنل | `/` |
| `XUI_ENABLE_FAIL2BAN` | فعال‌سازی اعمال محدودیت IP با Fail2ban | `true` |
| `XUI_LOG_LEVEL` | سطح لاگ (`debug`، `info`، `warning`، `error`) | `info` |
| `XUI_DEBUG` | حالت دیباگ | `false` |

## دونیت و اعتبار

- **پروژه‌ی اصلی:** [Dark-Sky07/phoenix](https://github.com/Dark-Sky07/phoenix) — این ریپازیتوری فورک **PHOENIX نسخه‌ی 3.3.1** است
  و طراحی، مستندات و مجوز آن را به ارث می‌برد. سپاس از [alireza0](https://github.com/alireza0/) و همه‌ی مشارکت‌کنندگان.
- **اضافه‌شده اینجا:** قابلیت نمایندگی، AmneziaWG native، یکپارچه‌سازی OpenVPN و L2TP/IPsec، accounting و quota چرخه‌ی کلاینت، آپدیت امن، برندینگ PHOENIX و نصب‌کننده‌ی فورک‌محورِ آخرین release.
- **مجوز:** [GPL-3.0](LICENSE) — مانند پروژه‌ی اصلی.

<h2 align="center">حمایت از پروژه‌ی اصلی</h2>

<p align="center">
<b>اگر این پروژه برایتان مفید بود، یک</b> :star2: <b>بدهید — حمایت مالی به نویسنده‌ی اصلی PHOENIX می‌رسد:</b>
</p>

<p align="center">
<a href="https://www.buymeacoffee.com/MHSanaei" target="_blank">
<img src="./media/default-yellow.png" alt="Buy Me A Coffee" style="height: 70px !important;width: 277px !important;" >
</a>
</p>

<p align="center">
<a href="https://nowpayments.io/donation/hsanaei" target="_blank" rel="noreferrer noopener">
   <img src="./media/donation-button-black.svg" alt="Crypto donation button by NOWPayments">
</a>
</p>
