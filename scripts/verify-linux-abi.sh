#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 <amd64|arm64> <bins-dir>" >&2
    exit 2
fi

arch="$1"
bins_dir="$2"
baseline="2.28"
binaries=(pngquant oxipng cjpeg-static djpeg-static zxing-reader)

case "$arch" in
    amd64) expected_machine="Advanced Micro Devices X86-64" ;;
    arm64) expected_machine="AArch64" ;;
    *) echo "unsupported architecture: $arch" >&2; exit 2 ;;
esac

allowed_dependency() {
    case "$1" in
        libc.so.6|libm.so.6|libgcc_s.so.1|libpthread.so.0|libdl.so.2|librt.so.1) return 0 ;;
        *) return 1 ;;
    esac
}

read_elf_info() {
    local binary="$1"
    local header version_info dynamic_info machine versions_text
    if ! header="$(LC_ALL=C readelf -h "$binary")"; then
        echo "ERROR: invalid ELF binary: $binary" >&2
        exit 1
    fi
    machine="$(awk -F: '/Machine:/{gsub(/^[[:space:]]+/, "", $2); print $2; exit}' <<< "$header")"
    if [[ "$machine" != "$expected_machine" ]]; then
        echo "ERROR: $binary has ELF machine $machine; expected $expected_machine" >&2
        exit 1
    fi
    if ! version_info="$(LC_ALL=C readelf --version-info "$binary")"; then
        echo "ERROR: failed to read ELF version info: $binary" >&2
        exit 1
    fi
    if ! dynamic_info="$(LC_ALL=C readelf -d "$binary")"; then
        echo "ERROR: failed to read ELF dynamic section: $binary" >&2
        exit 1
    fi
    versions_text="$(printf '%s\n' "$version_info" | sed -nE 's/.*GLIBC_([0-9]+(\.[0-9]+)+).*/\1/p' | sort -Vu)"
    if [[ -z "$versions_text" ]]; then
        printf '%-20s no GLIBC symbol dependency\n' "$(basename "$binary")"
    else
        local max_version
        max_version="$(tail -n 1 <<< "$versions_text")"
        if [[ "$(printf '%s\n%s\n' "$max_version" "$baseline" | sort -V | tail -1)" != "$baseline" ]]; then
            echo "ERROR: $binary requires GLIBC_$max_version; baseline is GLIBC_$baseline" >&2
            exit 1
        fi
        printf '%-20s GLIBC <= %s\n' "$(basename "$binary")" "$max_version"
    fi

    local dependency
    while IFS= read -r dependency; do
        [[ -z "$dependency" ]] && continue
        if ! allowed_dependency "$dependency"; then
            echo "ERROR: $binary requires unsupported shared library: $dependency" >&2
            exit 1
        fi
    done <<< "$(awk -F'[][]' '/NEEDED/ { print $2 }' <<< "$dynamic_info")"
}

for name in "${binaries[@]}"; do
    binary="$bins_dir/$name"
    if [[ ! -x "$binary" ]]; then
        echo "missing binary: $binary" >&2
        exit 1
    fi
    read_elf_info "$binary"
done
