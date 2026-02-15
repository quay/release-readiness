#!/usr/bin/env bash
#
# Reference script for uploading snapshot data from a Konflux Integration Test
# Scenario (ITS) to S3. Run this after tests complete to feed the release
# readiness dashboard.
#
# Expected environment variables (set via Tekton workspace secrets):
#   SNAPSHOT              - Raw Konflux Snapshot CR JSON (provided by Konflux)
#   S3_ENDPOINT           - S3-compatible endpoint URL
#   S3_REGION             - S3 region (e.g. "us-east-1", "garage")
#   S3_BUCKET             - Target bucket name
#   AWS_ACCESS_KEY_ID     - S3 access key
#   AWS_SECRET_ACCESS_KEY - S3 secret key
#
# Expected directory structure for JUnit results:
#   ${JUNIT_DIR}/
#     {scenario-name}/
#       *.xml
#
# Resulting S3 layout:
#   {application}/snapshots/{snapshot-name}/snapshot.json
#   {application}/snapshots/{snapshot-name}/junit/{scenario}/*.xml
#
# Requires: aws, jq

set -euo pipefail

JUNIT_DIR="${JUNIT_DIR:-/tmp/junit}"

for var in SNAPSHOT S3_ENDPOINT S3_REGION S3_BUCKET AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY; do
    if [[ -z "${!var:-}" ]]; then
        echo "error: $var is not set" >&2
        exit 1
    fi
done

APPLICATION=$(echo "${SNAPSHOT}" | jq -r '.spec.application')
SNAPSHOT_NAME=$(echo "${SNAPSHOT}" | jq -r '.metadata.name')
S3_BASE="s3://${S3_BUCKET}/${APPLICATION}/snapshots/${SNAPSHOT_NAME}"

# 1. Upload the raw Snapshot CR JSON.
echo "Uploading snapshot CR: ${S3_BASE}/snapshot.json"
echo "${SNAPSHOT}" | aws s3 cp \
    --endpoint-url "${S3_ENDPOINT}" \
    --region "${S3_REGION}" \
    --content-type "application/json" \
    - "${S3_BASE}/snapshot.json"

# 2. Upload JUnit XML results per scenario.
if [[ -d "${JUNIT_DIR}" ]]; then
    for scenario_dir in "${JUNIT_DIR}"/*/; do
        [[ -d "${scenario_dir}" ]] || continue
        scenario=$(basename "${scenario_dir}")
        echo "Uploading JUnit results: ${S3_BASE}/junit/${scenario}/"
        aws s3 cp "${scenario_dir}" "${S3_BASE}/junit/${scenario}/" \
            --endpoint-url "${S3_ENDPOINT}" \
            --region "${S3_REGION}" \
            --recursive --exclude "*" --include "*.xml"
    done
else
    echo "No JUnit directory found at ${JUNIT_DIR}, skipping test results"
fi

echo "Done: ${S3_BASE}/"
