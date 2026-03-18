#!/bin/bash
# Deploy BDS Mail on a GCP VM
# Run this script on the VM as root
#
# This script:
#   1. Copies the binary and web assets
#   2. Installs and configures certbot (auto-renewal via systemd timer)
#   3. Generates DKIM keys for each domain
#   4. Creates the systemd service
#   5. Starts everything

set -e

APP_DIR="/opt/bdsmail"
DKIM_DIR="${APP_DIR}/dkim"
DOMAINS="${BDS_DOMAINS:?Set BDS_DOMAINS (comma-separated, e.g. domain1.com,domain2.com)}"
DATABASE_URL="${DATABASE_URL:?Set DATABASE_URL (e.g. postgres://user:pass@host:5432/bdsmail?sslmode=require)}"

echo "=== BDS Mail Deployment ==="
echo "Domains: ${DOMAINS}"
echo ""

# Parse domains into array
IFS=',' read -ra DOMAIN_ARRAY <<< "$DOMAINS"

# ---- Create directories ----
mkdir -p "${APP_DIR}" "${DKIM_DIR}"

# ---- Copy binary and assets ----
if [ -f "./bdsmail" ]; then
    cp ./bdsmail "${APP_DIR}/bdsmail"
    chmod +x "${APP_DIR}/bdsmail"
    echo "[OK] Binary copied"
fi
cp -r ./web "${APP_DIR}/"
cp ./scripts/generate_dkim.sh "${APP_DIR}/generate_dkim.sh"
chmod +x "${APP_DIR}/generate_dkim.sh"
echo "[OK] Web assets and scripts copied"

# ---- Install certbot ----
if ! command -v certbot &>/dev/null; then
    echo "[..] Installing certbot..."
    apt-get update -qq && apt-get install -y -qq certbot > /dev/null
    echo "[OK] Certbot installed"
else
    echo "[OK] Certbot already installed"
fi

# ---- Obtain TLS certificates ----
CERTBOT_DOMAINS=""
FIRST_DOMAIN=""
for domain in "${DOMAIN_ARRAY[@]}"; do
    domain=$(echo "$domain" | xargs)
    CERTBOT_DOMAINS="${CERTBOT_DOMAINS} -d mail.${domain}"
    if [ -z "$FIRST_DOMAIN" ]; then
        FIRST_DOMAIN="mail.${domain}"
    fi
done

if [ ! -f "/etc/letsencrypt/live/${FIRST_DOMAIN}/fullchain.pem" ]; then
    echo "[..] Obtaining TLS certificates for:${CERTBOT_DOMAINS}"
    certbot certonly --standalone ${CERTBOT_DOMAINS} \
        --non-interactive --agree-tos \
        --email "postmaster@${DOMAIN_ARRAY[0]}"
    echo "[OK] TLS certificates obtained"
else
    echo "[OK] TLS certificates already exist"
fi

# ---- Set up automatic certificate renewal ----
cat > /etc/systemd/system/certbot-renewal.service << 'EOF'
[Unit]
Description=Certbot certificate renewal

[Service]
Type=oneshot
ExecStart=/usr/bin/certbot renew --quiet --deploy-hook "systemctl restart bdsmail"
EOF

cat > /etc/systemd/system/certbot-renewal.timer << 'EOF'
[Unit]
Description=Run certbot renewal daily

[Timer]
OnCalendar=*-*-* 03:00:00
RandomizedDelaySec=3600
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable certbot-renewal.timer
systemctl start certbot-renewal.timer
echo "[OK] Automatic certificate renewal configured"

# ---- Generate DKIM keys ----
echo ""
echo "=== DKIM Key Generation ==="
for domain in "${DOMAIN_ARRAY[@]}"; do
    domain=$(echo "$domain" | xargs)
    "${APP_DIR}/generate_dkim.sh" "${domain}" "${DKIM_DIR}"
done

