[English](/README.md) | فارسی | [العربية](/README.ar_EG.md) | [中文](/README.zh_CN.md) | [Español](/README.es_ES.md) | [Русский](/README.ru_RU.md) | [Türkçe](/README.tr_TR.md)

<div align="center">

<img src="./media/omega-banner.png" alt="OMEGA" width="100%">

# OMEGA

**پنل مدیریت سرورهای Xray-core، بر پایه‌ی [3x-ui](https://github.com/MHSanaei/3x-ui) نسخه‌ی `v3.3.1`، به‌اضافه‌ی یک قابلیت: [نمایندگی‌ها](#-نمایندگی-نمایندگی).**

[English](README.md) · فارسی

[![Release](https://img.shields.io/github/v/release/Dark-Sky07/OMEGA?include_prereleases&label=release&color=blue)](https://github.com/Dark-Sky07/OMEGA/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/Dark-Sky07/OMEGA/release.yml?label=build)](https://github.com/Dark-Sky07/OMEGA/actions)
[![License](https://img.shields.io/badge/license-GPL--3.0-blue.svg)](LICENSE)
[![Base](https://img.shields.io/badge/base-3x--ui%20v3.3.1-informational)](https://github.com/MHSanaei/3x-ui/releases/tag/v3.3.1)

**نصب با یک دستور** — روی سرور تازه، با کاربر `root`:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Dark-Sky07/OMEGA/v3.3.13-omega/install-omega.sh) v3.3.13-omega
```

</div>

> [!NOTE]
> این پروژه یک فورک است: هسته‌ی پنل دقیقاً **3x-ui نسخه‌ی 3.3.1** است. فقط قابلیت نمایندگی اضافه شده و نام
> نمایشی پنل به OMEGA تغییر کرده. نام سرویس (`x-ui`)، مسیرهای نصب (`/usr/local/x-ui`، `/etc/x-ui`)،
> متغیرهای محیطی، فرمت کانفیگ و شماره‌ی نسخه (`3.3.1`) دست‌نخورده‌اند؛ یعنی هر آموزش و ابزاری که برای 3x-ui
> می‌شناسید اینجا هم کار می‌کند.

---

## :sparkles: نسبت به 3x-ui اصلی چه فرقی دارد؟

| | تغییر |
| --- | --- |
| ➕ **اضافه‌شده** | **نمایندگی‌ها** — زیرحساب‌هایی با ورود مستقل، مالکیت محدود روی ورودی/کلاینت، سهمیه‌ی ترافیک و تعداد، و گزارش فروش و حساب. |
| ➕ **اضافه‌شده** | **اینباندهای OpenVPN** — برای هر اینباند یک daemon محلی OpenVPN روی پورت آن گوش می‌کند؛ گواهی هر کلاینت خودکار ساخته می‌شود (CN = ایمیل)، هر کلاینت پروفایل آماده‌ی `.ovpn` دارد (کپی/دانلود از صفحه‌ی اطلاعات کلاینت) و ترافیک و آنلاین بودن هر کلاینت وارد همان خط لوله‌ی آمار می‌شود. installer نسخه‌ی tag‌شده بسته‌ی سیستم‌عامل `openvpn` و `/dev/net/tun` را نصب و بررسی می‌کند؛ سپس پنل برای هر اینباند محلی فعال، کانفیگ و daemon مستقل می‌سازد. فایل tar پنل به‌تنهایی بسته‌ی OpenVPN سیستم‌عامل را شامل نمی‌شود؛ باید installer اجرا شود. |
| 🎨 **برندینگ** | نام پنل در سایدبار، صفحه‌ی ورود، عنوان صفحه‌ها، مستندات API و ترجمه‌ها **OMEGA** است. فقط ظاهر — بدون تغییر در مسیرها، نام سرویس و شماره‌ی نسخه. |
| 🛠 **نصب** | [`install-omega.sh`](install-omega.sh) همین پنل را از همین ریپازیتوری نصب می‌کند و [`x-ui.sh`](x-ui.sh) هم از همین‌جا آپدیت می‌گیرد؛ بنابراین `x-ui update` هرگز پنل را با نسخه‌ی خام 3x-ui عوض نمی‌کند. |
| ✅ **بدون تغییر** | بقیه‌ی همه‌چیز — تمام 3x-ui نسخه‌ی 3.3.1 (پروتکل‌ها، ترنسپورت‌ها، نودها، اشتراک‌ها، ربات تلگرام، روتینگ، API، تم‌ها و ۱۳ زبان). |

### نصب OpenVPN و چک‌لیست سمت سرور

OpenVPN یک daemon سیستم‌عامل است، نه بخشی از Xray و نه فایلی داخل tar پنل. installer نسخه‌ی release بسته‌ی `openvpn` را نصب، وجود `/dev/net/tun` را بررسی و در صورت شکست متوقف می‌شود؛ بنابراین پنل بی‌صدا بدون قابلیت VPN نصب نمی‌شود. پنل عمداً سرویس عمومی `openvpn.service` را فعال نمی‌کند؛ برای هر اینباند محلی فعال، فایل `bin/openvpn/<inbound-id>/openvpn.conf` و یک daemon مستقل می‌سازد.

برای نصب یا تعمیر سرور موجود با آخرین release پایدار، با کاربر `root` اجرا کنید:

```bash
curl -fsSL https://raw.githubusercontent.com/Dark-Sky07/OMEGA/v3.3.13-omega/install-omega.sh \
  -o /tmp/install-omega.sh
env OMEGA_REF=v3.3.13-omega bash /tmp/install-omega.sh v3.3.13-omega
```

پیش‌نیاز را قبل از بررسی شبکه verify کنید:

```bash
command -v openvpn
openvpn --version | head -3
test -c /dev/net/tun && echo "TUN آماده است"
systemctl restart x-ui
```

اسکریپت `x-ui update` که در این release قرار دارد، آخرین tag پایدار را پیدا و installer همان tag را اجرا می‌کند و دیگر installer قدیمی و بدون tag را از `main` نمی‌گیرد. برای اینباند OpenVPN، پورت تنظیم‌شده را با transport انتخابی (`udp` یا `tcp`) هم در فایروال سرور و هم در فایروال ارائه‌دهندهٔ VPS باز کنید. اگر گزینهٔ **Redirect gateway** روشن است، روی سرور باید IPv4 forwarding و NAT/masquerade برای subnet تونل `10.x.x.0/24` هم تنظیم شده باشد؛ پنل policy فایروال شما را خودکار بازنویسی نمی‌کند. اگر profile import می‌شود اما روی `Trying to connect` می‌ماند، این موارد را بررسی کنید: `command -v openvpn`، وجود `/dev/net/tun`، خروجی `ss -lunpt`، فایل `/var/log/x-ui/3xui.log` و فایل `bin/openvpn/<inbound-id>/openvpn.log`.

---

## :rocket: نصب

### نصب سریع (پیشنهادی)

روی سرور تازه، با کاربر **root**:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Dark-Sky07/OMEGA/v3.3.13-omega/install-omega.sh) v3.3.13-omega
```

اگر می‌خواهید نسخه‌ی مشخصی نصب شود (مثلاً قبل از merge شدن شاخه‌ی اصلی):

```bash
bash <(curl -Ls https://raw.githubusercontent.com/Dark-Sky07/OMEGA/v3.3.13-omega/install-omega.sh) v3.3.13-omega
```

نصب‌کننده خودش این کارها را انجام می‌دهد:

1. نصب پیش‌نیازها (`curl`، `tar`، `socat`، `openssl`، `tzdata`، کرون و …) به‌علاوه‌ی بسته‌ی سیستم‌عامل `openvpn`
2. بررسی وجود `/dev/net/tun`؛ اگر پیش‌نیاز OpenVPN قابل استفاده نباشد نصب متوقف می‌شود
3. دانلود پکیج آماده‌ی معماری سرور (پنل **+ Xray-core + geoip/geosite + mtg**)
4. نصب در `/usr/local/x-ui` و ساخت سرویس systemd با همان نام `x-ui`
5. در آپدیت، **دیتابیس و تنظیمات قبلی حفظ می‌شود**
6. ری‌استارت پنل و نمایش آدرس ورود

پیش‌فرض‌ها: پورت **2053**، یوزر/پس **admin / admin** — بلافاصله بعد از اولین ورود هر دو را عوض کنید.

### مدیریت پنل

```bash
x-ui              # منوی مدیریت: شروع / توقف / ری‌استارت / تنظیمات / لاگ / آپدیت / حذف
x-ui status       # وضعیت سرویس
x-ui settings     # تنظیمات فعلی و مسیر مخفی ورود
x-ui log          # لاگ پنل
x-ui update       # آپدیت به آخرین نسخه‌ی OMEGA (هرگز به نسخه‌ی خام 3x-ui برنمی‌گردد)
x-ui uninstall    # حذف کامل (دیتابیس در /etc/x-ui می‌ماند؛ قبلش بکاپ بگیرید)
```

### نصب دستی

از [صفحه‌ی ریلیزها](https://github.com/Dark-Sky07/OMEGA/releases) فایل `x-ui-linux-<arch>.tar.gz` معماری خودتان را
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
x-ui update                                   # آپدیت درجا، تنظیمات حفظ می‌شود
cp /etc/x-ui/x-ui.db /root/x-ui-backup.db     # یا از بخش تنظیمات → بکاپ در خود پنل
```

📖 **راهنمای فارسی گام‌به‌گام:** [docs/OMEGA-INSTALL.fa.md](docs/OMEGA-INSTALL.fa.md)

---

## :briefcase: نمایندگی‌ها

**نماینده** یک زیرحساب پنل است که بخشی از پنل را در اختیار دارد: تعدادی ورودی، تعدادی کلاینت و سهمیه‌های مشخص —
در حالی که کنترل کامل پنل همچنان دست مدیر است.

### چطور کار می‌کند

- **ورود مستقل.** نماینده با یوزر/پس خودش در همان صفحه‌ی ورود لاگین می‌کند. نشست او فقط به
  `/panel/api/inbounds/*`، `/panel/api/clients/*`، `/panel/api/reseller/*` و `/panel/api/auth/me` دسترسی دارد؛
  بقیه‌ی آدرس‌ها `403` می‌دهند و فید وب‌سوکت پنل هم فقط برای مدیر است.
- **مالکیت محدود.** هر ورودی را می‌توان به یک نماینده واگذار کرد و کلاینت‌های تک‌نفره را هم می‌توان مستقیم
  واگذار کرد. نماینده فقط دارایی خودش را می‌بیند؛ دسترسی به دارایی دیگران با `inbound not found` /
  `client not found` رد می‌شود. مالکیت در جدول‌های نگاشت ذخیره می‌شود و جداول اصلی ورودی/کلاینت دست‌نخورده می‌مانند.
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
| **مدیر** | **نمایندگی‌ها** (لیست، ساخت/ویرایش، سهمیه‌ها، واگذاری ورودی، واگذاری کلاینت، موجودی، بازنشانی رمز، گزارش) + همه‌ی صفحه‌های 3x-ui |
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

صفحه‌های ارث‌بری‌شده از 3x-ui نسخه‌ی 3.3.1 (صفحه‌های نمایندگی با همان طراحی ساخته شده‌اند):

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

## امکانات (ارث‌بری از 3x-ui، دست‌نخورده)

- **ورودی‌های چندپروتکلی** — VLESS، VMess، Trojan، Shadowsocks، WireGuard، Hysteria2، HTTP، SOCKS (Mixed)، Dokodemo-door / Tunnel و TUN.
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
x-ui migrate-db --dsn "postgres://xui:password@127.0.0.1:5432/xui?sslmode=disable"
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

- **پروژه‌ی اصلی:** [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui) — این ریپازیتوری فورک **3x-ui نسخه‌ی 3.3.1** است
  و طراحی، مستندات و مجوز آن را به ارث می‌برد. سپاس از [alireza0](https://github.com/alireza0/) و همه‌ی مشارکت‌کنندگان.
- **اضافه‌شده اینجا:** قابلیت نمایندگی، برندینگ OMEGA و نصب‌کننده‌ی فورک‌محور.
- **مجوز:** [GPL-3.0](LICENSE) — مانند پروژه‌ی اصلی.

<h2 align="center">حمایت از پروژه‌ی اصلی</h2>

<p align="center">
<b>اگر این پروژه برایتان مفید بود، یک</b> :star2: <b>بدهید — حمایت مالی به نویسنده‌ی اصلی 3x-ui می‌رسد:</b>
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
