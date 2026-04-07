#!/bin/bash
set -e

# BDS Mail - Deploy script (run from dev machine)
# Usage:
#   deploy.sh              — deploy everything (binary + web + vue)
#   deploy.sh bin          — deploy Go binary only
#   deploy.sh web          — deploy Go templates + static CSS only
#   deploy.sh vue          — build and deploy Vue SPA only
#   deploy.sh all          — build all + deploy all

HOST="bdsmail"
REMOTE="/opt/bdsmail"
TARGET="${1:-all}"

# Fix permissions and restart service
fix_and_restart() {
    ssh ${HOST} "sudo chown -R bdsmail:bdsmail ${REMOTE} && sudo chmod -R g+w ${REMOTE} && sudo chmod 600 ${REMOTE}/sec/secrets.json && sudo systemctl restart bdsmail"
}

case "$TARGET" in
  bin)
    echo "=== Building Go binary ==="
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/
    echo "=== Uploading binary ==="
    scp bin/bdsmail ${HOST}:${REMOTE}/bin/
    echo "=== Restarting service ==="
    fix_and_restart
    ;;

  web)
    echo "=== Uploading templates + static ==="
    scp web/templates/* ${HOST}:${REMOTE}/web/templates/
    scp web/static/* ${HOST}:${REMOTE}/web/static/
    echo "=== Restarting service ==="
    fix_and_restart
    ;;

  vue)
    echo "=== Building Vue SPA ==="
    cd web/vue && npm run build && cd ../..
    echo "=== Uploading dist ==="
    scp -r web/vue/dist/* ${HOST}:${REMOTE}/web/vue/dist/
    ssh ${HOST} "sudo chown -R bdsmail:bdsmail ${REMOTE}/web/vue"
    echo "=== Done (no restart needed for static files) ==="
    ;;

  all)
    echo "=== Building Go binary ==="
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/bdsmail ./cmd/bdsmail/
    echo "=== Building Vue SPA ==="
    cd web/vue && npm run build && cd ../..
    echo "=== Uploading everything ==="
    scp bin/bdsmail ${HOST}:${REMOTE}/bin/
    scp web/templates/* ${HOST}:${REMOTE}/web/templates/
    scp web/static/* ${HOST}:${REMOTE}/web/static/
    scp -r web/vue/dist/* ${HOST}:${REMOTE}/web/vue/dist/
    scp scripts/*.sh scripts/*.service ${HOST}:${REMOTE}/scripts/
    scp sql/*.sql sql/*.md ${HOST}:${REMOTE}/sql/
    echo "=== Fixing permissions and restarting ==="
    fix_and_restart
    ;;

  *)
    echo "Usage: deploy.sh [bin|web|vue|all]"
    exit 1
    ;;
esac

echo "=== Deploy complete ==="
