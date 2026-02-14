#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="garage-dev"
IMAGE="docker.io/dxflrs/garage:v2.2.0"
BUCKET="quay-release-readiness"
KEY_NAME="dashboard-dev"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/s3.env"

garage() {
    podman exec "${CONTAINER_NAME}" /garage "$@" 2>/dev/null
}

# Stop existing container if running
if podman container exists "${CONTAINER_NAME}" 2>/dev/null; then
    echo "Stopping existing ${CONTAINER_NAME}..."
    podman rm -f "${CONTAINER_NAME}" >/dev/null
fi

# Clean up stale data from previous runs
rm -rf "${SCRIPT_DIR}/garage"
mkdir -p "${SCRIPT_DIR}/garage/meta"
mkdir -p "${SCRIPT_DIR}/garage/data"

echo "Starting GarageFS..."
podman run -d \
    --name "${CONTAINER_NAME}" \
    -p 3900:3900 \
    -p 3901:3901 \
    -p 3903:3903 \
    -v "${SCRIPT_DIR}/garage.toml:/etc/garage.toml:Z" \
    -v "${SCRIPT_DIR}/garage/meta:/var/lib/garage/meta:Z" \
    -v "${SCRIPT_DIR}/garage/data:/var/lib/garage/data:Z" \
    "${IMAGE}"

# Wait for garage to be ready
echo "Waiting for Garage to start..."
for i in $(seq 1 30); do
    if garage status >/dev/null 2>&1; then
        break
    fi
    sleep 0.5
done

# Get node ID and configure layout
NODE_ID=$(garage status | awk '/^[a-f0-9]/{print $1}')
echo "Node ID: ${NODE_ID}"

garage layout assign -z dc1 -c 1G "${NODE_ID}"
garage layout apply --version 1

# Create bucket
garage bucket create "${BUCKET}"

# Create API key and parse credentials
KEY_OUTPUT=$(garage key create "${KEY_NAME}")
KEY_ID=$(echo "${KEY_OUTPUT}" | awk '/^Key ID/{print $3}')
KEY_SECRET=$(echo "${KEY_OUTPUT}" | awk '/^Secret key/{print $3}')

# Grant permissions
garage bucket allow --read --write --owner "${BUCKET}" --key "${KEY_NAME}"

# Write env file for the dashboard
cat > "${ENV_FILE}" <<EOF
S3_ENDPOINT=http://localhost:3900
S3_REGION=garage
S3_BUCKET=${BUCKET}
AWS_ACCESS_KEY_ID=${KEY_ID}
AWS_SECRET_ACCESS_KEY=${KEY_SECRET}
EOF

echo ""
echo "GarageFS is running."
echo "  S3 API:  http://localhost:3900"
echo "  Admin:   http://localhost:3903"
echo "  Bucket:  ${BUCKET}"
echo ""
echo "Credentials written to ${ENV_FILE}"
echo ""
echo "To use with awscli:"
echo "  source ${ENV_FILE}"
echo "  aws --endpoint-url \${S3_ENDPOINT} s3 ls s3://${BUCKET}/"
echo ""
echo "To stop:"
echo "  podman rm -f ${CONTAINER_NAME}"
