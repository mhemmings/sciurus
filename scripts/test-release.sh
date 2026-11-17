#!/usr/bin/env bash
set -euo pipefail

project_directory=$(cd "$(dirname "$0")/.." && pwd)
test_directory=$(mktemp -d)
trap 'rm -rf "$test_directory"' EXIT
trap 'printf "Release checks failed at line %s\n" "$LINENO" >&2' ERR
repository="$test_directory/source repository"
origin="$test_directory/origin.git"
assets="$test_directory/release assets"
export FAKE_GITHUB="$test_directory/github"
mkdir -p "$repository" "$assets" "$test_directory/bin" "$FAKE_GITHUB/releases"
export PATH="$test_directory/bin:$PATH:/sbin"
export GITHUB_REPOSITORY=example/sciurus
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_TERMINAL_PROMPT=0

# Git pushes stay inside the temporary directory; every GitHub call uses this stub.
cat > "$test_directory/bin/gh" <<'BASH'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$FAKE_GITHUB/calls"
case "$1 $2" in
  'api --paginate')
    for release in "$FAKE_GITHUB/releases/"*; do
      [[ -f "$release" ]] || continue
      if [[ $(cat "$release") == false ]]; then basename "$release"; fi
    done
    ;;
  'release view')
    [[ -f "$FAKE_GITHUB/releases/$3" ]] || exit 1
    cat "$FAKE_GITHUB/releases/$3"
    ;;
  'release create'|'release upload'|'release edit')
    printf '%s\n' "$@" > "$FAKE_GITHUB/$2"
    release="$FAKE_GITHUB/releases/$3"
    if [[ $2 == create ]]; then
      [[ ! -f "$release" ]]
      printf 'true\n' > "$release"
    else
      [[ -f "$release" ]]
    fi
    if [[ $2 == edit ]]; then
      for argument in "$@"; do
        case "$argument" in
          --draft=false) printf 'false\n' > "$release" ;;
          --latest=true) printf '%s\n' "$3" > "$FAKE_GITHUB/latest" ;;
        esac
      done
    fi
    ;;
  *) printf 'Unexpected gh invocation: %s\n' "$*" >&2; exit 1 ;;
esac
BASH
chmod +x "$test_directory/bin/gh"

git init --bare "$origin" >/dev/null 2>&1
cd "$repository"
git init -b main >/dev/null
git config user.name 'Release tests'
git config user.email release-tests@example.invalid
git config commit.gpgsign false
git remote add origin "$origin"
printf 'initial\n' > change.txt
git add change.txt
git commit -qm initial
initial_commit=$(git rev-parse HEAD)
git tag v0.1.0
git push -q origin main refs/tags/v0.1.0

for target in darwin_amd64.tar.gz darwin_arm64.tar.gz linux_amd64.tar.gz linux_arm64.tar.gz windows_amd64.zip; do
  printf '%s\n' "$target" > "$assets/$target"
done

publish() {
  local expected=$1 version=$2 status=0
  rm -f "$FAKE_GITHUB/create" "$FAKE_GITHUB/upload" "$FAKE_GITHUB/edit"
  : > "$FAKE_GITHUB/calls"
  bash "$project_directory/scripts/publish-release.sh" "$version" "$(git rev-parse HEAD)" "$assets" \
    > "$test_directory/log" 2>&1 || status=$?
  if [[ ($expected == success && $status != 0) || ($expected == failure && $status == 0) ]]; then
    cat "$test_directory/log" >&2
    printf 'Expected %s for %s, received exit %s\n' "$expected" "$version" "$status" >&2
    exit 1
  fi
}

assert_uploaded_assets() {
  local command=$1
  for asset in "$assets/"*; do
    grep -Fqx -- "$asset" "$FAKE_GITHUB/$command"
  done
}

# First publication includes all archives, valid checksums, and the moving major tag.
publish success v0.1.0
[[ $(cat "$FAKE_GITHUB/releases/v0.1.0") == false ]]
[[ $(cat "$FAKE_GITHUB/latest") == v0.1.0 ]]
assert_uploaded_assets create
grep -qx -- --verify-tag "$FAKE_GITHUB/create"
grep -qx -- --generate-notes "$FAKE_GITHUB/create"
grep -qx -- --draft "$FAKE_GITHUB/create"
[[ $(sed -n '/^--target$/{n;p;}' "$FAKE_GITHUB/create") == "$initial_commit" ]]
[[ $(wc -l < "$assets/checksums.txt" | tr -d ' ') == 5 ]]
(cd "$assets" && sha256sum --check checksums.txt >/dev/null)
[[ $(git --git-dir="$origin" rev-parse refs/tags/v0.1.0) == "$initial_commit" ]]
[[ $(git --git-dir="$origin" rev-parse refs/tags/v0) == "$initial_commit" ]]

# Published assets remain immutable when a workflow is rerun.
publish success v0.1.0
[[ ! -e "$FAKE_GITHUB/create" && ! -e "$FAKE_GITHUB/upload" && ! -e "$FAKE_GITHUB/edit" ]]
[[ $(git --git-dir="$origin" rev-parse refs/tags/v0) == "$initial_commit" ]]

# A draft can recover even when a future version has been tagged but not published.
printf 'next release\n' > change.txt
git commit -qam next
release_commit=$(git rev-parse HEAD)
git tag v0.2.0
printf 'future release\n' > change.txt
git commit -qam future
git tag v0.9.0
git push -q origin refs/tags/v0.2.0 refs/tags/v0.9.0
git checkout -q --detach "$release_commit"
printf 'true\n' > "$FAKE_GITHUB/releases/v0.2.0"
printf 'false\n' > "$FAKE_GITHUB/releases/nightly"
publish success v0.2.0
[[ ! -e "$FAKE_GITHUB/create" ]]
assert_uploaded_assets upload
grep -qx -- --clobber "$FAKE_GITHUB/upload"
[[ $(cat "$FAKE_GITHUB/releases/v0.2.0") == false ]]
[[ $(cat "$FAKE_GITHUB/latest") == v0.2.0 ]]
[[ $(git --git-dir="$origin" rev-parse refs/tags/v0) == "$release_commit" ]]

# Publishing an older version cannot move latest or the major tag backwards.
git tag v0.1.1 "$initial_commit"
git push -q origin refs/tags/v0.1.1
git checkout -q --detach "$initial_commit"
publish success v0.1.1
[[ $(cat "$FAKE_GITHUB/releases/v0.1.1") == false ]]
grep -qx -- --latest=false "$FAKE_GITHUB/edit"
[[ $(cat "$FAKE_GITHUB/latest") == v0.2.0 ]]
[[ $(git --git-dir="$origin" rev-parse refs/tags/v0) == "$release_commit" ]]

# A release requires an existing version tag that points at the build commit.
git checkout -q --detach "$release_commit"
for version in v0.1.0 v0.3.0; do
  publish failure "$version"
  [[ ! -s "$FAKE_GITHUB/calls" ]]
done
[[ $(git --git-dir="$origin" rev-parse refs/tags/v0.1.0) == "$initial_commit" ]]
! git --git-dir="$origin" show-ref --verify --quiet refs/tags/v0.3.0
[[ $(git --git-dir="$origin" rev-parse refs/tags/v0) == "$release_commit" ]]

echo 'Release publishing checks passed.'