# ---- Install ClamAV (optional) ----
if [ "${BDS_CLAMAV_ENABLED}" = "true" ]; then
    if ! command -v clamd &>/dev/null; then
        echo "[..] Installing ClamAV..."
        apt-get update -qq && apt-get install -y -qq clamav clamav-daemon > /dev/null
        echo "[OK] ClamAV installed"
    else
        echo "[OK] ClamAV already installed"
    fi
    systemctl stop clamav-freshclam 2>/dev/null || true
    freshclam --quiet
    systemctl enable clamav-daemon clamav-freshclam
    systemctl start clamav-freshclam
    systemctl start clamav-daemon
    echo "[OK] ClamAV daemon started with updated virus definitions"
fi

# ---- Create environment file ----
cat > "${APP_DIR}/.env" << EOF
BDS_DOMAINS=${DOMAINS}
BDS_SMTP_PORT=25
BDS_POP3_PORT=110
BDS_IMAP_PORT=143
BDS_HTTPS_PORT=443
BDS_TLS_CERT=/etc/letsencrypt/live/${FIRST_DOMAIN}/fullchain.pem
BDS_TLS_KEY=/etc/letsencrypt/live/${FIRST_DOMAIN}/privkey.pem
BDS_GCS_BUCKET=${BDS_GCS_BUCKET:-bdsmail-bodies}
BDS_DKIM_KEY_DIR=${DKIM_DIR}
BDS_DKIM_SELECTOR=default
DATABASE_URL=${DATABASE_URL}
# Security checks (uncomment to enable)
# BDS_CLAMAV_ENABLED=true
# BDS_CLAMAV_ADDRESS=unix:/var/run/clamav/clamd.ctl
# BDS_SAFEBROWSING_ENABLED=true
# BDS_SAFEBROWSING_API_KEY=your-api-key
# BDS_AUTH_CHECK_ENABLED=true
EOF

if [ "${BDS_CLAMAV_ENABLED}" = "true" ]; then
    echo "BDS_CLAMAV_ENABLED=true" >> "${APP_DIR}/.env"
    echo "BDS_CLAMAV_ADDRESS=unix:/var/run/clamav/clamd.ctl" >> "${APP_DIR}/.env"
fi
if [ -n "${BDS_SAFEBROWSING_API_KEY}" ]; then
    echo "BDS_SAFEBROWSING_ENABLED=true" >> "${APP_DIR}/.env"
    echo "BDS_SAFEBROWSING_API_KEY=${BDS_SAFEBROWSING_API_KEY}" >> "${APP_DIR}/.env"
fi
if [ "${BDS_AUTH_CHECK_ENABLED}" = "true" ]; then
    echo "BDS_AUTH_CHECK_ENABLED=true" >> "${APP_DIR}/.env"
fi

echo "[OK] Environment file created"

# ---- Create systemd service ----
cat > /etc/systemd/system/bdsmail.service << EOF
[Unit]
Description=BDS Mail Server
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=${APP_DIR}
EnvironmentFile=${APP_DIR}/.env
ExecStart=${APP_DIR}/bdsmail
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=${APP_DIR}
ReadOnlyPaths=/etc/letsencrypt

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable bdsmail
systemctl restart bdsmail

echo ""
echo "=== Deployment Complete ==="
echo ""
echo "Service status:  systemctl status bdsmail"
echo "View logs:       journalctl -u bdsmail -f"
echo "Cert renewal:    systemctl list-timers | grep certbot"
echo ""
echo "=== Next Steps ==="
echo ""
echo "1. Add the DNS records printed above to GoDaddy for each domain"
echo "2. Create users:"
for domain in "${DOMAIN_ARRAY[@]}"; do
    domain=$(echo "$domain" | xargs)
    echo "     ${APP_DIR}/bdsmail -adduser user@${domain} -password 'yourpassword'"
done
echo ""
echo "3. Verify DNS propagation:"
echo "     dig MX ${DOMAIN_ARRAY[0]}"
echo "     dig TXT default._domainkey.${DOMAIN_ARRAY[0]}"
echo ""
echo "4. Test the web UI:"
echo "     https://mail.${DOMAIN_ARRAY[0]}"
