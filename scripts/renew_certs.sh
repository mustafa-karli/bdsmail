#!/bin/bash
set -e

# BDS Mail - Renew Let's Encrypt certificates and copy to SSL directory
# Cron: 0 2 * * * /opt/bdsmail/scripts/renew_certs.sh /opt/bdsmail/ssl >> /var/log/bdsmail-certs.log 2>&1

SSL_DIR="${1:-/opt/bdsmail/ssl}"
CHANGED=0

echo "[$(date)] Starting certificate renewal..."
certbot renew --quiet

# Copy only if cert is newer than our copy
for dir in /etc/letsencrypt/live/*/; do
    certname=$(basename "$dir")

    [ "$certname" = "README" ] && continue
    [ ! -f "$dir/fullchain.pem" ] && continue

    # Strip mail./mailsrv. prefix to get domain name
    domain="${certname#mail.}"
    domain="${domain#mailsrv.}"
    dst_dir="$SSL_DIR/$domain"
    dst_cert="$dst_dir/fullchain.pem"

    # Skip if our copy is up to date
    if [ -f "$dst_cert" ] && [ ! "$dir/fullchain.pem" -nt "$dst_cert" ]; then
        continue
    fi

    mkdir -p "$dst_dir"
    cp -L "$dir/fullchain.pem" "$dst_dir/fullchain.pem"
    cp -L "$dir/privkey.pem" "$dst_dir/privkey.pem"
    chown bdsmail:bdsmail "$dst_dir"/*.pem
    chmod 600 "$dst_dir/privkey.pem"
    echo "[$(date)] Updated cert: $certname → $dst_dir"
    CHANGED=1
done

# Restart bdsmail only if certs changed
if [ "$CHANGED" -eq 1 ] && systemctl is-active --quiet bdsmail; then
    systemctl restart bdsmail
    echo "[$(date)] bdsmail restarted (certs updated)"
fi

echo "[$(date)] Certificate renewal complete."
