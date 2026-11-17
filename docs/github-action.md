# GitHub Action

Use Sciurus as a step in another repository's workflow to validate models,
review schema changes, or check that a model and SQL file agree. No database,
Docker, Go toolchain or browser tooling is needed.

The action lives at the repository root. Reference it as `mhemmings/sciurus@v0`,
or select a published version tag or commit. Supply the binary release separately
with the required `version` input, such as `v0.1.2`. The action downloads that
release's matching archive and verifies its SHA-256 checksum before extraction
or execution. The runner's existing Go toolchain is untouched.

If your repository requires full commit SHA pins, use the release's commit SHA
instead of `@v0`, and still supply `version: v0.1.2`. The action ref selects the
installer implementation; `version` selects the binary. Versions must use a
published stable `vMAJOR.MINOR.PATCH` tag; `latest`, branches and commit SHAs are
not binary-version inputs. The action has no nested third-party actions or
source-build fallback.

## Validate a model

Check the model's format, SQL syntax and references on pushes and pull requests:

```yaml
name: Schema

on: [push, pull_request]

permissions:
  contents: read

jobs:
  validate:
    runs-on: ubuntu-26.04
    steps:
      - uses: actions/checkout@v7
      - uses: mhemmings/sciurus@v0
        with:
          version: v0.1.2
          command: validate
          model: db/diagram.er.json
```

`validate` is the default command. Invalid models fail the step.

## Check model and SQL consistency

Compare the database definitions and fail when they differ. Diagram positions,
colours, IDs and other presentation details do not affect the comparison.

```yaml
- uses: mhemmings/sciurus@v0
  with:
    version: v0.1.2
    command: diff
    before: db/diagram.er.json
    after: db/schema.sql
    fail-on-changes: 'true'
```

Either input can be model JSON or supported MySQL `CREATE TABLE` SQL. This is a
schema comparison, not a byte-for-byte check of the SQL file.

## Review a pull request's schema changes

The action compares two files. Your workflow chooses which commits supply them.
This example compares the model from the pull request's base commit with the
checked-out version, then uploads a visual report only when the schema changes:

```yaml
name: Schema changes

on: pull_request

permissions:
  contents: read

jobs:
  diff:
    runs-on: ubuntu-26.04
    steps:
      - uses: actions/checkout@v7
        with:
          fetch-depth: 0

      - name: Read the base model
        env:
          BASE_SHA: ${{ github.event.pull_request.base.sha }}
        run: git show "${BASE_SHA}:db/diagram.er.json" > "$RUNNER_TEMP/before.er.json"

      - uses: mhemmings/sciurus@v0
        id: changes
        with:
          version: v0.1.2
          command: diff
          before: ${{ runner.temp }}/before.er.json
          after: db/diagram.er.json

      - uses: mhemmings/sciurus@v0
        if: steps.changes.outputs.changed == 'true'
        id: schema
        with:
          version: v0.1.2
          command: diff
          before: ${{ runner.temp }}/before.er.json
          after: db/diagram.er.json
          format: png

      - uses: actions/upload-artifact@v7
        if: ${{ always() && steps.schema.outputs.report != '' }}
        with:
          name: schema-diff
          path: ${{ steps.schema.outputs.report }}
```

The model must exist in the base commit for this example. For other comparisons,
provide any two model/SQL snapshots. The action does not fetch refs or choose a
merge base.

Diffs succeed when they find changes unless `fail-on-changes` is `true`. Input,
comparison and output errors always fail. Reports and outputs are available
before a consistency check fails, so the artifact step above also works with
`fail-on-changes: 'true'` on the PNG step.

## Installation and runner support

Supported targets are Linux amd64/arm64, macOS Intel/Apple Silicon, and Windows
amd64. The installer uses Bash (Git Bash on Windows), `curl`, `sha256sum` or
`shasum`, and `tar` for Linux/macOS or `unzip` for Windows. Windows also uses
Git Bash's `cygpath`. These tools are available on standard GitHub-hosted runners;
self-hosted runners must supply them.

Downloads are reused within a job for the same version and platform. Every
invocation rechecks the cached archive's checksum, extracts a fresh executable,
checks its reported version, and adds it to `PATH`. Download, checksum and
installation failures fail the action.

## Inputs

Paths are relative to the checked-out repository's workspace, or absolute.
Check out your repository before using the action. Each invocation accepts one
model or one pair of schemas; use additional steps or a matrix for more files.

| Input | Default | Purpose |
| --- | --- | --- |
| `version` | — | Required published binary release, such as `v0.1.2`. Independent of the action ref. |
| `command` | `validate` | `validate` or `diff`. |
| `model` | — | Required for validation; path to a model JSON file. |
| `before` | — | Required for diff; path to the earlier schema. |
| `after` | — | Required for diff; path to the later schema. |
| `format` | `text` | Diff report format: `text` or `png`. |
| `output` | Unique temporary file | Optional report path; parent directories are created. |
| `fail-on-changes` | `'false'` | Set to `'true'` to fail when a diff finds database changes. |

Text reports appear in the step's logs. PNG reports are written to a file; use
`actions/upload-artifact` to retain them. Both formats have the same `changed`
output. Use text for comparisons too large to render as a PNG.

## Outputs

| Output | Meaning |
| --- | --- |
| `changed` | `'true'` or `'false'` after a completed diff. |
| `report` | Absolute path to the completed text or PNG report. |

Both outputs are empty for validation or an unsuccessful comparison. A diff
that finds changes still publishes both outputs when `fail-on-changes` fails
the step. The installed `sciurus` executable also remains on `PATH` for later
steps in the same job.

The action uses [GitHub's composite action format](https://docs.github.com/en/actions/tutorials/create-actions/create-a-composite-action).
