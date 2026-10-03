#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
    echo "usage: $0 <output-dir>" >&2
    exit 2
fi

output_dir="$(realpath -m "$1")"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
mkdir -p "$output_dir"

echo "Building Linux compression binaries into: $output_dir"

git clone --depth 1 --branch 3.0.3 --recursive \
    https://github.com/kornelski/pngquant.git "$work_dir/pngquant"
pngquant_lock=()
if [[ -f "$work_dir/pngquant/Cargo.lock" ]]; then
    pngquant_lock+=(--locked)
fi
cargo build --manifest-path "$work_dir/pngquant/Cargo.toml" --release \
    "${pngquant_lock[@]}" --features static,z-static
install -m 0755 "$work_dir/pngquant/target/release/pngquant" "$output_dir/pngquant"

git clone --depth 1 --branch v10.1.0 https://github.com/oxipng/oxipng.git "$work_dir/oxipng"
oxipng_lock=()
if [[ -f "$work_dir/oxipng/Cargo.lock" ]]; then
    oxipng_lock+=(--locked)
fi
cargo build --manifest-path "$work_dir/oxipng/Cargo.toml" --release "${oxipng_lock[@]}"
install -m 0755 "$work_dir/oxipng/target/release/oxipng" "$output_dir/oxipng"

git clone --depth 1 --branch 3.1.3 https://github.com/libjpeg-turbo/libjpeg-turbo.git "$work_dir/libjpeg-turbo"
cmake -S "$work_dir/libjpeg-turbo" -B "$work_dir/libjpeg-turbo/build" \
    -DENABLE_SHARED=FALSE \
    -DENABLE_STATIC=TRUE \
    -DWITH_TESTS=FALSE \
    -DWITH_TURBOJPEG=FALSE \
    -DCMAKE_BUILD_TYPE=Release
cmake --build "$work_dir/libjpeg-turbo/build" --target cjpeg-static djpeg-static --parallel
install -m 0755 "$work_dir/libjpeg-turbo/build/cjpeg-static" "$output_dir/cjpeg-static"
install -m 0755 "$work_dir/libjpeg-turbo/build/djpeg-static" "$output_dir/djpeg-static"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cmake -S "$repo_root/native/zxing-reader" -B "$work_dir/zxing-reader" -DCMAKE_BUILD_TYPE=Release
cmake --build "$work_dir/zxing-reader" --parallel
cmake --install "$work_dir/zxing-reader" --component ITBReader --prefix "$output_dir"

for binary in pngquant oxipng cjpeg-static djpeg-static zxing-reader; do
    strip --strip-unneeded "$output_dir/$binary" || true
done
