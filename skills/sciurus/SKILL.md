---
name: sciurus
description: Use Sciurus to edit MySQL ER models, import or export schemas, validate models, and compare schema changes.
---

# Sciurus

Sciurus stores a MySQL schema and its ER diagram in one JSON file. Work directly
with that file and use the CLI to validate and compare changes. SQL is exported,
never executed; Sciurus does not generate database migrations.

## Find the model and CLI

Locate the repository's model and SQL files, and follow its source-of-truth and
naming conventions. Examples below use `diagram.er.json` and `schema.sql`;
substitute the actual paths. Prefer the repository's pinned Sciurus version.
Pass input filenames positionally and place options before the filenames.

Use `sciurus version` and `sciurus --help` to check the available CLI. If it is
missing, follow the repository's installation instructions. Install a published
version with Go:

```sh
go install github.com/mhemmings/sciurus/cmd/sciurus@latest
```

Replace `latest` with the pinned version when provided. From a Sciurus source
checkout, use `go install ./cmd/sciurus` instead. Prebuilt executables are also
available from [GitHub Releases](https://github.com/mhemmings/sciurus/releases).
Both methods include the complete editor and examples; Node.js, Yarn and a
frontend build are unnecessary for installation. Rebuild frontend assets only
when changing frontend source in a Sciurus checkout.

## Edit a model

Before editing, save a temporary copy of the model for the before/after diff.
Use existing objects as templates and preserve these contracts:

- Keep existing IDs when renaming objects. Indexes and foreign keys reference
  IDs; generated-column and CHECK expressions reference SQL column names.
- Update dependent indexes, foreign keys and expressions when changing or
  deleting referenced objects.
- Preserve column order, composite-key order and ENUM/SET value order unless the
  requested change requires otherwise.
- Keep schema-only edits separate from positions, widths, groups, routes and
  viewport. Table coordinates are absolute; moving a group in JSON requires
  translating its member tables too.
- Use supported fields and required empty arrays (`[]`). Unknown fields,
  duplicate keys and JSON comments are rejected.

Column attributes are separate from the SQL type. For example, a new column is:

```json
{"id":"status","name":"status","type":"VARCHAR(20)","nullable":false,"default":"'draft'"}
```

Defaults contain SQL expressions: `"'draft'"` is a SQL string literal and
`"NULL"` is SQL NULL. Omit `default` for no explicit default. Use IDs unique in
the same scope as the existing model.

After editing, validate the model and review the database changes:

```sh
sciurus validate diagram.er.json
sciurus diff before.er.json diagram.er.json
```

Here, `before.er.json` is the saved copy. For a layout-only change, add
`--exit-code` to the diff; it must return 0. Regenerate SQL when the repository's
workflow keeps an exported SQL file alongside the model.

## Common commands

Import a MySQL Workbench model directly into a new ER model:

```sh
sciurus import --output diagram.er.json diagram.mwb
```

Input extensions are case-insensitive: `.mwb` selects Workbench import and `.sql`
selects SQL schema import.
Workbench inputs support one MySQL schema and zero or one diagram; without a
diagram, tables receive an initial grid layout. Model-only and commented-out
objects are excluded from the physical schema. Unsupported schema constructs
fail explicitly.

For schema-only MySQL `CREATE TABLE` SQL, use:

```sh
sciurus import --output diagram.er.json schema.sql
```

With a SQL input, optionally add `--workbench diagram.mwb` to take layout from
exactly one diagram while SQL supplies the schema. Full database dumps, data and
migration scripts are not supported SQL imports.

Export reproducible SQL, without the generated-at timestamp:

```sh
sciurus generate --no-timestamp --output schema.sql diagram.er.json
```

The SQL creates tables in an empty, already-created database. Offline validation
checks syntax and model consistency; MySQL determines server-specific validity.

Compare any pairing of model JSON and supported SQL, or export a visual report:

```sh
sciurus diff before.er.json after.sql
sciurus diff --format png --output schema-diff.png before.er.json after.er.json
sciurus diff --exit-code diagram.er.json schema.sql
```

Diffs ignore diagram layout; renames appear as removal/addition. With
`--exit-code`, 0 means equal, 1 means different and 2 means an error. Without it,
a successful diff returns 0 even when schemas differ. Use text output for changes
too large for PNG rendering.

Open the visual editor when requested:

```sh
sciurus diagram.er.json
```

Save in the editor to persist changes. An externally changed file must be
reopened before saving again; Save As can preserve conflicting edits separately.
