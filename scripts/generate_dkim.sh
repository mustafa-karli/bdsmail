#!/bin/bash
# Generate DKIM key pair for a domain
# Usage: ./generate_dkim.sh <domain> [output_dir]
#
# Creates:
#   - {output_dir}/{domain}.pem  (private key, used by bdsmail)
#   - Prints the DNS TXT record to add in your DNS provider

set -e

DOMAIN="${1:?Usage: $0 <domain> [output_dir]}"
OUTPUT_DIR="${2:-./dkim}"

mkdir -p "${OUTPUT_DIR}"

PRIVATE_KEY="${OUTPUT_DIR}/${DOMAIN}.pem"

if [ -f "${PRIVATE_KEY}" ]; then
    echo "DKIM key already exists for ${DOMAIN} at ${PRIVATE_KEY}"
    echo "Delete it first if you want to regenerate."
else
    # Generate 2048-bit RSA private key in PKCS#1 format
    openssl genrsa -out "${PRIVATE_KEY}" 2048 2>/dev/null
    chmod 600 "${PRIVATE_KEY}"
    echo "Generated DKIM private key: ${PRIVATE_KEY}"
fi

# Extract public key and format for DNS
PUBLIC_KEY=$(openssl rsa -in "${PRIVATE_KEY}" -pubout -outform DER 2>/dev/null | openssl base64 -A)

echo ""
echo "================================================================"
echo "  DKIM DNS Record for: ${DOMAIN}"
echo "================================================================"
echo ""
echo "  Add this TXT record in your DNS provider (e.g. GoDaddy):"
echo ""
echo "  Type:  TXT"
echo "  Name:  default._domainkey"
echo "  Value: v=DKIM1; k=rsa; p=${PUBLIC_KEY}"
echo ""
echo "================================================================"
echo ""
echo "  Also add a DMARC record:"
echo ""
echo "  Type:  TXT"
echo "  Name:  _dmarc"
echo "  Value: v=DMARC1; p=none; rua=mailto:postmaster@${DOMAIN}"
echo ""
echo "================================================================"
