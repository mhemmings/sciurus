# CI and releases

## Pull requests and main

Every pull request and push to `main` runs the same CI suite:

- Go vet and race tests, including schemadiff comparisons and live MySQL 8.4.9.
- Frontend formatting, types, unit tests, example layouts and a production build.
- A check that the committed frontend assets match that build.
- Playwright Chromium journeys, with failure diagnostics saved as artifacts.
- The reusable GitHub Action's validation, diff and failure behaviour.
- Release builds and archive checks for all supported platforms, with executable smoke checks on Linux.
- Verification of the GitHub Actions dependency lockfile against upstream commits.

The aggregate **CI checks** job succeeds only when every job passes. Configure it
as a required check on `main` in the GitHub repository's branch rules.

All jobs use Ubuntu 26.04 LTS. Linux arm64 builds use its arm64 runner so both
Linux archives can be executed. macOS and Windows binaries are cross-compiled
with `CGO_ENABLED=0` and checked without execution.

## Workflow files

| File | Purpose |
| --- | --- |
| [ci.yml](../.github/workflows/ci.yml) | Runs checks on PRs and `main`, and is called for releases. |
| [action-tests.yml](../.github/workflows/action-tests.yml) | Tests the reusable Sciurus action defined in the root `action.yml`. |
| [release-builds.yml](../.github/workflows/release-builds.yml) | Builds and checks archives for all five targets on Ubuntu. Called by CI for both PRs and releases. |
| [publish-release.yml](../.github/workflows/publish-release.yml) | Checks a pushed version tag, runs CI, then publishes the resulting archives. |
| [.github/release.yml](../.github/release.yml) | Configures the categories in GitHub's generated release notes. It does not run jobs. |

`release-builds.yml` and `action-tests.yml` use `workflow_call` so CI and releases
share the same checks. They have no manual `workflow_dispatch` trigger.
In action tests, `uses: $/` means the Sciurus action at the repository root,
at the workflow's own commit. Its remote dependencies are included in the lockfile.

## Action dependencies

[actions.lock](../.github/workflows/actions.lock) uses GitHub's
[workflow dependency lockfile](https://docs.github.com/en/actions/concepts/security/workflow-dependency-lockfiles)
to record the exact commits of workflow actions, including the Sciurus composite
action's dependencies. Generate it with GitHub's CLI extension; do not edit it by hand:

```sh
gh extension install github/gh-actions-lock --pin v0.4.1
gh actions-lock --no-narrow
gh actions-lock --verify
```

Run `gh actions-lock --no-narrow` after changing an action reference and commit
its changes together with the workflow. `--no-narrow` preserves the composite
action's full SHA pins, which consuming repositories may require independently
of this repository's lockfile. Dependabot checks Go modules, frontend dependencies
(including development tools), and GitHub Actions weekly. It updates the matching
dependency lockfiles in its PRs. CI verifies action coverage and upstream provenance.

Frontend dependency PRs also need `make frontend` and the resulting `web/dist`
changes committed before CI can pass. Versions pinned in shell commands, such as
the MySQL test image, Yarn and the lockfile CLI, require manual updates.

## Ship a release

Releases are triggered by stable version tags, giving you control over when and
what to ship. After merging your changes, tag a commit on `main`:

```sh
git switch main
git pull --ff-only
git tag -a v0.1.0 -m "Sciurus v0.1.0"
git push origin v0.1.0
```

Choose the version in the tag:

| Change | Example next release |
| --- | --- |
| Patch | `v0.1.0` → `v0.1.1` |
| Minor | `v0.1.1` → `v0.2.0` |
| Major | `v0.2.0` → `v1.0.0` |

No version file, PR label or release commit is needed. Source builds report
`dev`; versioned `go install` builds report their module version, and release
archives embed the tag's version.

The release workflow runs the full CI suite against the tagged commit, packages
and tests the binaries, and publishes those exact archives. Failed checks block
publication. Only the publication job has permission to write repository contents.

## Release contents

| Platform | Architecture | Archive |
| --- | --- | --- |
| Linux | amd64, arm64 | `.tar.gz` |
| macOS | amd64, arm64 | `.tar.gz` |
| Windows | amd64 | `.zip` |

For example, `sciurus_0.1.0_linux_amd64.tar.gz` contains the executable and license
files at its root. Every archive is extracted and checked for its contents,
binary target, embedded version string and disabled CGO. Linux executable smoke checks
run outside the source checkout and exercise the CLI, embedded editor assets
and bundled examples. macOS and Windows executables are not run in CI.

`checksums.txt` lists SHA-256 hashes for all five archives. The published release
includes generated notes listing merged PRs, contributors and a changelog link.
[Release-note categories](../.github/release.yml) group `breaking-change`,
`enhancement` and `bug` labels, with all other PRs included under Other changes.
Clear PR titles make the generated notes useful; labels only affect grouping.

Each release also updates its floating major action tag: `v0.1.0` updates `v0`,
while `v1.0.0` updates `v1`. Consumers can use `mhemmings/sciurus@v0`, or pin an
exact release tag or commit. Full version tags stay fixed.

## Reruns

Release runs queue rather than cancel one another. If publishing fails, rerun
the failed workflow: an unfinished draft resumes uploading before publication.
An already-published release is left intact. Publishing an older version does
not move GitHub's latest release or a major action tag backward, and a newer
unpublished tag does not hide a successfully published release.

## First GitHub push

Publish the repository with `main` as its default branch, including `web/dist`.
GitHub Actions must be enabled, and tag rules must allow the release workflow to
update floating major tags. The workflows use the automatic `GITHUB_TOKEN`;
no separate deployment secret is required.

After the first CI run, require **CI checks** in the branch rules. Push the first
version tag when ready to publish the initial release.
