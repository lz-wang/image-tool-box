#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 <amd64|arm64> <output-dir>" >&2
    exit 2
fi

arch="$1"
output_dir="$2"
case "$arch" in
    amd64) image="quay.io/pypa/manylinux_2_28_x86_64@sha256:0bf9db09181f36be8cf0628332abb5d31855b7d0f372faa776f492ccd9d3100d" ;;
    arm64) image="quay.io/pypa/manylinux_2_28_aarch64@sha256:d3524021bb8b15d2258481be1ea46f1dfa8a9c7fc09bec43a6c5407945f9dc11" ;;
    *) echo "unsupported architecture: $arch" >&2; exit 2 ;;
esac

repo_root="$(git rev-parse --show-toplevel)"
mkdir -p "$repo_root/$output_dir"

docker run --rm \
    -v "$repo_root:/workspace" \
    -w /workspace \
    -e OUTPUT_DIR="/workspace/$output_dir" \
    -e HOST_UID="$(id -u)" \
    -e HOST_GID="$(id -g)" \
    "$image" \
    bash -lc '
        set -euo pipefail
        dnf install -y git nasm make cmake gcc gcc-c++ binutils curl pkgconf-pkg-config openssl
        export RUSTUP_HOME=/tmp/rustup
        export CARGO_HOME=/tmp/cargo
        export PATH="$CARGO_HOME/bin:$PATH"
        curl --proto "=https" --tlsv1.2 -sSf https://sh.rustup.rs |
            sh -s -- -y --profile minimal --default-toolchain 1.89.0
        /workspace/scripts/build-linux-bins.sh "$OUTPUT_DIR"
        chown -R "$HOST_UID:$HOST_GID" "$OUTPUT_DIR"
    '
