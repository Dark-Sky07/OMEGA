#!/bin/sh

# Start fail2ban with the 3x-ipl jail
if [ "$XUI_ENABLE_FAIL2BAN" = "true" ]; then
    LOG_FOLDER="${XUI_LOG_FOLDER:-/var/log/x-ui}"
    mkdir -p "$LOG_FOLDER"
    touch "$LOG_FOLDER/3xipl.log" "$LOG_FOLDER/3xipl-banned.log"

    mkdir -p /etc/fail2ban/jail.d /etc/fail2ban/filter.d /etc/fail2ban/action.d

    cat > /etc/fail2ban/jail.d/3x-ipl.conf << EOF
[3x-ipl]
enabled=true
backend=auto
filter=3x-ipl
action=3x-ipl
logpath=$LOG_FOLDER/3xipl.log
maxretry=1
findtime=32
bantime=30m
EOF

    cat > /etc/fail2ban/filter.d/3x-ipl.conf << 'EOF'
[Definition]
datepattern = ^%%Y/%%m/%%d %%H:%%M:%%S
failregex   = \[LIMIT_IP\]\s*Email\s*=\s*<F-USER>.+</F-USER>\s*\|\|\s*Disconnecting OLD IP\s*=\s*<ADDR>\s*\|\|\s*Timestamp\s*=\s*\d+
ignoreregex =
EOF

    # Ports to exempt from the ban so an over-limit proxy client can never lock
    # the administrator out of SSH or the panel. The ban still covers every other
    # TCP port (including all Xray inbounds), so IP-limit keeps working for inbounds
    # added later without regenerating these files.
    SSH_PORTS=$(grep -oE '^[[:space:]]*Port[[:space:]]+[0-9]+' /etc/ssh/sshd_config 2>/dev/null | grep -oE '[0-9]+' | paste -sd, -)
    [ -z "$SSH_PORTS" ] && SSH_PORTS="22"
    PANEL_PORT=$(/app/x-ui setting -show true 2>/dev/null | grep -Eo 'port: .+' | awk '{print $2}')
    EXEMPT_PORTS="$SSH_PORTS"
    [ -n "$PANEL_PORT" ] && EXEMPT_PORTS="$EXEMPT_PORTS,$PANEL_PORT"

    cat > /etc/fail2ban/action.d/3x-ipl.conf << EOF
[INCLUDES]
before = iptables-allports.conf

[Definition]
actionstart = <iptables> -N f2b-<name>
              <iptables> -A f2b-<name> -j <returntype>
              <iptables> -I <chain> -p <protocol> -j f2b-<name>

actionstop = <iptables> -D <chain> -p <protocol> -j f2b-<name>
             <actionflush>
             <iptables> -X f2b-<name>

actioncheck = <iptables> -n -L <chain> | grep -q 'f2b-<name>[ \t]'

actionban = <iptables> -I f2b-<name> 1 -s <ip> -p <protocol> -m multiport ! --dports <exemptports> -j <blocktype>
            echo "\$(date +"%%Y/%%m/%%d %%H:%%M:%%S")   BAN   [Email] = <F-USER> [IP] = <ip> banned for <bantime> seconds." >> $LOG_FOLDER/3xipl-banned.log

actionunban = <iptables> -D f2b-<name> -s <ip> -p <protocol> -m multiport ! --dports <exemptports> -j <blocktype>
              echo "\$(date +"%%Y/%%m/%%d %%H:%%M:%%S")   UNBAN   [Email] = <F-USER> [IP] = <ip> unbanned." >> $LOG_FOLDER/3xipl-banned.log

[Init]
name = default
protocol = tcp
chain = INPUT
exemptports = $EXEMPT_PORTS
EOF

    fail2ban-client -x start
fi

# Start the separately configured, loopback-only Simurgh panel. Its relay starts
# unpaired and has no forwarding listeners until an administrator configures it.
if [ -x /opt/phoenix-simurgh/simurgh ] && command -v python3 >/dev/null 2>&1; then
    mkdir -p /etc/phoenix-simurgh /var/log/phoenix-simurgh
    chmod 700 /etc/phoenix-simurgh
    SIMURGH_CONFIG_OK=true
    if [ ! -s /etc/phoenix-simurgh/relay.toml ]; then
        if ! SIMURGH_HOME=/etc/phoenix-simurgh /opt/phoenix-simurgh/simurgh install \
            --role relay --pair-later --panel-port 18787 --engine python \
            --no-units --no-enable --no-start --force \
            >> /var/log/phoenix-simurgh/setup.log 2>&1; then
            echo "[Simurgh] Relay configuration failed; check /var/log/phoenix-simurgh/setup.log. PHOENIX will still start."
            SIMURGH_CONFIG_OK=false
        fi
    fi

    if [ "$SIMURGH_CONFIG_OK" = true ]; then
        SIMURGH_HOME=/etc/phoenix-simurgh /opt/phoenix-simurgh/simurgh relay \
            --panel-host 127.0.0.1 --panel-port 18787 \
            >> /var/log/phoenix-simurgh/relay.log 2>&1 &

        # Verify the bundled service and its UI assets in the background so a
        # Simurgh startup issue never delays PHOENIX's own panel.
        (
            attempt=0
            while [ "$attempt" -lt 30 ]; do
                health="$(curl --noproxy '*' -fsS --max-time 1 http://127.0.0.1:18787/healthz 2>/dev/null || true)"
                if [ -n "$health" ] && printf '%s' "$health" | python3 -c 'import json,sys; d=json.load(sys.stdin); raise SystemExit(0 if d.get("ok") is True and d.get("product") == "Simurgh Tunnel" and d.get("role") == "relay" else 1)' 2>/dev/null \
                    && curl --noproxy '*' -fsS --max-time 2 http://127.0.0.1:18787/app.js -o /dev/null 2>/dev/null \
                    && curl --noproxy '*' -fsS --max-time 2 http://127.0.0.1:18787/app.css -o /dev/null 2>/dev/null; then
                    echo "[Simurgh] Tunnel service and panel assets are healthy."
                    exit 0
                fi
                attempt=$((attempt + 1))
                sleep 1
            done
            echo "[Simurgh] Panel health or UI assets failed; see /var/log/phoenix-simurgh/setup.log and relay.log. PHOENIX remains available."
        ) &
    fi
fi

# Run PHOENIX as the container's primary process.
exec /app/x-ui
