#!/usr/bin/env bash
set -euo pipefail

binary=${1:?usage: smoke-release.sh BINARY VERSION}
version=${2:?usage: smoke-release.sh BINARY VERSION}
binary=$(cd "$(dirname "$binary")" && pwd)/$(basename "$binary")
directory=$(mktemp -d)
server_pid=
trap 'if [[ -n "$server_pid" ]]; then kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true; fi; rm -rf "$directory"' EXIT
cd "$directory"

test "$("$binary" version)" = "Sciurus $version"
printf 'CREATE TABLE task (id INT NOT NULL, PRIMARY KEY (id));\n' > before.sql
printf 'CREATE TABLE task (id INT NOT NULL, title VARCHAR(80) NULL, PRIMARY KEY (id));\n' > after.sql
"$binary" import --output diagram.er.json before.sql
"$binary" validate diagram.er.json
"$binary" generate --no-timestamp --output first.sql diagram.er.json
"$binary" generate --no-timestamp --output second.sql diagram.er.json
cmp first.sql second.sql
"$binary" diff --exit-code before.sql first.sql
"$binary" diff --format png --output diff.png before.sql after.sql
test -s diff.png

: > server.log
"$binary" serve --addr 127.0.0.1:0 diagram.er.json > server.log 2>&1 &
server_pid=$!
url=
for attempt in {1..100}; do
  if IFS= read -r line < server.log && [[ "$line" == 'Sciurus → http://'* ]]; then
    url=${line#Sciurus → }
    break
  fi
  if ! kill -0 "$server_pid" 2>/dev/null; then
    cat server.log >&2
    exit 1
  fi
  sleep 0.1
done
if [[ -z "$url" ]]; then
  cat server.log >&2
  echo 'Packaged server did not become ready.' >&2
  exit 1
fi

curl --fail --silent --show-error --max-time 10 "$url/" --output index.html
grep -q '<title>Sciurus</title>' index.html
curl --fail --silent --show-error --max-time 10 "$url/api/document" --output document.json
grep -Eq '"path"[[:space:]]*:[[:space:]]*"[^"]*diagram\.er\.json"' document.json
grep -oE '/assets/[^" ]+\.(js|css)' index.html > assets.txt
test -s assets.txt
while IFS= read -r asset; do
  curl --fail --silent --show-error --max-time 10 "$url$asset" --output asset
  test -s asset
done < assets.txt
curl --fail --silent --show-error --max-time 10 "$url/api/examples" --output examples.json
grep -q starter.json examples.json
curl --fail --silent --show-error --max-time 10 "$url/api/examples/starter.json" --output example.er.json
"$binary" validate example.er.json
printf 'Packaged binary smoke check passed for %s.\n' "$version"
