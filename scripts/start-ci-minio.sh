#!/usr/bin/env bash
# CI-only MinIO, fixed to the release formerly used by the removed image.
set -euo pipefail

: "${RUNNER_TEMP:?RUNNER_TEMP is required}"
minio_commit=20960b6a2ddb9594ee418035b3c7c7fe92ae6a12
minio_dir="$RUNNER_TEMP/itb-minio"
mkdir -p "$minio_dir"
git init "$minio_dir/source"
git -C "$minio_dir/source" fetch --depth 1 https://github.com/minio/minio.git "$minio_commit"
git -C "$minio_dir/source" checkout --detach FETCH_HEAD
[[ "$(git -C "$minio_dir/source" rev-parse HEAD)" == "$minio_commit" ]]
(
    cd "$minio_dir/source"
    CGO_ENABLED=0 go build -trimpath -o "$minio_dir/minio" .
)
mkdir -p "$minio_dir/data"
MINIO_ROOT_USER="${ITB_TEST_MINIO_ACCESS_KEY:?}" \
MINIO_ROOT_PASSWORD="${ITB_TEST_MINIO_SECRET_KEY:?}" \
MINIO_BROWSER=off MINIO_UPDATE=off \
nohup "$minio_dir/minio" server "$minio_dir/data" --address 127.0.0.1:9000 \
    >"$minio_dir/server.log" 2>&1 &
printf '%s\n' "$!" >"$minio_dir/server.pid"
for attempt in {1..30}; do
    if curl -sf http://127.0.0.1:9000/minio/health/live >/dev/null; then
        echo "MinIO is ready (probe $attempt)"
        exit 0
    fi
    sleep 2
done
echo "::error::MinIO failed to become healthy"
tail -100 "$minio_dir/server.log"
exit 1
