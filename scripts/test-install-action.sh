#!/usr/bin/env bash
set -euo pipefail

project_directory=$(cd "$(dirname "$0")/.." && pwd)
test_directory=$(mktemp -d)
trap 'rm -rf "$test_directory"' EXIT
trap 'printf "Installer checks failed at line %s\n" "$LINENO" >&2' ERR
export REAL_TAR REAL_UNZIP
REAL_TAR=$(command -v tar)
REAL_UNZIP=$(command -v unzip)
real_zip=$(command -v zip)
mkdir "$test_directory/bin"

# Downloads stay inside the fixture directory; archive tools record extraction.
cat > "$test_directory/bin/curl" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
output=
url=
while [[ $# -gt 0 ]]; do
  case "$1" in
    --output|-o) output=$2; shift 2 ;;
    *) url=$1; shift ;;
  esac
done
printf '%s\n' "$url" >> "$FAKE_DOWNLOAD_LOG"
[[ "$url" == https://github.com/mhemmings/sciurus/releases/download/v0.1.1/* ]]
name=${url##*/}
if [[ "$name" == "${FAKE_DOWNLOAD_FAILURE:-}" ]]; then exit 22; fi
cp "$FAKE_RELEASE/$name" "$output"
BASH
cat > "$test_directory/bin/tar" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
printf 'tar\n' >> "$FAKE_EXTRACT_LOG"
exec "$REAL_TAR" "$@"
BASH
cat > "$test_directory/bin/unzip" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
printf 'unzip\n' >> "$FAKE_EXTRACT_LOG"
exec "$REAL_UNZIP" "$@"
BASH
cat > "$test_directory/bin/cygpath" <<'BASH'
#!/usr/bin/env bash
printf '%s\n' "$2"
BASH
cat > "$test_directory/bin/go" <<'BASH'
#!/usr/bin/env bash
printf 'go\n' >> "$FAKE_GO_LOG"
exit 99
BASH
chmod +x "$test_directory/bin/"*
export PATH="$test_directory/bin:$PATH"

new_case() {
  case_directory="$test_directory/$1"
  export FAKE_RELEASE="$case_directory/release" RUNNER_TEMP="$case_directory/runner temp"
  export GITHUB_PATH="$case_directory/github path" FAKE_DOWNLOAD_LOG="$case_directory/downloads"
  export FAKE_EXTRACT_LOG="$case_directory/extractions" FAKE_EXECUTION_LOG="$case_directory/executions"
  export FAKE_GO_LOG="$case_directory/go calls" FAKE_DOWNLOAD_FAILURE=
  export SCIURUS_VERSION=v0.1.1 RUNNER_OS=Linux RUNNER_ARCH=X64
  mkdir -p "$FAKE_RELEASE" "$RUNNER_TEMP"
  : > "$FAKE_DOWNLOAD_LOG"
}

make_archive() {
  local binary_version=${1:-v0.1.1} mode=${2:-binary}
  local stage="$case_directory/source" binary=sciurus
  if [[ "$asset" == *.zip ]]; then binary=sciurus.exe; fi
  mkdir "$stage"
  printf 'Fixture license\n' > "$stage/LICENSE"
  if [[ "$mode" == binary ]]; then
    cat > "$stage/$binary" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$0" >> "$FAKE_EXECUTION_LOG"
test "$1" = version
BASH
    printf '%s\n' "printf '%s\\n' 'Sciurus $binary_version'" >> "$stage/$binary"
    chmod +x "$stage/$binary"
  fi
  if [[ "$asset" == *.zip ]]; then
    (cd "$stage" && "$real_zip" -q "$FAKE_RELEASE/$asset" ./*)
  else
    "$REAL_TAR" -czf "$FAKE_RELEASE/$asset" -C "$stage" .
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    digest=$(sha256sum < "$FAKE_RELEASE/$asset")
  else
    digest=$(shasum -a 256 < "$FAKE_RELEASE/$asset")
  fi
  digest=${digest%% *}
  # A release manifest also lists targets that are not downloaded for this runner.
  printf '%064d  other-target.tar.gz\n%s  %s\n' 0 "$digest" "$asset" > "$FAKE_RELEASE/checksums.txt"
}

install() {
  local expected=$1 status=0
  : > "$GITHUB_PATH"
  : > "$FAKE_EXTRACT_LOG"
  : > "$FAKE_EXECUTION_LOG"
  : > "$FAKE_GO_LOG"
  bash "$project_directory/scripts/install-action.sh" > "$case_directory/log" 2>&1 || status=$?
  if [[ ($expected == success && $status != 0) || ($expected == failure && $status == 0) ]]; then
    cat "$case_directory/log" >&2
    printf 'Expected %s, received exit %s\n' "$expected" "$status" >&2
    exit 1
  fi
  if [[ -s "$FAKE_GO_LOG" ]]; then
    echo 'The installer must not invoke the Go toolchain.' >&2
    exit 1
  fi
}

assert_uninstalled() {
  [[ ! -s "$GITHUB_PATH" && ! -s "$FAKE_EXTRACT_LOG" && ! -s "$FAKE_EXECUTION_LOG" ]]
}

while read -r runner_os runner_arch asset; do
  new_case "$runner_os-$runner_arch"
  export RUNNER_OS=$runner_os RUNNER_ARCH=$runner_arch
  make_archive
  install success
  grep -Fqx "https://github.com/mhemmings/sciurus/releases/download/v0.1.1/$asset" "$FAKE_DOWNLOAD_LOG"
  IFS= read -r installed_directory < "$GITHUB_PATH"
  executable=sciurus
  if [[ "$RUNNER_OS" == Windows ]]; then executable=sciurus.exe; fi
  [[ -x "$installed_directory/$executable" && -s "$FAKE_EXTRACT_LOG" && -s "$FAKE_EXECUTION_LOG" ]]

  if [[ "$RUNNER_OS/$RUNNER_ARCH" == Linux/X64 ]]; then
    calls=$(wc -l < "$FAKE_DOWNLOAD_LOG")
    install success
    [[ -s "$FAKE_EXTRACT_LOG" ]]
    [[ $(wc -l < "$FAKE_DOWNLOAD_LOG") == "$calls" ]]
    cached_archive=$(find "$RUNNER_TEMP" -type f -name "$asset")
    [[ -f "$cached_archive" ]]
    printf 'corrupted\n' >> "$cached_archive"
    install failure
    assert_uninstalled
  fi
done <<'TARGETS'
Linux X64 sciurus_0.1.1_linux_amd64.tar.gz
Linux ARM64 sciurus_0.1.1_linux_arm64.tar.gz
macOS X64 sciurus_0.1.1_darwin_amd64.tar.gz
macOS ARM64 sciurus_0.1.1_darwin_arm64.tar.gz
Windows X64 sciurus_0.1.1_windows_amd64.zip
TARGETS

for invalid in wrong missing duplicate malformed; do
  new_case "$invalid checksum"
  asset=sciurus_0.1.1_linux_amd64.tar.gz
  make_archive
  case "$invalid" in
    wrong) printf '%064d  %s\n' 0 "$asset" > "$FAKE_RELEASE/checksums.txt" ;;
    missing) printf '%s  other.tar.gz\n' "$digest" > "$FAKE_RELEASE/checksums.txt" ;;
    duplicate) printf '%s  %s\n%s  %s\n' "$digest" "$asset" "$digest" "$asset" > "$FAKE_RELEASE/checksums.txt" ;;
    malformed) printf 'not-a-checksum  %s\n' "$asset" > "$FAKE_RELEASE/checksums.txt" ;;
  esac
  install failure
  assert_uninstalled
done

new_case 'download failure'
asset=sciurus_0.1.1_linux_amd64.tar.gz
make_archive
export FAKE_DOWNLOAD_FAILURE=$asset
install failure
[[ -s "$FAKE_DOWNLOAD_LOG" ]]
assert_uninstalled

for version in '' latest v01.1.1 v0.1.1-rc.1 '../v0.1.1'; do
  new_case "invalid version ${version//\//_}"
  export SCIURUS_VERSION=$version
  install failure
  [[ ! -s "$FAKE_DOWNLOAD_LOG" ]]
  assert_uninstalled
done

new_case 'unsupported target'
export RUNNER_OS=Windows RUNNER_ARCH=ARM64
install failure
[[ ! -s "$FAKE_DOWNLOAD_LOG" ]]
assert_uninstalled

for invalid in missing-executable wrong-version; do
  new_case "$invalid"
  asset=sciurus_0.1.1_linux_amd64.tar.gz
  if [[ "$invalid" == missing-executable ]]; then make_archive v0.1.1 missing; else make_archive v9.9.9; fi
  install failure
  [[ -s "$FAKE_EXTRACT_LOG" && ! -s "$GITHUB_PATH" ]]
  if [[ "$invalid" == missing-executable ]]; then [[ ! -s "$FAKE_EXECUTION_LOG" ]]; else [[ -s "$FAKE_EXECUTION_LOG" ]]; fi
done

echo 'Prebuilt installer checks passed.'
