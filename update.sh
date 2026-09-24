#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
blue='\033[0;34m'
yellow='\033[0;33m'
plain='\033[0m'

xui_folder="${XUI_MAIN_FOLDER:=/usr/local/x-ui}"
xui_service="${XUI_SERVICE:=/etc/systemd/system}"

# Don't edit this config
b_source="${BASH_SOURCE[0]}"
while [ -h "$b_source" ]; do
    b_dir="$(cd -P "$(dirname "$b_source")" > /dev/null 2>&1 && pwd || pwd -P)"
    b_source="$(readlink "$b_source")"
    [[ $b_source != /* ]] && b_source="$b_dir/$b_source"
done
cur_dir="$(cd -P "$(dirname "$b_source")" > /dev/null 2>&1 && pwd || pwd -P)"
script_name=$(basename "$0")

# Check command exist function
_command_exists() {
    type "$1" &> /dev/null
}

# Fail, log and exit script function
_fail() {
    local msg=${1}
    echo -e "${red}${msg}${plain}"
    exit 2
}

# check root
[[ $EUID -ne 0 ]] && _fail "FATAL ERROR: Please run this script with root privilege."

if _command_exists curl; then
    curl_bin=$(which curl)
else
    _fail "ERROR: Command 'curl' not found."
fi

# Check OS and set release variable
if [[ -f /etc/os-release ]]; then
    source /etc/os-release
    release=$ID
elif [[ -f /usr/lib/os-release ]]; then
    source /usr/lib/os-release
    release=$ID
else
    _fail "Failed to check the system OS, please contact the author!"
fi
echo "The OS release is: $release"

arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) echo 'amd64' ;;
        i*86 | x86) echo '386' ;;
        armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
        armv7* | armv7 | arm) echo 'armv7' ;;
        armv6* | armv6) echo 'armv6' ;;
        armv5* | armv5) echo 'armv5' ;;
        s390x) echo 's390x' ;;
        *) echo -e "${red}Unsupported CPU architecture!${plain}" && rm -f "${cur_dir}/${script_name}" > /dev/null 2>&1 && exit 2 ;;
    esac
}

echo "Arch: $(arch)"

# Resolve and stage the current Xray-core before stopping the panel.
# This keeps failures observable and prevents a broken download from replacing
# the currently installed core.
xray_update_archive=""
xray_update_version=""
panel_update_archive=""
update_stage_root=""
update_stage_dir=""
update_wrapper_stage=""
update_service_stage=""
update_backup_dir=""
update_service_backup=""
update_wrapper_backup=""
update_old_service_present=0
update_old_service_active=0
update_old_service_enabled=0
update_commit_started=0
update_transaction_active=0
update_db_kind=""
update_db_dsn=""
update_db_path=""
update_db_snapshot=""
update_db_snapshot_dir=""
update_db_snapshot_ready=0
update_defer_service_start=0
resolve_latest_xray_version() {
    local releases version
    releases="$(curl -4fsSL --retry 3 --connect-timeout 10 "https://api.github.com/repos/XTLS/Xray-core/releases?per_page=100")" || return 1
    version="$(printf '%s\n' "$releases" \
        | grep -oE '"tag_name"[[:space:]]*:[[:space:]]*"v[0-9]+\.[0-9]+\.[0-9]+"' \
        | sed -E 's/.*"(v[0-9]+\.[0-9]+\.[0-9]+)"/\1/' \
        | sort -V \
        | tail -n 1)"
    [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
    printf '%s\n' "$version"
}

stage_latest_xray() {
    local xray_arch archive_url
    xray_update_version="${OMEGA_XRAY_VERSION:-$(resolve_latest_xray_version)}" || _fail "ERROR: Failed to resolve the latest Xray-core release."
    case "$(arch)" in
        amd64) xray_arch="64" ;;
        386) xray_arch="32" ;;
        arm64) xray_arch="arm64-v8a" ;;
        armv7) xray_arch="arm32-v7a" ;;
        armv6) xray_arch="arm32-v6" ;;
        armv5) xray_arch="arm32-v5" ;;
        s390x) xray_arch="s390x" ;;
        *) _fail "ERROR: Unsupported architecture for Xray-core: $(arch)" ;;
    esac
    archive_url="https://github.com/XTLS/Xray-core/releases/download/${xray_update_version}/Xray-linux-${xray_arch}.zip"
    xray_update_archive=$(mktemp "/tmp/xray-${xray_update_version#v}.XXXXXX.zip")
    if ! ${curl_bin} -fsSL --retry 3 -o "$xray_update_archive" "$archive_url"; then
        rm -f "$xray_update_archive"
        xray_update_archive=""
        _fail "ERROR: Failed to download Xray-core ${xray_update_version}; panel update aborted safely."
    fi
    if ! unzip -tq "$xray_update_archive" >/dev/null 2>&1; then
        rm -f "$xray_update_archive"
        xray_update_archive=""
        _fail "ERROR: Downloaded Xray-core archive is invalid; panel update aborted safely."
    fi
    echo -e "${green}Staged latest Xray-core ${xray_update_version}${plain}"
}

install_staged_xray() {
    local stage_dir="$1" extract_dir target
    [ -n "$xray_update_archive" ] || _fail "ERROR: No staged Xray-core archive is available."
    [ -d "$stage_dir" ] || _fail "ERROR: Xray staging directory does not exist."
    extract_dir=$(mktemp -d "/tmp/xray-extract.XXXXXX")
    if ! unzip -q "$xray_update_archive" -d "$extract_dir" || [ ! -f "$extract_dir/xray" ]; then
        rm -rf "$extract_dir" "$xray_update_archive"
        xray_update_archive=""
        _fail "ERROR: Failed to extract staged Xray-core ${xray_update_version}."
    fi
    mkdir -p "$stage_dir/bin"
    target="${stage_dir}/bin/xray-linux-$(arch)"
    if ! install -m 0755 "$extract_dir/xray" "$target"; then
        rm -rf "$extract_dir" "$xray_update_archive"
        xray_update_archive=""
        _fail "ERROR: Failed to install Xray-core ${xray_update_version}."
    fi
    rm -rf "$extract_dir" "$xray_update_archive"
    xray_update_archive=""
    echo -e "${green}Installed Xray-core ${xray_update_version} in staged panel${plain}"
}

# Simple helpers
is_ipv4() {
    [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] && return 0 || return 1
}
is_ipv6() {
    [[ "$1" =~ : ]] && return 0 || return 1
}
is_ip() {
    is_ipv4 "$1" || is_ipv6 "$1"
}
is_domain() {
    [[ "$1" =~ ^([A-Za-z0-9](-*[A-Za-z0-9])*\.)+(xn--[a-z0-9]{2,}|[A-Za-z]{2,})$ ]] && return 0 || return 1
}

# acme.sh's standalone server binds IPv4 by default; --listen-v6 makes it
# v6-only, which breaks HTTP-01 validation when the domain's A record points
# at this host's IPv4 (#4994). Only force IPv6 when the host has no global
# IPv4 address at all.
acme_listen_flag() {
    if ip -4 addr show scope global 2> /dev/null | grep -q "inet "; then
        echo ""
    else
        echo "--listen-v6"
    fi
}

# Port helpers
is_port_in_use() {
    local port="$1"
    if command -v ss > /dev/null 2>&1; then
        ss -ltn 2> /dev/null | awk -v p=":${port}$" '$4 ~ p {exit 0} END {exit 1}'
        return
    fi
    if command -v netstat > /dev/null 2>&1; then
        netstat -lnt 2> /dev/null | awk -v p=":${port} " '$4 ~ p {exit 0} END {exit 1}'
        return
    fi
    if command -v lsof > /dev/null 2>&1; then
        lsof -nP -iTCP:${port} -sTCP:LISTEN > /dev/null 2>&1 && return 0
    fi
    return 1
}

gen_random_string() {
    local length="$1"
    openssl rand -base64 $((length * 2)) \
        | tr -dc 'a-zA-Z0-9' \
        | head -c "$length"
}

xui_env_file_path() {
    case "${release}" in
        ubuntu | debian | armbian)
            echo "/etc/default/x-ui"
            ;;
        arch | manjaro | parch | alpine)
            echo "/etc/conf.d/x-ui"
            ;;
        *)
            echo "/etc/sysconfig/x-ui"
            ;;
    esac
}

load_xui_env() {
    local env_file
    env_file="$(xui_env_file_path)"
    if [[ -r "$env_file" ]]; then
        set -a
        # shellcheck disable=SC1090
        source "$env_file"
        set +a
    fi
}

install_base() {
    echo -e "${green}Updating and install dependency packages...${plain}"
    case "${release}" in
        ubuntu | debian | armbian)
            apt-get update > /dev/null 2>&1 && apt-get install -y -q cron curl tar unzip tzdata socat openssl > /dev/null 2>&1
            ;;
        fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol)
            dnf -y update > /dev/null 2>&1 && dnf install -y -q cronie curl tar unzip tzdata socat openssl > /dev/null 2>&1
            ;;
        centos)
            if [[ "${VERSION_ID}" =~ ^7 ]]; then
                yum -y update > /dev/null 2>&1 && yum install -y -q cronie curl tar unzip tzdata socat openssl > /dev/null 2>&1
            else
                dnf -y update > /dev/null 2>&1 && dnf install -y -q cronie curl tar unzip tzdata socat openssl > /dev/null 2>&1
            fi
            ;;
        arch | manjaro | parch)
            pacman -Syu > /dev/null 2>&1 && pacman -Syu --noconfirm cronie curl tar unzip tzdata socat openssl > /dev/null 2>&1
            ;;
        opensuse-tumbleweed | opensuse-leap)
            zypper refresh > /dev/null 2>&1 && zypper -q install -y cron curl tar unzip timezone socat openssl > /dev/null 2>&1
            ;;
        alpine)
            apk update > /dev/null 2>&1 && apk add dcron curl tar unzip tzdata socat openssl > /dev/null 2>&1
            ;;
        *)
            apt-get update > /dev/null 2>&1 && apt install -y -q cron curl tar unzip tzdata socat openssl > /dev/null 2>&1
            ;;
    esac
}

install_acme() {
    echo -e "${green}Installing acme.sh for SSL certificate management...${plain}"
    cd ~ || return 1
    curl -s https://get.acme.sh | sh > /dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo -e "${red}Failed to install acme.sh${plain}"
        return 1
    else
        echo -e "${green}acme.sh installed successfully${plain}"
    fi
    return 0
}

setup_ssl_certificate() {
    local domain="$1"
    local server_ip="$2"
    local existing_port="$3"
    local existing_webBasePath="$4"

    echo -e "${green}Setting up SSL certificate...${plain}"

    # Check if acme.sh is installed
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        install_acme
        if [ $? -ne 0 ]; then
            echo -e "${yellow}Failed to install acme.sh, skipping SSL setup${plain}"
            return 1
        fi
    fi

    # Create certificate directory
    local certPath="/root/cert/${domain}"
    mkdir -p "$certPath"

    # Issue certificate
    echo -e "${green}Issuing SSL certificate for ${domain}...${plain}"
    echo -e "${yellow}Note: Port 80 must be open and accessible from the internet${plain}"

    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt --force > /dev/null 2>&1
    ~/.acme.sh/acme.sh --issue -d ${domain} $(acme_listen_flag) --standalone --httpport 80 --force

    if [ $? -ne 0 ]; then
        echo -e "${yellow}Failed to issue certificate for ${domain}${plain}"
        echo -e "${yellow}Please ensure port 80 is open and try again later with: x-ui${plain}"
        rm -rf ~/.acme.sh/${domain} 2> /dev/null
        rm -rf "$certPath" 2> /dev/null
        return 1
    fi

    # Install certificate
    ~/.acme.sh/acme.sh --installcert -d ${domain} \
        --key-file /root/cert/${domain}/privkey.pem \
        --fullchain-file /root/cert/${domain}/fullchain.pem \
        --reloadcmd "systemctl restart x-ui" > /dev/null 2>&1

    if [ $? -ne 0 ]; then
        echo -e "${yellow}Failed to install certificate${plain}"
        return 1
    fi

    # Enable auto-renew
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade > /dev/null 2>&1
    chmod 600 $certPath/privkey.pem 2> /dev/null
    chmod 644 $certPath/fullchain.pem 2> /dev/null

    # Set certificate for panel
    local webCertFile="/root/cert/${domain}/fullchain.pem"
    local webKeyFile="/root/cert/${domain}/privkey.pem"

    if [[ -f "$webCertFile" && -f "$webKeyFile" ]]; then
        ${xui_folder}/x-ui cert -webCert "$webCertFile" -webCertKey "$webKeyFile" > /dev/null 2>&1
        echo -e "${green}SSL certificate installed and configured successfully!${plain}"
        return 0
    else
        echo -e "${yellow}Certificate files not found${plain}"
        return 1
    fi
}

# Issue Let's Encrypt IP certificate with shortlived profile (~6 days validity)
# Requires acme.sh and port 80 open for HTTP-01 challenge
setup_ip_certificate() {
    local ipv4="$1"
    local ipv6="$2" # optional

    echo -e "${green}Setting up Let's Encrypt IP certificate (shortlived profile)...${plain}"
    echo -e "${yellow}Note: IP certificates are valid for ~6 days and will auto-renew.${plain}"
    echo -e "${yellow}Default listener is port 80. If you choose another port, ensure external port 80 forwards to it.${plain}"

    # Check for acme.sh
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        install_acme
        if [ $? -ne 0 ]; then
            echo -e "${red}Failed to install acme.sh${plain}"
            return 1
        fi
    fi

    # Validate IP address
    if [[ -z "$ipv4" ]]; then
        echo -e "${red}IPv4 address is required${plain}"
        return 1
    fi

    if ! is_ipv4 "$ipv4"; then
        echo -e "${red}Invalid IPv4 address: $ipv4${plain}"
        return 1
    fi

    # Create certificate directory
    local certDir="/root/cert/ip"
    mkdir -p "$certDir"

    # Build domain arguments
    local domain_args="-d ${ipv4}"
    if [[ -n "$ipv6" ]] && is_ipv6 "$ipv6"; then
        domain_args="${domain_args} -d ${ipv6}"
        echo -e "${green}Including IPv6 address: ${ipv6}${plain}"
    fi

    # Set reload command for auto-renewal (add || true so it doesn't fail if service stopped)
    local reloadCmd="systemctl restart x-ui 2>/dev/null || rc-service x-ui restart 2>/dev/null || true"

    # Choose port for HTTP-01 listener (default 80, prompt override)
    local WebPort=""
    read -rp "Port to use for ACME HTTP-01 listener (default 80): " WebPort
    WebPort="${WebPort:-80}"
    if ! [[ "${WebPort}" =~ ^[0-9]+$ ]] || ((WebPort < 1 || WebPort > 65535)); then
        echo -e "${red}Invalid port provided. Falling back to 80.${plain}"
        WebPort=80
    fi
    echo -e "${green}Using port ${WebPort} for standalone validation.${plain}"
    if [[ "${WebPort}" -ne 80 ]]; then
        echo -e "${yellow}Reminder: Let's Encrypt still connects on port 80; forward external port 80 to ${WebPort}.${plain}"
    fi

    # Ensure chosen port is available
    while true; do
        if is_port_in_use "${WebPort}"; then
            echo -e "${yellow}Port ${WebPort} is currently in use.${plain}"

            local alt_port=""
            read -rp "Enter another port for acme.sh standalone listener (leave empty to abort): " alt_port
            alt_port="${alt_port// /}"
            if [[ -z "${alt_port}" ]]; then
                echo -e "${red}Port ${WebPort} is busy; cannot proceed.${plain}"
                return 1
            fi
            if ! [[ "${alt_port}" =~ ^[0-9]+$ ]] || ((alt_port < 1 || alt_port > 65535)); then
                echo -e "${red}Invalid port provided.${plain}"
                return 1
            fi
            WebPort="${alt_port}"
            continue
        else
            echo -e "${green}Port ${WebPort} is free and ready for standalone validation.${plain}"
            break
        fi
    done

    # Issue certificate with shortlived profile
    echo -e "${green}Issuing IP certificate for ${ipv4}...${plain}"
    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt --force > /dev/null 2>&1

    ~/.acme.sh/acme.sh --issue \
        ${domain_args} \
        --standalone \
        --server letsencrypt \
        --certificate-profile shortlived \
        --days 6 \
        --httpport ${WebPort} \
        --force

    if [ $? -ne 0 ]; then
        echo -e "${red}Failed to issue IP certificate${plain}"
        echo -e "${yellow}Please ensure port ${WebPort} is reachable (or forwarded from external port 80)${plain}"
        # Cleanup acme.sh data for both IPv4 and IPv6 if specified
        rm -rf ~/.acme.sh/${ipv4} 2> /dev/null
        [[ -n "$ipv6" ]] && rm -rf ~/.acme.sh/${ipv6} 2> /dev/null
        rm -rf ${certDir} 2> /dev/null
        return 1
    fi

    echo -e "${green}Certificate issued successfully, installing...${plain}"

    # Install certificate
    # Note: acme.sh may report "Reload error" and exit non-zero if reloadcmd fails,
    # but the cert files are still installed. We check for files instead of exit code.
    ~/.acme.sh/acme.sh --installcert -d ${ipv4} \
        --key-file "${certDir}/privkey.pem" \
        --fullchain-file "${certDir}/fullchain.pem" \
        --reloadcmd "${reloadCmd}" 2>&1 || true

    # Verify certificate files exist (don't rely on exit code - reloadcmd failure causes non-zero)
    if [[ ! -f "${certDir}/fullchain.pem" || ! -f "${certDir}/privkey.pem" ]]; then
        echo -e "${red}Certificate files not found after installation${plain}"
        # Cleanup acme.sh data for both IPv4 and IPv6 if specified
        rm -rf ~/.acme.sh/${ipv4} 2> /dev/null
        [[ -n "$ipv6" ]] && rm -rf ~/.acme.sh/${ipv6} 2> /dev/null
        rm -rf ${certDir} 2> /dev/null
        return 1
    fi

    echo -e "${green}Certificate files installed successfully${plain}"

    # Enable auto-upgrade for acme.sh (ensures cron job runs)
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade > /dev/null 2>&1

    chmod 600 ${certDir}/privkey.pem 2> /dev/null
    chmod 644 ${certDir}/fullchain.pem 2> /dev/null

    # Configure panel to use the certificate
    echo -e "${green}Setting certificate paths for the panel...${plain}"
    ${xui_folder}/x-ui cert -webCert "${certDir}/fullchain.pem" -webCertKey "${certDir}/privkey.pem"
    if [ $? -ne 0 ]; then
        echo -e "${yellow}Warning: Could not set certificate paths automatically.${plain}"
        echo -e "${yellow}You may need to set them manually in the panel settings.${plain}"
        echo -e "${yellow}Cert path: ${certDir}/fullchain.pem${plain}"
        echo -e "${yellow}Key path: ${certDir}/privkey.pem${plain}"
    else
        echo -e "${green}Certificate paths set successfully!${plain}"
    fi

    echo -e "${green}IP certificate installed and configured successfully!${plain}"
    echo -e "${green}Certificate valid for ~6 days, auto-renews via acme.sh cron job.${plain}"
    echo -e "${yellow}Panel will automatically restart after each renewal.${plain}"
    return 0
}

# Comprehensive manual SSL certificate issuance via acme.sh
ssl_cert_issue() {
    local existing_webBasePath=$(${xui_folder}/x-ui setting -show true | grep 'webBasePath:' | awk -F': ' '{print $2}' | tr -d '[:space:]' | sed 's#^/##')
    local existing_port=$(${xui_folder}/x-ui setting -show true | grep 'port:' | awk -F': ' '{print $2}' | tr -d '[:space:]')

    # check for acme.sh first
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        echo "acme.sh could not be found. Installing now..."
        cd ~ || return 1
        curl -s https://get.acme.sh | sh
        if [ $? -ne 0 ]; then
            echo -e "${red}Failed to install acme.sh${plain}"
            return 1
        else
            echo -e "${green}acme.sh installed successfully${plain}"
        fi
    fi

    # get the domain here, and we need to verify it
    local domain=""
    while true; do
        read -rp "Please enter your domain name: " domain
        domain="${domain// /}" # Trim whitespace

        if [[ -z "$domain" ]]; then
            echo -e "${red}Domain name cannot be empty. Please try again.${plain}"
            continue
        fi

        if ! is_domain "$domain"; then
            echo -e "${red}Invalid domain format: ${domain}. Please enter a valid domain name.${plain}"
            continue
        fi

        break
    done
    echo -e "${green}Your domain is: ${domain}, checking it...${plain}"
    SSL_ISSUED_DOMAIN="${domain}"

    # detect existing certificate and reuse it if present
    local cert_exists=0
    if ~/.acme.sh/acme.sh --list 2> /dev/null | awk '{print $1}' | grep -Fxq "${domain}"; then
        cert_exists=1
        local certInfo=$(~/.acme.sh/acme.sh --list 2> /dev/null | grep -F "${domain}")
        echo -e "${yellow}Existing certificate found for ${domain}, will reuse it.${plain}"
        [[ -n "${certInfo}" ]] && echo "$certInfo"
    else
        echo -e "${green}Your domain is ready for issuing certificates now...${plain}"
    fi

    # create a directory for the certificate
    certPath="/root/cert/${domain}"
    if [ ! -d "$certPath" ]; then
        mkdir -p "$certPath"
    else
        rm -rf "$certPath"
        mkdir -p "$certPath"
    fi

    # get the port number for the standalone server
    local WebPort=80
    read -rp "Please choose which port to use (default is 80): " WebPort
    if [[ ${WebPort} -gt 65535 || ${WebPort} -lt 1 ]]; then
        echo -e "${yellow}Your input ${WebPort} is invalid, will use default port 80.${plain}"
        WebPort=80
    fi
    echo -e "${green}Will use port: ${WebPort} to issue certificates. Please make sure this port is open.${plain}"

    # Stop panel temporarily
    echo -e "${yellow}Stopping panel temporarily...${plain}"
    systemctl stop x-ui 2> /dev/null || rc-service x-ui stop 2> /dev/null

    if [[ ${cert_exists} -eq 0 ]]; then
        # issue the certificate
        ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt --force
        ~/.acme.sh/acme.sh --issue -d ${domain} $(acme_listen_flag) --standalone --httpport ${WebPort} --force
        if [ $? -ne 0 ]; then
            echo -e "${red}Issuing certificate failed, please check logs.${plain}"
            rm -rf ~/.acme.sh/${domain}
            service_start_for_config || true
            return 1
        else
            echo -e "${green}Issuing certificate succeeded, installing certificates...${plain}"
        fi
    else
        echo -e "${green}Using existing certificate, installing certificates...${plain}"
    fi

    # Setup reload command
    reloadCmd="systemctl restart x-ui || rc-service x-ui restart"
    echo -e "${green}Default --reloadcmd for ACME is: ${yellow}systemctl restart x-ui || rc-service x-ui restart${plain}"
    echo -e "${green}This command will run on every certificate issue and renew.${plain}"
    read -rp "Would you like to modify --reloadcmd for ACME? (y/n): " setReloadcmd
    if [[ "$setReloadcmd" == "y" || "$setReloadcmd" == "Y" ]]; then
        echo -e "\n${green}\t1.${plain} Preset: systemctl reload nginx ; systemctl restart x-ui"
        echo -e "${green}\t2.${plain} Input your own command"
        echo -e "${green}\t0.${plain} Keep default reloadcmd"
        read -rp "Choose an option: " choice
        case "$choice" in
            1)
                echo -e "${green}Reloadcmd is: systemctl reload nginx ; systemctl restart x-ui${plain}"
                reloadCmd="systemctl reload nginx ; systemctl restart x-ui"
                ;;
            2)
                echo -e "${yellow}It's recommended to put x-ui restart at the end${plain}"
                read -rp "Please enter your custom reloadcmd: " reloadCmd
                echo -e "${green}Reloadcmd is: ${reloadCmd}${plain}"
                ;;
            *)
                echo -e "${green}Keeping default reloadcmd${plain}"
                ;;
        esac
    fi

    # install the certificate
    local installOutput=""
    installOutput=$(~/.acme.sh/acme.sh --installcert -d ${domain} \
        --key-file /root/cert/${domain}/privkey.pem \
        --fullchain-file /root/cert/${domain}/fullchain.pem --reloadcmd "${reloadCmd}" 2>&1)
    local installRc=$?
    echo "${installOutput}"

    local installWroteFiles=0
    if echo "${installOutput}" | grep -q "Installing key to:" && echo "${installOutput}" | grep -q "Installing full chain to:"; then
        installWroteFiles=1
    fi

    if [[ -f "/root/cert/${domain}/privkey.pem" && -f "/root/cert/${domain}/fullchain.pem" && (${installRc} -eq 0 || ${installWroteFiles} -eq 1) ]]; then
        echo -e "${green}Installing certificate succeeded, enabling auto renew...${plain}"
    else
        echo -e "${red}Installing certificate failed, exiting.${plain}"
        if [[ ${cert_exists} -eq 0 ]]; then
            rm -rf ~/.acme.sh/${domain}
        fi
        service_start_for_config || true
        return 1
    fi

    # enable auto-renew
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade
    if [ $? -ne 0 ]; then
        echo -e "${yellow}Auto renew setup had issues, certificate details:${plain}"
        ls -lah /root/cert/${domain}/
        chmod 600 $certPath/privkey.pem
        chmod 644 $certPath/fullchain.pem
    else
        echo -e "${green}Auto renew succeeded, certificate details:${plain}"
        ls -lah /root/cert/${domain}/
        chmod 600 $certPath/privkey.pem
        chmod 644 $certPath/fullchain.pem
    fi

    # Restart panel
    service_start_for_config || true

    # Prompt user to set panel paths after successful certificate installation
    read -rp "Would you like to set this certificate for the panel? (y/n): " setPanel
    if [[ "$setPanel" == "y" || "$setPanel" == "Y" ]]; then
        local webCertFile="/root/cert/${domain}/fullchain.pem"
        local webKeyFile="/root/cert/${domain}/privkey.pem"

        if [[ -f "$webCertFile" && -f "$webKeyFile" ]]; then
            ${xui_folder}/x-ui cert -webCert "$webCertFile" -webCertKey "$webKeyFile"
            echo -e "${green}Certificate paths set for the panel${plain}"
            echo -e "${green}Certificate File: $webCertFile${plain}"
            echo -e "${green}Private Key File: $webKeyFile${plain}"
            echo ""
            echo -e "${green}Access URL: https://${domain}:${existing_port}/${existing_webBasePath}${plain}"
            echo -e "${yellow}Panel will restart to apply SSL certificate...${plain}"
            service_restart_for_config || return 1
        else
            echo -e "${red}Error: Certificate or private key file not found for domain: $domain.${plain}"
        fi
    else
        echo -e "${yellow}Skipping panel path setting.${plain}"
    fi

    return 0
}
# Unified interactive SSL setup (domain or IP)
# Sets global `SSL_HOST` to the chosen domain/IP
prompt_and_setup_ssl() {
    local panel_port="$1"
    local web_base_path="$2" # expected without leading slash
    local server_ip="$3"

    local ssl_choice=""

    echo -e "${yellow}Choose SSL certificate setup method:${plain}"
    echo -e "${green}1.${plain} Let's Encrypt for Domain (90-day validity, auto-renews)"
    echo -e "${green}2.${plain} Let's Encrypt for IP Address (6-day validity, auto-renews)"
    echo -e "${green}3.${plain} Custom SSL Certificate (Path to existing files)"
    echo -e "${green}4.${plain} Skip SSL (advanced — behind reverse proxy / SSH tunnel only)"
    echo -e "${blue}Note:${plain} Options 1 & 2 require port 80 open. Option 3 requires manual paths."
    echo -e "${blue}Note:${plain} Option 4 serves the panel over plain HTTP — only safe behind nginx/Caddy or an SSH tunnel."
    read -rp "Choose an option (default 2 for IP): " ssl_choice
    ssl_choice="${ssl_choice// /}" # Trim whitespace

    # Default to 2 (IP cert) if input is empty or invalid (not 1, 3 or 4)
    if [[ "$ssl_choice" != "1" && "$ssl_choice" != "3" && "$ssl_choice" != "4" ]]; then
        ssl_choice="2"
    fi

    case "$ssl_choice" in
        1)
            # User chose Let's Encrypt domain option
            echo -e "${green}Using Let's Encrypt for domain certificate...${plain}"
            if ssl_cert_issue; then
                local cert_domain="${SSL_ISSUED_DOMAIN}"
                if [[ -z "${cert_domain}" ]]; then
                    cert_domain=$(~/.acme.sh/acme.sh --list 2> /dev/null | tail -1 | awk '{print $1}')
                fi

                if [[ -n "${cert_domain}" ]]; then
                    SSL_HOST="${cert_domain}"
                    echo -e "${green}✓ SSL certificate configured successfully with domain: ${cert_domain}${plain}"
                else
                    echo -e "${yellow}SSL setup may have completed, but domain extraction failed${plain}"
                    SSL_HOST="${server_ip}"
                fi
            else
                echo -e "${red}SSL certificate setup failed for domain mode.${plain}"
                SSL_HOST="${server_ip}"
            fi
            ;;
        2)
            # User chose Let's Encrypt IP certificate option
            echo -e "${green}Using Let's Encrypt for IP certificate (shortlived profile)...${plain}"

            # Ask for optional IPv6
            local ipv6_addr=""
            read -rp "Do you have an IPv6 address to include? (leave empty to skip): " ipv6_addr
            ipv6_addr="${ipv6_addr// /}" # Trim whitespace

            # Stop panel if running (port 80 needed)
            if [[ $release == "alpine" ]]; then
                rc-service x-ui stop > /dev/null 2>&1
            else
                systemctl stop x-ui > /dev/null 2>&1
            fi

            setup_ip_certificate "${server_ip}" "${ipv6_addr}"
            if [ $? -eq 0 ]; then
                SSL_HOST="${server_ip}"
                echo -e "${green}✓ Let's Encrypt IP certificate configured successfully${plain}"
            else
                echo -e "${red}✗ IP certificate setup failed. Please check port 80 is open.${plain}"
                SSL_HOST="${server_ip}"
            fi

            # Restart panel after SSL is configured (restart applies new cert settings)
            service_restart_for_config || return 1

            ;;
        3)
            # User chose Custom Paths (User Provided) option
            echo -e "${green}Using custom existing certificate...${plain}"
            local custom_cert=""
            local custom_key=""
            local custom_domain=""

            # 3.1 Request Domain to compose Panel URL later
            read -rp "Please enter domain name certificate issued for: " custom_domain
            custom_domain="${custom_domain// /}" # Remove spaces

            # 3.2 Loop for Certificate Path
            while true; do
                read -rp "Input certificate path (keywords: .crt / fullchain): " custom_cert
                # Strip quotes if present
                custom_cert=$(echo "$custom_cert" | tr -d '"' | tr -d "'")

                if [[ -f "$custom_cert" && -r "$custom_cert" && -s "$custom_cert" ]]; then
                    break
                elif [[ ! -f "$custom_cert" ]]; then
                    echo -e "${red}Error: File does not exist! Try again.${plain}"
                elif [[ ! -r "$custom_cert" ]]; then
                    echo -e "${red}Error: File exists but is not readable (check permissions)!${plain}"
                else
                    echo -e "${red}Error: File is empty!${plain}"
                fi
            done

            # 3.3 Loop for Private Key Path
            while true; do
                read -rp "Input private key path (keywords: .key / privatekey): " custom_key
                # Strip quotes if present
                custom_key=$(echo "$custom_key" | tr -d '"' | tr -d "'")

                if [[ -f "$custom_key" && -r "$custom_key" && -s "$custom_key" ]]; then
                    break
                elif [[ ! -f "$custom_key" ]]; then
                    echo -e "${red}Error: File does not exist! Try again.${plain}"
                elif [[ ! -r "$custom_key" ]]; then
                    echo -e "${red}Error: File exists but is not readable (check permissions)!${plain}"
                else
                    echo -e "${red}Error: File is empty!${plain}"
                fi
            done

            # 3.4 Apply Settings via x-ui binary
            ${xui_folder}/x-ui cert -webCert "$custom_cert" -webCertKey "$custom_key" > /dev/null 2>&1

            # Set SSL_HOST for composing Panel URL
            if [[ -n "$custom_domain" ]]; then
                SSL_HOST="$custom_domain"
            else
                SSL_HOST="${server_ip}"
            fi

            echo -e "${green}✓ Custom certificate paths applied.${plain}"
            echo -e "${yellow}Note: You are responsible for renewing these files externally.${plain}"

            service_restart_for_config || return 1
            ;;
        4)
            echo ""
            echo -e "${red}⚠ Panel will be installed WITHOUT SSL/TLS.${plain}"
            echo -e "${yellow}Login credentials and cookies will travel as plain HTTP.${plain}"
            echo -e "${yellow}Only safe when:${plain}"
            echo -e "${yellow}  • A reverse proxy (nginx, Caddy, Traefik) terminates TLS for you, or${plain}"
            echo -e "${yellow}  • You access the panel exclusively via SSH tunnel${plain}"
            echo ""

            SSL_SCHEME="http"
            SSL_HOST="${server_ip}"

            local bind_local=""
            read -rp "Bind the panel to 127.0.0.1 only? (recommended — forces SSH tunnel / reverse-proxy access) [y/N]: " bind_local
            if [[ "$bind_local" == "y" || "$bind_local" == "Y" ]]; then
                ${xui_folder}/x-ui setting -listenIP "127.0.0.1" > /dev/null 2>&1
                SSL_HOST="127.0.0.1"
                echo -e "${green}✓ Panel bound to 127.0.0.1 only. It is now unreachable from the public internet.${plain}"
                echo ""
                echo -e "${green}SSH Port Forwarding — open the panel from your local machine via:${plain}"
                echo -e "  Standard SSH command:"
                echo -e "  ${yellow}ssh -L 2222:127.0.0.1:${panel_port} root@${server_ip}${plain}"
                echo -e "  If using an SSH key:"
                echo -e "  ${yellow}ssh -i <sshkeypath> -L 2222:127.0.0.1:${panel_port} root@${server_ip}${plain}"
                echo -e "  Then open in your browser:"
                echo -e "  ${yellow}http://localhost:2222/${web_base_path}${plain}"
                echo ""
                echo -e "${yellow}Alternative: point a reverse proxy (nginx/Caddy) at 127.0.0.1:${panel_port} and let it terminate TLS.${plain}"
            else
                echo -e "${yellow}Panel will listen on all interfaces over plain HTTP. Make sure something else is terminating TLS in front of it.${plain}"
            fi

            service_restart_for_config || return 1
            echo -e "${green}✓ SSL setup skipped.${plain}"
            ;;
        *)
            echo -e "${red}Invalid option. Skipping SSL setup.${plain}"
            SSL_HOST="${server_ip}"
            ;;
    esac
}

config_after_update() {
    local panel_needs_restart=0

    echo -e "${yellow}x-ui settings:${plain}"
    if ! ${xui_folder}/x-ui setting -show true; then
        echo -e "${red}Failed to read x-ui settings after replacing the panel.${plain}"
        return 1
    fi
    if ! ${xui_folder}/x-ui migrate; then
        echo -e "${red}Database migration failed; the transaction will be rolled back.${plain}"
        return 1
    fi

    # Properly detect empty cert by checking if cert: line exists and has content after it
    local existing_cert=$(${xui_folder}/x-ui setting -getCert true 2> /dev/null | grep 'cert:' | awk -F': ' '{print $2}' | tr -d '[:space:]')
    local existing_port=$(${xui_folder}/x-ui setting -show true | grep -Eo 'port: .+' | awk '{print $2}')
    local existing_webBasePath=$(${xui_folder}/x-ui setting -show true | grep -Eo 'webBasePath: .+' | awk '{print $2}' | sed 's#^/##')

    # Get server IP
    local URL_lists=(
        "https://api4.ipify.org"
        "https://ipv4.icanhazip.com"
        "https://v4.api.ipinfo.io/ip"
        "https://ipv4.myexternalip.com/raw"
        "https://4.ident.me"
        "https://check-host.net/ip"
    )
    local server_ip=""
    for ip_address in "${URL_lists[@]}"; do
        local response=$(curl -s -w "\n%{http_code}" --max-time 3 "${ip_address}" 2> /dev/null)
        local http_code=$(echo "$response" | tail -n1)
        local ip_result=$(echo "$response" | head -n-1 | tr -d '[:space:]"')
        if [[ "${http_code}" == "200" && "${ip_result}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            server_ip="${ip_result}"
            break
        fi
    done

    if [[ -z "$server_ip" ]]; then
        echo -e "${yellow}Could not auto-detect server IP from any provider.${plain}"
        while [[ -z "$server_ip" ]]; do
            read -rp "Please enter your server's public IPv4 address: " server_ip
            server_ip="${server_ip// /}"
            if [[ ! "$server_ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                echo -e "${red}Invalid IPv4 address. Please try again.${plain}"
                server_ip=""
            fi
        done
    fi

    # Handle missing/short webBasePath
    if [[ ${#existing_webBasePath} -lt 4 ]]; then
        echo -e "${yellow}WebBasePath is missing or too short. Generating a new one...${plain}"
        local config_webBasePath=$(gen_random_string 18)
        ${xui_folder}/x-ui setting -webBasePath "${config_webBasePath}"
        existing_webBasePath="${config_webBasePath}"
        panel_needs_restart=1
        echo -e "${green}New WebBasePath: ${config_webBasePath}${plain}"
    fi

    # Check and prompt for SSL if missing
    if [[ -z "$existing_cert" ]]; then
        echo ""
        echo -e "${red}═══════════════════════════════════════════${plain}"
        echo -e "${red}      ⚠ NO SSL CERTIFICATE DETECTED ⚠     ${plain}"
        echo -e "${red}═══════════════════════════════════════════${plain}"
        echo -e "${yellow}For security, SSL certificate is MANDATORY for all panels.${plain}"
        echo -e "${yellow}Let's Encrypt now supports both domains and IP addresses!${plain}"
        echo ""

        # Prompt and setup SSL (domain or IP)
        if ! prompt_and_setup_ssl "${existing_port}" "${existing_webBasePath}" "${server_ip}"; then
            echo -e "${red}SSL setup failed; the transaction will be rolled back.${plain}"
            return 1
        fi

        echo ""
        echo -e "${green}═══════════════════════════════════════════${plain}"
        echo -e "${green}     Panel Access Information              ${plain}"
        echo -e "${green}═══════════════════════════════════════════${plain}"
        echo -e "${green}Access URL: https://${SSL_HOST}:${existing_port}/${existing_webBasePath}${plain}"
        echo -e "${green}═══════════════════════════════════════════${plain}"
        echo -e "${yellow}⚠ SSL Certificate: Enabled and configured${plain}"
    else
        echo -e "${green}SSL certificate is already configured${plain}"
        # Show access URL with existing certificate
        local cert_domain=$(basename "$(dirname "$existing_cert")")
        echo ""
        echo -e "${green}═══════════════════════════════════════════${plain}"
        echo -e "${green}     Panel Access Information              ${plain}"
        echo -e "${green}═══════════════════════════════════════════${plain}"
        echo -e "${green}Access URL: https://${cert_domain}:${existing_port}/${existing_webBasePath}${plain}"
        echo -e "${green}═══════════════════════════════════════════${plain}"
    fi

    if [[ "$panel_needs_restart" -eq 1 ]]; then
        echo -e "${yellow}Restarting panel to apply the new web base path...${plain}"
        service_restart_for_config || return 1
    fi
}

download_update_file() {
    local destination="$1" url="$2"
    if ${curl_bin} -fLRsS --retry 3 -o "$destination" "$url"; then
        return 0
    fi
    echo -e "${yellow}Retrying download over IPv4...${plain}"
    ${curl_bin} -4fLRsS --retry 3 -o "$destination" "$url"
}

preserve_update_data() {
    local relative
    for relative in "db" "bin/config.json"; do
        if [[ -e "${xui_folder}/${relative}" || -L "${xui_folder}/${relative}" ]]; then
            rm -rf "${update_stage_dir}/${relative}"
            mkdir -p "${update_stage_dir}/$(dirname "$relative")"
            cp -a "${xui_folder}/${relative}" "${update_stage_dir}/${relative}" || return 1
        fi
    done
}

# Resolve the database used by the currently installed binary before the live
# directory is moved. SQLite's filename is derived from the embedded panel
# name, which may be versioned in older OMEGA installations; do not assume that
# it is always x-ui.db when more than one database is present.
configure_update_database() {
    local kind folder embedded_name candidate
    local -a candidates=()

    kind="$(printf '%s' "${XUI_DB_TYPE:-sqlite}" | tr '[:upper:]' '[:lower:]')"
    case "$kind" in
        postgres|postgresql|pg)
            update_db_kind="postgres"
            update_db_dsn="${XUI_DB_DSN:-}"
            if [[ -z "$update_db_dsn" ]]; then
                echo -e "${red}PostgreSQL is configured but XUI_DB_DSN is empty; the installation was not touched.${plain}"
                return 1
            fi
            if ! command -v pg_dump >/dev/null 2>&1 || ! command -v pg_restore >/dev/null 2>&1; then
                echo -e "${red}PostgreSQL rollback tools pg_dump and pg_restore are required; the installation was not touched.${plain}"
                return 1
            fi
            ;;
        *)
            update_db_kind="sqlite"
            folder="${XUI_DB_FOLDER:-/etc/x-ui}"
            if [[ "$folder" != /* ]]; then
                folder="${xui_folder}/${folder}"
            fi

            # Prefer the exact name embedded in the old executable. This keeps
            # upgrades compatible with releases whose database name included
            # the panel version.
            if command -v strings >/dev/null 2>&1; then
                embedded_name=$(strings "${xui_folder}/x-ui" 2>/dev/null \
                    | grep -Eo 'x-ui[0-9]+\.[0-9]+\.[0-9]+-[A-Za-z0-9._-]+' \
                    | head -n1 || true)
            fi
            if [[ -n "$embedded_name" && -f "${folder}/${embedded_name}.db" ]]; then
                update_db_path="${folder}/${embedded_name}.db"
                return 0
            fi
            if [[ -f "${folder}/x-ui.db" ]]; then
                update_db_path="${folder}/x-ui.db"
                return 0
            fi

            if [[ -d "$folder" ]]; then
                while IFS= read -r -d '' candidate; do
                    candidates+=("$candidate")
                done < <(find "$folder" -maxdepth 1 -type f -name '*.db' -print0 2>/dev/null)
            fi
            if [[ "${#candidates[@]}" -eq 1 ]]; then
                update_db_path="${candidates[0]}"
            elif [[ "${#candidates[@]}" -gt 1 ]]; then
                echo -e "${red}Multiple SQLite databases were found in ${folder}; refusing an ambiguous rollback.${plain}"
                return 1
            elif [[ -n "$embedded_name" ]]; then
                update_db_path="${folder}/${embedded_name}.db"
            else
                update_db_path="${folder}/x-ui.db"
            fi
            ;;
    esac
    return 0
}

# Take the database snapshot only after the old service has stopped. That makes
# SQLite's main/WAL/SHM set coherent and prevents a live writer from racing the
# snapshot. PostgreSQL is dumped in its native custom format for an atomic
# pg_restore during rollback.
snapshot_update_database() {
    local suffix source destination
    [[ "$update_db_snapshot_ready" -eq 0 ]] || return 0

    if [[ "$update_db_kind" == "postgres" ]]; then
        update_db_snapshot="${update_stage_root}/database.dump"
        if ! pg_dump --format=custom --no-owner --no-privileges \
            --dbname="$update_db_dsn" --file="$update_db_snapshot"; then
            rm -f "$update_db_snapshot"
            update_db_snapshot=""
            echo -e "${red}Could not snapshot PostgreSQL before migration; the installation was not touched.${plain}"
            return 1
        fi
        chmod 0600 "$update_db_snapshot" || return 1
    else
        if [[ ! -f "$update_db_path" && ! -L "$update_db_path" ]]; then
            echo -e "${red}SQLite database ${update_db_path} is missing; the installation was not touched.${plain}"
            return 1
        fi
        update_db_snapshot_dir="${update_stage_root}/database"
        mkdir -p "$update_db_snapshot_dir" || return 1
        for suffix in "" "-wal" "-shm" "-journal"; do
            source="${update_db_path}${suffix}"
            destination="${update_db_snapshot_dir}/db${suffix}"
            rm -rf "$destination"
            if [[ -e "$source" || -L "$source" ]]; then
                cp -a "$source" "$destination" || return 1
            fi
        done
    fi
    update_db_snapshot_ready=1
    return 0
}

# Restore every database artifact that existed at the transaction boundary.
# This is deliberately separate from filesystem rollback because SQLite is
# normally stored outside the panel directory and PostgreSQL is never part of
# the directory swap.
restore_update_database() {
    local suffix source snapshot
    [[ "$update_db_snapshot_ready" -eq 1 ]] || return 0

    if [[ "$update_db_kind" == "postgres" ]]; then
        [[ -f "$update_db_snapshot" ]] || return 1
        pg_restore --clean --if-exists --no-owner --no-privileges \
            --single-transaction --exit-on-error \
            --dbname="$update_db_dsn" "$update_db_snapshot"
        return $?
    fi

    [[ -n "$update_db_snapshot_dir" && -d "$update_db_snapshot_dir" ]] || return 1
    for suffix in "" "-wal" "-shm" "-journal"; do
        source="${update_db_path}${suffix}"
        snapshot="${update_db_snapshot_dir}/db${suffix}"
        rm -rf "$source" || return 1
        if [[ -e "$snapshot" || -L "$snapshot" ]]; then
            mkdir -p "$(dirname "$source")" || return 1
            cp -a "$snapshot" "$source" || return 1
        fi
    done
    return 0
}

stage_update_assets() {
    local parent archive_name service_url
    parent="$(dirname "$xui_folder")"
    update_stage_root=$(mktemp -d "${parent}/.x-ui-update.XXXXXX") || return 1
    update_stage_dir="${update_stage_root}/x-ui"
    archive_name="x-ui-linux-$(arch).tar.gz"
    panel_update_archive="${update_stage_root}/${archive_name}"

    if ! download_update_file "$panel_update_archive" "https://github.com/Dark-Sky07/OMEGA/releases/download/${tag_version}/${archive_name}"; then
        return 1
    fi
    if ! tar tzf "$panel_update_archive" >/dev/null 2>&1; then
        echo -e "${red}Downloaded x-ui archive is invalid; the existing installation was not touched.${plain}"
        return 1
    fi
    if ! tar xzf "$panel_update_archive" -C "$update_stage_root"; then
        return 1
    fi
    [[ -d "$update_stage_dir" && -f "$update_stage_dir/x-ui" ]] || return 1

    if ! preserve_update_data; then
        return 1
    fi
    install_staged_xray "$update_stage_dir"
    chmod +x "$update_stage_dir/x-ui" || return 1
    [[ -f "$update_stage_dir/bin/xray-linux-$(arch)" || -f "$update_stage_dir/bin/xray-linux-arm32" ]] || return 1
    if [[ "$(arch)" == "armv5" || "$(arch)" == "armv6" || "$(arch)" == "armv7" ]]; then
        if [[ -f "$update_stage_dir/bin/xray-linux-$(arch)" ]]; then
            mv "$update_stage_dir/bin/xray-linux-$(arch)" "$update_stage_dir/bin/xray-linux-arm32" || return 1
        fi
    fi

    update_wrapper_stage="${update_stage_root}/x-ui-wrapper"
    if ! download_update_file "$update_wrapper_stage" "https://raw.githubusercontent.com/Dark-Sky07/OMEGA/main/x-ui.sh"; then
        return 1
    fi
    chmod 0755 "$update_wrapper_stage" || return 1

    if [[ "$release" == "alpine" ]]; then
        update_service_stage="${update_stage_root}/x-ui.rc"
        if [[ -f "$update_stage_dir/x-ui.rc" ]]; then
            cp -f "$update_stage_dir/x-ui.rc" "$update_service_stage" || return 1
        else
            if ! download_update_file "$update_service_stage" "https://raw.githubusercontent.com/Dark-Sky07/OMEGA/main/x-ui.rc"; then
                return 1
            fi
        fi
        chmod 0755 "$update_service_stage" || return 1
    else
        update_service_stage="${update_stage_root}/x-ui.service"
        if [[ -f "$update_stage_dir/x-ui.service" ]]; then
            cp -f "$update_stage_dir/x-ui.service" "$update_service_stage" || return 1
        else
            case "$release" in
                ubuntu|debian|armbian) service_url="https://raw.githubusercontent.com/Dark-Sky07/OMEGA/main/x-ui.service.debian" ;;
                arch|manjaro|parch) service_url="https://raw.githubusercontent.com/Dark-Sky07/OMEGA/main/x-ui.service.arch" ;;
                *) service_url="https://raw.githubusercontent.com/Dark-Sky07/OMEGA/main/x-ui.service.rhel" ;;
            esac
            if ! download_update_file "$update_service_stage" "$service_url"; then
                return 1
            fi
        fi
        chmod 0644 "$update_service_stage" || return 1
    fi
    rm -f "$panel_update_archive"
    panel_update_archive=""
    return 0
}

cleanup_update_transaction() {
    [[ -n "$xray_update_archive" ]] && rm -f "$xray_update_archive"
    [[ -n "$panel_update_archive" ]] && rm -f "$panel_update_archive"
    [[ -n "$update_stage_root" && -d "$update_stage_root" ]] && rm -rf "$update_stage_root"
    update_stage_root=""
    update_stage_dir=""
    update_db_snapshot=""
    update_db_snapshot_dir=""
    update_db_snapshot_ready=0
    update_defer_service_start=0
}

service_is_active() {
    if [[ "$release" == "alpine" ]]; then
        rc-service x-ui status >/dev/null 2>&1
    else
        systemctl is-active --quiet x-ui
    fi
}

service_is_enabled() {
    if [[ "$release" == "alpine" ]]; then
        rc-update show default 2>/dev/null | grep -Eq '(^|[[:space:]])x-ui([[:space:]]|$)'
    else
        systemctl is-enabled --quiet x-ui
    fi
}

service_stop_for_update() {
    if [[ "$release" == "alpine" ]]; then
        rc-service x-ui stop >/dev/null 2>&1
    else
        systemctl stop x-ui >/dev/null 2>&1
    fi
}

service_start_after_update() {
    if [[ "$release" == "alpine" ]]; then
        rc-service x-ui start >/dev/null 2>&1
    else
        systemctl daemon-reload >/dev/null 2>&1 || return 1
        systemctl start x-ui >/dev/null 2>&1
    fi
}

# Configuration is applied while the candidate is stopped. SSL helpers were
# originally written for the installer and may otherwise start/restart x-ui in
# the middle of the transaction, before migration and settings have succeeded.
service_start_for_config() {
    [[ "$update_defer_service_start" -eq 1 ]] && return 0
    service_start_after_update
}

service_restart_for_config() {
    [[ "$update_defer_service_start" -eq 1 ]] && return 0
    if [[ "$release" == "alpine" ]]; then
        rc-service x-ui restart >/dev/null 2>&1
    else
        systemctl restart x-ui >/dev/null 2>&1
    fi
}

service_reload_after_update() {
    if [[ "$release" == "alpine" ]]; then
        return 0
    fi
    systemctl daemon-reload >/dev/null 2>&1
}

restore_update_service() {
    local service_path
    if [[ "$release" == "alpine" ]]; then
        service_path="/etc/init.d/x-ui"
    else
        service_path="${xui_service}/x-ui.service"
    fi
    if [[ "$update_old_service_present" -eq 1 ]]; then
        rm -f "$service_path"
        cp -a "$update_service_backup" "$service_path" || return 1
    else
        rm -f "$service_path"
    fi
    service_reload_after_update
}

rollback_update() {
    [[ "$update_transaction_active" -eq 1 ]] || return 0
    echo -e "${red}Update failed; restoring the previous x-ui installation and service state.${plain}"

    if [[ "$update_commit_started" -eq 1 ]]; then
        # Stop the candidate before moving it out of the live path. Never kill
        # unrelated daemon processes: active VPN users must not be disconnected
        # by rollback cleanup.
        service_stop_for_update >/dev/null 2>&1 || true
        if [[ -n "$update_backup_dir" && -d "$update_backup_dir" ]]; then
            if [[ -e "$xui_folder" || -L "$xui_folder" ]]; then
                rm -rf "$xui_folder" || true
            fi
            mv "$update_backup_dir" "$xui_folder" || true
        fi
        restore_update_service >/dev/null 2>&1 || true
        if [[ -n "$update_wrapper_backup" && -e "$update_wrapper_backup" ]]; then
            rm -f /usr/bin/x-ui
            cp -a "$update_wrapper_backup" /usr/bin/x-ui || true
        elif [[ -n "$update_wrapper_backup" ]]; then
            rm -f /usr/bin/x-ui
        fi
        if [[ "$update_db_snapshot_ready" -eq 1 ]]; then
            echo -e "${yellow}Restoring the pre-update database/migration state.${plain}"
            if ! restore_update_database; then
                echo -e "${red}CRITICAL: Database rollback failed; the previous service will not be started automatically.${plain}"
                update_old_service_active=0
            fi
        fi
        if [[ "$update_old_service_active" -eq 1 ]]; then
            service_start_after_update >/dev/null 2>&1 || true
        fi
    fi
    update_transaction_active=0
    update_commit_started=0
    cleanup_update_transaction
}

commit_update_transaction() {
    local parent service_path
    parent="$(dirname "$xui_folder")"
    update_backup_dir="${parent}/.x-ui-rollback.$$"
    [[ ! -e "$update_backup_dir" ]] || return 1

    if [[ "$release" == "alpine" ]]; then
        service_path="/etc/init.d/x-ui"
    else
        service_path="${xui_service}/x-ui.service"
    fi
    if [[ -e "$service_path" || -L "$service_path" ]]; then
        update_old_service_present=1
        update_service_backup="${update_stage_root}/old-service"
        cp -a "$service_path" "$update_service_backup" || return 1
    else
        echo -e "${red}Existing x-ui service unit is missing; the installation was not touched.${plain}"
        return 1
    fi

    update_wrapper_backup="${update_stage_root}/old-wrapper"
    if [[ -e /usr/bin/x-ui || -L /usr/bin/x-ui ]]; then
        cp -a /usr/bin/x-ui "$update_wrapper_backup" || return 1
    fi

    if service_is_active; then update_old_service_active=1; fi
    if service_is_enabled; then update_old_service_enabled=1; fi
    update_commit_started=1
    if ! service_stop_for_update; then return 1; fi
    if service_is_active; then
        echo -e "${red}Could not stop x-ui cleanly; the installation was not replaced.${plain}"
        return 1
    fi

    if ! snapshot_update_database; then
        return 1
    fi

    if ! mv "$xui_folder" "$update_backup_dir"; then return 1; fi
    if ! mv "$update_stage_dir" "$xui_folder"; then return 1; fi

    if ! install -m 0755 "$update_wrapper_stage" /usr/bin/x-ui; then return 1; fi
    rm -f "$service_path"
    if ! cp -a "$update_service_stage" "$service_path"; then return 1; fi
    if [[ "$release" == "alpine" ]]; then
        chown root:root "$service_path" 2>/dev/null || true
        rc-update del x-ui >/dev/null 2>&1 || true
        [[ "$update_old_service_enabled" -eq 1 ]] && rc-update add x-ui >/dev/null 2>&1 || true
    else
        chown root:root "$service_path" 2>/dev/null || true
        chmod 0644 "$service_path" || return 1
        service_reload_after_update || return 1
        if [[ "$update_old_service_enabled" -eq 1 ]]; then
            systemctl enable x-ui >/dev/null 2>&1 || return 1
        else
            systemctl disable x-ui >/dev/null 2>&1 || return 1
        fi
    fi

    # Do not start the candidate here. Migration and post-update settings are
    # applied while it is stopped; update_x-ui starts it only after they have
    # completed successfully.
    return 0
}

finalize_update_transaction() {
    rm -rf "$update_backup_dir"
    update_backup_dir=""
    update_transaction_active=0
    update_commit_started=0
    cleanup_update_transaction
    trap - EXIT
}

update_x-ui() {
    cd "${xui_folder%/x-ui}/" || _fail "ERROR: x-ui parent directory is unavailable."

    load_xui_env
    if [[ ! -f "${xui_folder}/x-ui" || ! -d "${xui_folder}" ]]; then
        _fail "ERROR: Current x-ui installation is missing."
    fi
    if ! configure_update_database; then
        _fail "ERROR: Could not establish a safe database rollback boundary; the existing installation was not touched."
    fi
    case "$xui_folder" in
        ""|/) _fail "ERROR: Refusing to update an unsafe installation path." ;;
    esac

    current_xui_version=$(${xui_folder}/x-ui -v 2>/dev/null || true)
    echo -e "${green}Current x-ui version: ${current_xui_version:-unknown}${plain}"
    echo -e "${green}Preparing a transactional x-ui update; the live installation will remain untouched until validation completes.${plain}"

    tag_version=$(${curl_bin} -Ls "https://api.github.com/repos/Dark-Sky07/OMEGA/releases?per_page=10" 2>/dev/null | grep -m1 '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    if [[ -z "$tag_version" ]]; then
        echo -e "${yellow}Trying to fetch version with IPv4...${plain}"
        tag_version=$(${curl_bin} -4 -Ls "https://api.github.com/repos/Dark-Sky07/OMEGA/releases?per_page=10" 2>/dev/null | grep -m1 '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    fi
    [[ -n "$tag_version" ]] || _fail "ERROR: Failed to fetch x-ui version; the existing installation was not touched."
    echo -e "Got x-ui latest version: ${tag_version}; staging the installation..."

    # From this point every failure cleans only temporary files, or restores the
    # old directory/service/wrapper after the single atomic commit point.
    update_transaction_active=1
    trap 'rollback_update' EXIT

    stage_latest_xray
    if ! stage_update_assets; then
        _fail "ERROR: Failed to stage x-ui ${tag_version}; the existing installation was not touched."
    fi
    echo -e "${green}Validated panel archive and latest Xray-core ${xray_update_version}; committing the staged installation...${plain}"

    if ! commit_update_transaction; then
        _fail "ERROR: Failed to activate x-ui ${tag_version}; rollback completed."
    fi
    mkdir -p /var/log/x-ui || _fail "ERROR: Failed to prepare x-ui log directory; rollback completed."
    chown -R root:root "$xui_folder" >/dev/null 2>&1 || _fail "ERROR: Failed to set x-ui ownership; rollback completed."
    if [[ -f "${xui_folder}/bin/config.json" ]]; then
        chmod 640 "${xui_folder}/bin/config.json" || _fail "ERROR: Failed to protect x-ui config; rollback completed."
    fi

    # Apply migration and settings while the candidate is stopped. This avoids
    # exposing a partially migrated database through the new process. All DB
    # writes remain covered by the snapshot taken at the commit boundary.
    update_defer_service_start=1
    if ! config_after_update; then
        _fail "ERROR: Post-update migration/configuration failed; rollback completed."
    fi
    update_defer_service_start=0

    if ! service_start_after_update || ! service_is_active; then
        _fail "ERROR: x-ui failed its post-configuration health check; rollback completed."
    fi
    finalize_update_transaction

    echo -e "${green}x-ui ${tag_version} and Xray-core ${xray_update_version} update finished; it is running now.${plain}"
    echo -e "${green}The prior installation was retained until the new service passed its health check, then removed.${plain}"
}

echo -e "${green}Running...${plain}"
install_base
update_x-ui $1
