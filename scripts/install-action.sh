#!/usr/bin/env bash
set -euo pipefail

version=${SCIURUS_VERSION:-}
if [[ ! "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo '::error::The version input must be a published vMAJOR.MINOR.PATCH release.' >&2
  exit 1
fi

case "${RUNNER_OS:-}/${RUNNER_ARCH:-}" in
  Linux/X64) os=linux; arch=amd64 ;;
  Linux/ARM64) os=linux; arch=arm64 ;;
  macOS/X64) os=darwin; arch=amd64 ;;
  macOS/ARM64) os=darwin; arch=arm64 ;;
  Windows/X64) os=windows; arch=amd64 ;;
  *) echo "::error::Unsupported Sciurus runner: ${RUNNER_OS:-unknown}/${RUNNER_ARCH:-unknown}." >&2; exit 1 ;;
esac

runner_temp=${RUNNER_TEMP:?RUNNER_TEMP is required}
github_path=${GITHUB_PATH:?GITHUB_PATH is required}
if [[ "$os" == windows ]]; then
  runner_temp=$(cygpath -u "$runner_temp")
  github_path=$(cygpath -u "$github_path")
fi
cache_directory="$runner_temp/sciurus-releases/$version/$os-$arch"
archive="sciurus_${version#v}_${os}_${arch}.tar.gz"
executable=sciurus
if [[ "$os" == windows ]]; then
  archive="sciurus_${version#v}_${os}_${arch}.zip"
  executable=sciurus.exe
fi

download_directory=$(mktemp -d "$runner_temp/sciurus-download.XXXXXX")
trap 'rm -rf "$download_directory"' EXIT
if [[ ! -s "$cache_directory/checksums.txt" || ! -s "$cache_directory/$archive" ]]; then
  base_url="https://github.com/mhemmings/sciurus/releases/download/$version"
  curl --fail --silent --show-error --location --retry 3 --retry-delay 1 \
    --output "$download_directory/checksums.txt" "$base_url/checksums.txt"
  curl --fail --silent --show-error --location --retry 3 --retry-delay 1 \
    --output "$download_directory/$archive" "$base_url/$archive"
  mkdir -p "$cache_directory"
  mv "$download_directory/$archive" "$cache_directory/$archive"
  mv "$download_directory/checksums.txt" "$cache_directory/checksums.txt"
else
  printf 'Reusing downloaded Sciurus %s for %s/%s.\n' "$version" "$os" "$arch"
fi

expected=$(awk -v archive="$archive" '$2 == archive { print $1 }' "$cache_directory/checksums.txt")
if [[ ! "$expected" =~ ^[a-f0-9]{64}$ ]]; then
  echo "::error::Expected exactly one SHA-256 checksum for $archive." >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum < "$cache_directory/$archive")
else
  actual=$(shasum -a 256 < "$cache_directory/$archive")
fi
actual=${actual%% *}
if [[ "$actual" != "$expected" ]]; then
  echo "::error::SHA-256 checksum mismatch for $archive." >&2
  exit 1
fi

# Recheck cached downloads before each fresh extraction; installed executables
# from an earlier step are never treated as proof of archive integrity.
install_directory=$(mktemp -d "$runner_temp/sciurus-bin.XXXXXX")
if [[ "$os" == windows ]]; then
  unzip -q "$cache_directory/$archive" -d "$install_directory"
else
  tar -xzf "$cache_directory/$archive" -C "$install_directory"
fi
chmod +x "$install_directory/$executable"
if [[ "$("$install_directory/$executable" version)" != "Sciurus $version" ]]; then
  echo '::error::The downloaded Sciurus executable reports an unexpected version.' >&2
  exit 1
fi
if [[ "$os" == windows ]]; then
  install_directory=$(cygpath -w "$install_directory")
fi
printf '%s\n' "$install_directory" >> "$github_path"
printf 'Installed verified Sciurus %s for %s/%s.\n' "$version" "$os" "$arch"
