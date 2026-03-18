#!/bin/bash
# Create GCS bucket for BDS Mail body storage

set -e

PROJECT_ID="${GCP_PROJECT_ID:?Set GCP_PROJECT_ID}"
BUCKET_NAME="${BDS_GCS_BUCKET:-bdsmail-bodies}"
REGION="${GCP_REGION:-us-central1}"

echo "Creating GCS bucket: gs://${BUCKET_NAME}"

gcloud storage buckets create "gs://${BUCKET_NAME}" \
    --project="${PROJECT_ID}" \
    --location="${REGION}" \
    --uniform-bucket-level-access

echo "Bucket created successfully."
echo ""
echo "To grant access to a service account:"
echo "  gcloud storage buckets add-iam-policy-binding gs://${BUCKET_NAME} \\"
echo "    --member=serviceAccount:YOUR_SA@${PROJECT_ID}.iam.gserviceaccount.com \\"
echo "    --role=roles/storage.objectAdmin"
