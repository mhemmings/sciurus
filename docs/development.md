# Development

The Go module is `github.com/mhemmings/sciurus`. Go 1.26.11+ is required.
To install from a checkout:

```sh
go install ./cmd/sciurus
```

This includes the committed, prebuilt frontend in the executable. Rebuild those
assets after changing frontend source, before installing or building Go again.
Frontend work needs **Node.js 24+** and **Yarn Classic 1.22**. Dependencies are
pinned in `go.mod`, `go.sum` and `web/yarn.lock`.

```sh
make frontend       # Install locked frontend dependencies and rebuild web/dist.
make build          # Build the self-contained binary into bin/sciurus.
```

For frontend hot reload, run:

```sh
make dev
```

This builds the Go binary, starts its HTTP API on `127.0.0.1:7480`, and starts
Vite on `127.0.0.1:5173`. Open Vite's printed URL; it proxies `/api` to the Go
server. Ctrl+C stops both processes. Install frontend dependencies first with
`make frontend`.

To bind a model during development, run the processes separately in two terminals:

```sh
# Terminal 1
go run ./cmd/sciurus serve --addr 127.0.0.1:7480 diagram.er.json

# Terminal 2
yarn --cwd web dev
```

Production assets live in `web/dist` and must be regenerated and included with
frontend changes; the Go binary embeds those assets at build time.

## Checks

```sh
make check              # Go vet, frontend formatting/types and example layout checks.
make test               # Go tests plus frontend and layout unit tests.
make test-e2e           # Rebuild assets and run local Playwright journeys.
make test-integration   # Schema equivalence with schemadiff and live MySQL.
```

Install Chromium once before browser tests:

```sh
cd web
yarn playwright install chromium
```

Integration tests need Docker or a supplied MySQL test server; see
[the testing guide](testing.md) for setup. Normal Go tests include schemadiff
comparisons without a database or additional executable. Use `go fmt ./...`,
`go fix ./...` and `go mod tidy` after Go changes; use `yarn format` from `web`
after frontend changes.

To regenerate bundled example geometry after changing its schema or membership:

```sh
cd web
yarn examples:layout
yarn examples:check
yarn build
```

The script updates geometry while preserving schema fields and stable IDs.
Review the resulting models along with the rebuilt assets. `make clean` removes
the compiled binary.

## Repository layout

```text
cmd/sciurus/            CLI entry point
internal/app/           Commands, local HTTP API and atomic file handling
internal/model/         Typed model, strict validation, SQL and Workbench import
internal/testing/       Schemadiff comparisons and independent MySQL checks
web/src/                Vue editor and TypeScript model helpers
web/e2e/                Browser interaction tests
web/scripts/            Example layout generation and checks
web/dist/               Embedded production frontend
examples/               Embedded sample models and standalone SQL fixtures
docs/                   Format contract, design decisions and testing guide
model.schema.json       JSON Schema authoring contract
```

The Go model is authoritative for validation and SQL generation. The Vue editor
uses the same serialized contract through the local HTTP API. There is no
database service or separate frontend process in the distributed application.

## Releases

PRs and `main` run the full CI suite. Pushing a version tag such as `v0.1.0`
publishes tested binaries and generated release notes. See [CI and releases](releases.md)
for versioning, supported platforms and first-time repository setup.
