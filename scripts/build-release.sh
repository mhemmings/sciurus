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
  (cd "$stage" && 7z a -tzip -bso0 -bsp0 "$archive" "$executable" LICENSE FONT-LICENSE.txt)
  mkdir "$stage/extracted"
  7z x -bso0 -bsp0 "-o$stage/extracted" "$archive"
else
  archive="$archive.tar.gz"
  tar -czf "$archive" -C "$stage" "$executable" LICENSE FONT-LICENSE.txt
  mkdir "$stage/extracted"
  tar -xzf "$archive" -C "$stage/extracted"
fi

bash scripts/smoke-release.sh "$stage/extracted/$executable" "$version"
printf 'Packaged %s\n' "$archive"
