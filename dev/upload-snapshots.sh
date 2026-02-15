#!/usr/bin/env bash
#
# Upload raw Konflux Snapshot CR JSON to S3 for dev/testing.
#
# Fetches Snapshot CRs from a Kubernetes namespace, filters out deleted ones,
# keeps the latest N per application, and uploads each as:
#   {application}/snapshots/{snapshot-name}/snapshot.json
#
# Usage:
#   ./dev/upload-snapshots.sh [-n namespace] [-k kubeconfig] [-m max-per-app]
#
# Requires: kubectl, jq, aws (or mc)
# S3 credentials: source dev/s3.env before running, or export S3_ENDPOINT,
#   S3_REGION, S3_BUCKET, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY.

set -euo pipefail

NAMESPACE="quay-eng-tenant"
KUBECONFIG_PATH="/tmp/kubeconfig"
MAX_PER_APP=5

while getopts "n:k:m:" opt; do
    case $opt in
        n) NAMESPACE="$OPTARG" ;;
        k) KUBECONFIG_PATH="$OPTARG" ;;
        m) MAX_PER_APP="$OPTARG" ;;
        *) echo "Usage: $0 [-n namespace] [-k kubeconfig] [-m max-per-app]" >&2; exit 1 ;;
    esac
done

# Load S3 credentials from dev/s3.env if not already set.
if [[ -z "${S3_ENDPOINT:-}" ]] && [[ -f dev/s3.env ]]; then
    # shellcheck disable=SC1091
    source dev/s3.env
fi

for var in S3_ENDPOINT S3_REGION S3_BUCKET AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY; do
    if [[ -z "${!var:-}" ]]; then
        echo "error: $var is not set" >&2
        exit 1
    fi
done

echo "Fetching snapshots from namespace $NAMESPACE..."
raw=$(kubectl --kubeconfig "$KUBECONFIG_PATH" get snapshots -n "$NAMESPACE" -o json)

# Filter out deleted snapshots and extract items.
items=$(echo "$raw" | jq '[.items[] | select(.metadata.deletionTimestamp == null)]')
count=$(echo "$items" | jq 'length')
echo "Found $count active snapshots"

if [[ "$count" -eq 0 ]]; then
    echo "No snapshots to upload"
    exit 0
fi

# Group by application, sort by creationTimestamp desc, keep latest N.
apps=$(echo "$items" | jq -r '[.[].spec.application] | unique | .[]')

uploaded=0
for app in $apps; do
    echo "Processing application: $app"

    # Get snapshots for this app, sorted newest first, limited to MAX_PER_APP.
    app_snapshots=$(echo "$items" | jq --arg app "$app" --argjson max "$MAX_PER_APP" \
        '[.[] | select(.spec.application == $app)] | sort_by(.metadata.creationTimestamp) | reverse | .[:$max]')

    snap_count=$(echo "$app_snapshots" | jq 'length')
    for i in $(seq 0 $((snap_count - 1))); do
        snapshot=$(echo "$app_snapshots" | jq ".[$i]")
        name=$(echo "$snapshot" | jq -r '.metadata.name')
        key="${app}/snapshots/${name}/snapshot.json"

        echo "$snapshot" | aws s3 cp \
            --endpoint-url "$S3_ENDPOINT" \
            --region "$S3_REGION" \
            --content-type "application/json" \
            - "s3://${S3_BUCKET}/${key}"

        echo "  uploaded: s3://${S3_BUCKET}/${key}"
        uploaded=$((uploaded + 1))
    done
done

echo ""
echo "summary: $uploaded snapshots uploaded across $(echo "$apps" | wc -w | tr -d ' ') applications"
