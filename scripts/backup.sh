#!/bin/bash
set -e

# BDS Mail - Daily PostgreSQL backup to S3
# Usage: backup.sh [s3-bucket-name]
# Cron:  0 3 * * * /opt/bdsmail/scripts/backup.sh your-bucket >> /var/log/bdsmail-backup.log 2>&1

S3_BUCKET="${1:?Usage: backup.sh <s3-bucket-name>}"
BACKUP_DIR="/opt/bdsmail/backups"
DB_NAME="bdsmail"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
FILENAME="bdsmail_${TIMESTAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"

# Dump and compress
echo "[$(date)] Starting backup..."
sudo -u postgres pg_dump -d "$DB_NAME" | gzip > "$BACKUP_DIR/$FILENAME"
echo "[$(date)] Dump complete: $FILENAME ($(du -h "$BACKUP_DIR/$FILENAME" | cut -f1))"

# Upload to S3
aws s3 cp "$BACKUP_DIR/$FILENAME" "s3://${S3_BUCKET}/db/$FILENAME"
echo "[$(date)] Uploaded to s3://${S3_BUCKET}/db/$FILENAME"

# Keep only last 7 local backups
ls -t "$BACKUP_DIR"/bdsmail_*.sql.gz 2>/dev/null | tail -n +8 | xargs -r rm
echo "[$(date)] Local rotation: kept last 7"

# Remove S3 backups older than 30 days
CUTOFF=$(date -d '30 days ago' +%Y%m%d 2>/dev/null || date -v-30d +%Y%m%d)
aws s3 ls "s3://${S3_BUCKET}/db/" | awk '{print $4}' | while read -r f; do
    file_date=$(echo "$f" | grep -oP '\d{8}' | head -1)
    if [ -n "$file_date" ] && [ "$file_date" -lt "$CUTOFF" ]; then
        aws s3 rm "s3://${S3_BUCKET}/db/$f"
        echo "[$(date)] Removed old backup: $f"
    fi
done

echo "[$(date)] Backup complete."
