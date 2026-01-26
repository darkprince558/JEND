#!/bin/bash
set -e

# JEND Web Deployment Script
# Usage: ./scripts/deploy-web.sh <bucket-name> <distribution-id> [dist-folder]

BUCKET_NAME=$1
DIST_ID=$2
DIST_FOLDER=${3:-"web/dist"}

if [ -z "$BUCKET_NAME" ] || [ -z "$DIST_ID" ]; then
    echo "Usage: $0 <bucket-name> <distribution-id> [dist-folder]"
    echo "Error: Missing arguments."
    exit 1
fi

if [ ! -d "$DIST_FOLDER" ]; then
    echo "Error: Directory '$DIST_FOLDER' does not exist."
    exit 1
fi

echo "Deploying '$DIST_FOLDER' to s3://$BUCKET_NAME..."
aws s3 sync "$DIST_FOLDER" "s3://$BUCKET_NAME" --delete

echo "Invalidating CloudFront cache for distribution $DIST_ID..."
aws cloudfront create-invalidation --distribution-id "$DIST_ID" --paths "/*"

echo "Deployment complete!"
