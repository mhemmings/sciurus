# Testing guide

Sciurus tests three separate properties: the model preserves intent, generated
SQL is deterministic, and imported/generated schemas create equivalent MySQL
definitions. Browser tests cover the editor that produces those models.

This is a reproducible test guide, not a claim that every MySQL feature or
version is supported. The supported subset is listed in the
[README](../README.md#mysql-support).

## Local checks

From the repository root, with Go 1.26.11+, Node.js 24+ and Yarn Classic:

```sh
make frontend
make check
make test
```

`make frontend` installs locked JavaScript dependencies and rebuilds the embedded
production assets. `make check` checks frontend formatting/types, example layouts
and Go vet.
`make test` runs Go and frontend/layout unit tests. Unconfigured live database
tests explicitly skip; this does not count as running database equivalence tests.

For Go changes, format and reconcile module dependencies before testing:

```sh
go fmt ./...
go fix ./...
go mod tidy
```

For browser journeys, install Playwright Chromium once, then run the production
build through the Go server:

```sh
(cd web && yarn playwright install chromium)
make test-e2e
```

The suite starts its own server on `127.0.0.1:17480`. That port must be available.
Browser failures retain a Playwright trace in `web/test-results`.

## GitHub Action checks

Run the action's command handling against a freshly installed CLI:

```sh
bash scripts/test-action.sh
```

These checks cover model validation, mixed model/SQL comparisons, changed and
unchanged schemas, failure outputs, PNG reports and quoted file paths. They
require Go, without a database or frontend tools. The GitHub Action workflow
also exercises the composite action itself.

## Schema equivalence and live MySQL

Normal Go tests compare SQL fixtures with imported, saved and generated schemas
using the embedded Vitess **schemadiff** library. These checks need no database
or additional executable.

For independent server checks, start Docker and run:

```sh
make test-integration
```

The test runner starts a disposable MySQL 8.4.9 container. Tests create uniquely
named temporary databases and remove them afterward; the runner removes its
container when finished. Required acceptance checks fail if MySQL is unavailable
rather than reporting a successful run with those checks skipped.

You can instead supply an existing **disposable test server**:

```sh
DB_ADDR=127.0.0.1:3306 DB_USER=tester DB_PASS=local-test-password \
  make test-integration
```

The account needs permission to create and drop the temporary databases, create
their objects, and read their definitions. The tests do not need application
data. Use a dedicated local test account.

A direct `go test ./...` needs no database. Setting `DB_ADDR` enables the live
comparison; `DB_USER` and `DB_PASS` select its credentials. `REQUIRE_MYSQL=1`
requires database comparison and cannot be used with `-short` to skip acceptance
checks. Schemadiff comparisons also run in short mode.

### What is compared

For each independent SQL fixture, the suite:

1. Imports its SQL into the typed model, encodes and decodes JSON, and generates
   SQL from the decoded model.
2. Parses independently supplied fixture definitions and generated SQL directly
   with schemadiff, omitting session `SET` statements, and compares them in both
   directions. Constraint names and AUTO_INCREMENT values must match; both
   comparisons must report no changes.
3. When MySQL is configured, creates separate source and generated databases
   using the original SQL and the generated SQL, respectively.
4. Compares the database character set/collation and the complete ordered
   `SHOW CREATE TABLE` output exactly.

Schemadiff provides comparison logic separate from Sciurus's normalizer, but
shares its Vitess SQL parser. The live MySQL check supplies an independent
authority and also checks index order, which schemadiff normally ignores. See
the [design notes](research.md#acceptance-criteria) for the comparison boundaries.

Two fixtures supply an equivalent comparison definition because schemadiff does
not handle quoted charset names or inline `KEY`/`UNIQUE` declarations correctly.
Their original SQL still exercises import and the live MySQL check. Workbench
import also compares its generated SQL directly with an independent SQL fixture.

References:

- [Vitess schemadiff API](https://pkg.go.dev/vitess.io/vitess/go/vt/schemadiff)
- [MySQL SHOW CREATE TABLE](https://dev.mysql.com/doc/refman/8.4/en/show-create-table.html)

## Coverage

The generic example and synthetic feature fixtures cover ordinary application
schemas plus composite/circular foreign keys, self references, expression and
temporal defaults, generated columns, CHECKs, ENUM/SET/BIT values, quoted names,
charset inheritance, prefixes, descending indexes, invisible columns/indexes,
and FULLTEXT/SPATIAL indexes. Reproducible seeded schemas add variation in table
creation order, constraints and column attributes without sharing SQL rendering
code with the implementation under test.

Model checks cover repeated generation, JSON save/load cycles, shuffled table
order and changed layout. Strict-decoding cases include unknown, duplicate or
case-mismatched fields, missing fields, nulls, invalid Unicode, broken references,
incompatible foreign keys and unsupported SQL attributes.

Synthetic Workbench archives exercise direct schema import, visible groups,
layer-relative positions, root layers and model-only relationships. Applying a
layout to an SQL-derived model must preserve SQL exactly. Invalid layout archives
must leave the original model unchanged.

CLI and HTTP tests cover output destinations, input/output separation, malformed
input, atomic saves, external-file conflicts, concurrent tabs and the
loopback/origin boundary.

The local Playwright suite covers:

- Schema editing, SQL preview/export, JSON validation, save/reopen and file conflicts.
- Click-to-place tables/groups, automatic membership, group movement and saved geometry.
- All three relationship tools, foreign-key editing and preservation of viewport/sidebar state.
- Right-angle route editing, undo and redo.
- Column sorting with handles and ghost rows, double-click editing, renaming,
  primary-key controls, advanced settings and type input.
- Resizable/closable sidebars and the bundled example models, including rendered
  table bounds and full identifier widths.

## Example layout checks

Example geometry has an explicit generation step:

```sh
cd web
yarn examples:layout
yarn examples:check
yarn build
```

The script sizes cards for their contents, packs tables inside groups, and packs
groups without intersections. It preserves schema fields and IDs while updating
positions, widths, group bounds/colors, routes and viewport. Tests check that
repeating layout is stable and that geometry does not change SQL semantics.
Normal application loads do not invoke this script.

## Packaged binary smoke check

After `make build`, copy `bin/sciurus` into an otherwise empty directory and run
it there. The editor and example picker should work without `web/`, `examples/`
or a JavaScript process beside the binary. Check headless generation twice:

```sh
sciurus validate diagram.er.json
sciurus generate --no-timestamp diagram.er.json > first.sql
sciurus generate --no-timestamp diagram.er.json > second.sql
cmp first.sql second.sql
```

Also compare before and after a layout-only edit. Both comparisons must be
byte-identical. When retaining timestamps, remove only the generated-at comment
for comparison; normalizing additional SQL could hide an ordering regression.

The [CI and release workflows](releases.md) run on Ubuntu. Both Linux archives
are extracted and executed on matching Ubuntu architectures. macOS and Windows
binaries are cross-compiled with `CGO_ENABLED=0`; their archives, target metadata
and embedded version strings are checked without executing them. CI does not
establish runtime behavior on macOS or Windows.

Build and check a native archive locally with:

```sh
bash scripts/build-release.sh dev /tmp/sciurus-release
```

For a host-matching target, the check exercises the extracted executable's
version, import, validation, deterministic generation, text/PNG diffs, HTTP
assets and embedded examples. Set `GOOS` and `GOARCH` to build another target:

```sh
GOOS=windows GOARCH=amd64 bash scripts/build-release.sh dev /tmp/sciurus-release
```

Foreign targets receive static archive and binary checks. Publication behaviour
can be checked against a temporary Git repository and a simulated GitHub CLI:

```sh
bash scripts/test-release.sh
```
