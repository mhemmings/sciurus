#!/usr/bin/env bash
set -euo pipefail

version=${1:?usage: publish-release.sh VERSION COMMIT ASSET_DIRECTORY}
commit=${2:?usage: publish-release.sh VERSION COMMIT ASSET_DIRECTORY}
asset_directory=${3:?usage: publish-release.sh VERSION COMMIT ASSET_DIRECTORY}
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'Release version must be vMAJOR.MINOR.PATCH.' >&2
  exit 1
fi
commit=$(git rev-parse --verify "$commit^{commit}")
test "$(git rev-parse HEAD)" = "$commit"

shopt -s nullglob
archives=("$asset_directory"/*.tar.gz "$asset_directory"/*.zip)
if [[ ${#archives[@]} != 5 ]]; then
  echo 'Expected all five native release archives.' >&2
  exit 1
fi
(
  cd "$asset_directory"
  sha256sum *.tar.gz *.zip > checksums.txt
)

test "$(git rev-parse --verify "refs/tags/$version^{commit}")" = "$commit"

# Unpublished tags may still be building or may have failed their release checks.
published=$(mktemp)
trap 'rm -f "$published"' EXIT
gh api --paginate "repos/$GITHUB_REPOSITORY/releases" \
  --jq '.[] | select(.draft == false and .prerelease == false) | .tag_name' > "$published"
printf '%s\n' "$version" >> "$published"
latest=$(awk '/^v[0-9]+\.[0-9]+\.[0-9]+$/' "$published" | sort -V | tail -n 1)
is_latest=false
if [[ "$version" == "$latest" ]]; then is_latest=true; fi

if state=$(gh release view "$version" --repo "$GITHUB_REPOSITORY" --json isDraft --jq .isDraft); then
  if [[ "$state" == true ]]; then
    gh release upload "$version" "${archives[@]}" "$asset_directory/checksums.txt" --repo "$GITHUB_REPOSITORY" --clobber
    gh release edit "$version" --repo "$GITHUB_REPOSITORY" --draft=false "--latest=$is_latest"
  else
    printf 'Release %s is already published.\n' "$version"
  fi
else
  gh release create "$version" "${archives[@]}" "$asset_directory/checksums.txt" \
    --repo "$GITHUB_REPOSITORY" --verify-tag --target "$commit" \
    --title "$version" --generate-notes --draft
  gh release edit "$version" --repo "$GITHUB_REPOSITORY" --draft=false "--latest=$is_latest"
fi

major=${version%.*.*}
latest_in_major=$(grep -E "^${major}\.[0-9]+\.[0-9]+$" "$published" | sort -V | tail -n 1)
if [[ "$version" == "$latest_in_major" ]]; then
  git tag --force "$major" "$commit"
  git push --force origin "refs/tags/$major"
fi
