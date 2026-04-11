#!/bin/bash
set -e

# BDS Mail - Daily PostgreSQL backup to Cloudflare R2
# Usage: backup.sh [r2-bucket-name]
# Cron:  0 3 * * * /opt/bdsmail/scripts/backup.sh bdsmail >> /var/log/bdsmail-backup.log 2>&1

R2_BUCKET="${1:?Usage: backup.sh <r2-bucket-name>}"
BACKUP_DIR="/opt/bdsmail/backups"
DB_NAME="bdsmail"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
FILENAME="bdsmail_${TIMESTAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"

# Dump and compress
echo "[$(date)] Starting backup..."
sudo -u postgres pg_dump -d "$DB_NAME" | gzip > "$BACKUP_DIR/$FILENAME"
echo "[$(date)] Dump complete: $FILENAME ($(du -h "$BACKUP_DIR/$FILENAME" | cut -f1))"

# Upload to R2
rclone copy "$BACKUP_DIR/$FILENAME" "r2:${R2_BUCKET}/db/"
echo "[$(date)] Uploaded to r2:${R2_BUCKET}/db/$FILENAME"

# Keep only last 7 local backups
ls -t "$BACKUP_DIR"/bdsmail_*.sql.gz 2>/dev/null | tail -n +8 | xargs -r rm
echo "[$(date)] Local rotation: kept last 7"

# Remove R2 backups older than 30 days
rclone delete "r2:${R2_BUCKET}/db/" --min-age 30d
echo "[$(date)] R2 rotation: removed files older than 30 days"

echo "[$(date)] Backup complete."
