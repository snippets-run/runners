#!/bin/sh

set -eu

repository="https://github.com/snippets-run/runners/releases/latest/download"
install_dir="${INSTALL_DIR:-/usr/local/bin}"

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *)
    printf '%s\n' "error: run supports macOS and Linux only" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    printf '%s\n' "error: unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

archive="run_${os}_${arch}.tar.gz"
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

download() {
  if command -v curl >/dev/null 2>&1; then
    curl --fail --location --silent --show-error "$1" --output "$2"
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget --quiet --output-document="$2" "$1"
    return
  fi
  printf '%s\n' "error: install curl or wget first" >&2
  exit 1
}

download "$repository/$archive" "$temporary/$archive"
download "$repository/checksums.txt" "$temporary/checksums.txt"

expected="$(awk -v archive="$archive" '$2 == archive || $2 == "*" archive { print $1; exit }' "$temporary/checksums.txt")"
if [ -z "$expected" ]; then
  printf '%s\n' "error: no checksum found for $archive" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary/$archive" | awk '{ print $1 }')"
else
  actual="$(shasum -a 256 "$temporary/$archive" | awk '{ print $1 }')"
fi
if [ "$actual" != "$expected" ]; then
  printf '%s\n' "error: checksum verification failed" >&2
  exit 1
fi

tar -xzf "$temporary/$archive" -C "$temporary"

if mkdir -p "$install_dir" 2>/dev/null && [ -w "$install_dir" ]; then
  install -m 0755 "$temporary/run" "$install_dir/run"
elif command -v sudo >/dev/null 2>&1; then
  sudo mkdir -p "$install_dir"
  sudo install -m 0755 "$temporary/run" "$install_dir/run"
else
  printf '%s\n' "error: $install_dir is not writable and sudo is unavailable; set INSTALL_DIR to a writable path" >&2
  exit 1
fi

printf '%s\n' "Installed run to $install_dir/run"
