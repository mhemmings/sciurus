# GitHub Action

Use Sciurus as a step in another repository's workflow to validate models,
review schema changes, or check that a model and SQL file agree. No database,
Docker or browser tooling is needed.

The action lives at the repository root. After the first release, reference it
as `mhemmings/sciurus@v0`, or select a published version tag or commit.
It installs the Go version Sciurus needs and builds the CLI from that exact
action revision. Your repository does not need to be a Go project.

If your repository requires full commit SHA pins, use the release's commit SHA
instead of `@v0`. Dependencies inside the Sciurus action also use full SHA pins.

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
checked-out version, then uploads a visual report:

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
        id: schema
        with:
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
`fail-on-changes: 'true'`.

## Inputs

Paths are relative to the checked-out repository's workspace, or absolute.
Check out your repository before using the action. Each invocation accepts one
model or one pair of schemas; use additional steps or a matrix for more files.

| Input | Default | Purpose |
| --- | --- | --- |
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
steps in the same job. The action's Go toolchain remains selected too; if your
project needs another Go version, configure it after this step or in a separate
job.

The action uses [GitHub's composite action format](https://docs.github.com/en/actions/tutorials/create-actions/create-a-composite-action)
and [setup-go](https://github.com/actions/setup-go) to manage its Go toolchain.
