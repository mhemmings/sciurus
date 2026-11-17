#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

version=${1:?usage: build-release.sh VERSION OUTPUT_DIRECTORY}
output_directory=${2:?usage: build-release.sh VERSION OUTPUT_DIRECTORY}
if [[ "$version" != dev && ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Version must be dev or vMAJOR.MINOR.PATCH.' >&2
  exit 1
fi

mkdir -p "$output_directory"
output_directory=$(cd "$output_directory" && pwd)
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
os=$(go env GOOS)
arch=$(go env GOARCH)
host_os=$(go env GOHOSTOS)
host_arch=$(go env GOHOSTARCH)
executable=sciurus
if [[ "$os" == windows ]]; then executable=sciurus.exe; fi

CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X github.com/mhemmings/sciurus/internal/app.version=$version" \
  -o "$stage/$executable" ./cmd/sciurus
cp LICENSE "$stage/LICENSE"
cp internal/diffimage/FONT-LICENSE.txt "$stage/FONT-LICENSE.txt"

archive="$output_directory/sciurus_${version#v}_${os}_${arch}"
if [[ "$os" == windows ]]; then
  archive="$archive.zip"
  (cd "$stage" && zip -q "$stage/release.zip" "$executable" LICENSE FONT-LICENSE.txt)
  mv "$stage/release.zip" "$archive"
  mkdir "$stage/extracted"
  unzip -q "$archive" -d "$stage/extracted"
else
  archive="$archive.tar.gz"
  tar -czf "$archive" -C "$stage" "$executable" LICENSE FONT-LICENSE.txt
  mkdir "$stage/extracted"
  tar -xzf "$archive" -C "$stage/extracted"
fi

cmp "$stage/$executable" "$stage/extracted/$executable"
cmp LICENSE "$stage/extracted/LICENSE"
cmp internal/diffimage/FONT-LICENSE.txt "$stage/extracted/FONT-LICENSE.txt"
go version -m "$stage/extracted/$executable" > "$stage/build-info"
grep -qFx $'\tbuild\tGOOS='"$os" "$stage/build-info"
grep -qFx $'\tbuild\tGOARCH='"$arch" "$stage/build-info"
grep -qFx $'\tbuild\tCGO_ENABLED=0' "$stage/build-info"

if [[ "$os" == "$host_os" && "$arch" == "$host_arch" ]]; then
  bash scripts/smoke-release.sh "$stage/extracted/$executable" "$version"
else
  # Trimmed build metadata omits linker flags; inspect the embedded version
  # without attempting to execute a binary for a different OS or architecture.
  LC_ALL=C strings -a -n 3 "$stage/extracted/$executable" > "$stage/strings"
  grep -qE "${version//./[.]}$" "$stage/strings"
  printf 'Cross-compiled binary checks passed for %s/%s (%s).\n' "$os" "$arch" "$version"
fi
printf 'Packaged %s\n' "$archive"
