#!/bin/bash
set -e

# BDS Mail - Renew Let's Encrypt certificates and copy to SSL directory
# Cron: 0 2 * * * /opt/bdsmail/scripts/renew_certs.sh >> /var/log/bdsmail-certs.log 2>&1

SSL_DIR="${1:-/opt/bdsmail/ssl}"

echo "[$(date)] Starting certificate renewal..."
certbot renew --quiet

# Copy renewed certs to SSL directory
for dir in /etc/letsencrypt/live/*/; do
    domain=$(basename "$dir")
    # Skip README and backup dirs
    [ "$domain" = "README" ] && continue

    src_cert="$dir/fullchain.pem"
    src_key="$dir/privkey.pem"
    dst_dir="$SSL_DIR/$domain"

    [ ! -f "$src_cert" ] && continue

    mkdir -p "$dst_dir"
    cp -L "$src_cert" "$dst_dir/fullchain.pem"
    cp -L "$src_key" "$dst_dir/privkey.pem"
    chmod 600 "$dst_dir/privkey.pem"
    echo "[$(date)] Renewed: $domain"
done

# Signal bdsmail to reload (graceful — no connection drop)
if systemctl is-active --quiet bdsmail; then
    systemctl reload bdsmail 2>/dev/null || systemctl restart bdsmail
    echo "[$(date)] bdsmail reloaded"
fi

echo "[$(date)] Certificate renewal complete."
