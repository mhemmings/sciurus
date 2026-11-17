# Format and library decisions

Sciurus uses one versioned JSON document for a physical MySQL schema and its
complete diagram. SQL generation and layout are independent: arranging a diagram
must not change its generated schema, and generating SQL must not move tables.

## Single-file format

| Candidate | Useful properties | Cost for this application |
| --- | --- | --- |
| JSON | Native Go and browser support; explicit scalar types; familiar to agents; JSON Schema; ordered arrays | Verbose and lacks comments; explanatory text needs explicit fields |
| YAML | Compact, readable and supports comments | Aliases, scalar interpretation and indentation add complexity; preserving comments requires syntax-tree editing |
| TOML | Clear primitive values and comments | Deep arrays of columns, index parts, foreign keys and layout become cumbersome; no null value |
| DBML | Concise schema notation, relationships and table groups | MySQL-specific attributes and exact connector/position persistence need conventions or extensions; another parser is required |
| SQL with layout comments | Familiar physical schema syntax | Renames make schema/layout identity difficult; SQL tooling may discard comments; form editing still requires a structured model |
| Custom language | Control over syntax and readability | Requires a grammar, escaping rules, formatter, diagnostics, migrations, editor tooling and long-term maintenance |

The project defines its own **data model**, not a new serialization language.
Strict JSON provides predictable machine editing and portable validation without
introducing another grammar. Schema and presentation live in separate fields of
the same document. Stable IDs separate identity from SQL names, so a rename does
not invalidate every relationship. Ordered arrays preserve column order,
composite-key order and deliberate display ordering.

Version numbers are explicit. Unknown versions and fields are errors rather than
instructions to guess or silently discard data. The format has no includes,
aliases, executable layout expressions or identities regenerated on load. SQL
expressions in defaults, generated columns and CHECKs are parsed as data; the
editor does not execute them.

Normal loading preserves stored positions, widths, group bounds, membership,
relationship bends and viewport. Any future automatic arrangement must be an
explicit edit that saves its result into those same fields. The current SQL
importer assigns an initial grid; semantic grouping and AI arrangement are not
implemented.

Specifications:

- [JSON, RFC 8259](https://www.rfc-editor.org/info/rfc8259/)
- [YAML 1.2.2](https://yaml.org/spec/1.2.2/)
- [TOML 1.0](https://toml.io/en/v1.0.0)
- [DBML syntax](https://dbml.dbdiagram.io/docs/)

## SQL parser

**Vitess sqlparser** supplies a Go AST and MySQL grammar. It handles quoted
identifiers, comments, expressions and nested parentheses without a bespoke SQL
lexer. Its grammar covers explicit index directions, visibility and quoted
character-set/collation names used by the importer.

Parsing is only the first check. The importer explicitly maps supported AST
attributes into the model and rejects unsupported constructs. Accepting an AST
without retaining every meaningful attribute would produce an incomplete schema.
MySQL is still needed to establish server acceptance and equivalence.

Vitess is a substantial module, pinned in `go.mod`. Only the packages used by the
application are linked; no Vitess server is needed at runtime.

**PingCAP's MySQL parser**, maintained within TiDB, is an alternative. Its parser
submodule has broad grammar coverage and dependencies including PingCAP
errors/log/failpoint, Zap and parser tooling. Carrying two parsers would add
complexity without replacing real-MySQL testing. A future parser change
should address a concrete unsupported requirement and pass the same equivalence
suite.

Sources:

- [Vitess grammar](https://github.com/vitessio/vitess/blob/main/go/vt/sqlparser/sql.y)
- [PingCAP parser](https://github.com/pingcap/tidb/blob/master/pkg/parser/README.md)
- [PingCAP parser dependencies](https://github.com/pingcap/tidb/blob/master/pkg/parser/go.mod)

## Schema diff

`sciurus diff` reads ER JSON or supported SQL into the same typed schema model.
It renders each table's SQL and parses it with **Vitess sqlparser**, then applies
Sciurus's own normalization and comparison rules. Objects are matched by SQL
name, so renames appear as removals and additions. Text and PNG reports use the
same comparison result.

Normalization ignores formatting, SQL comments, table ordering and diagram
metadata. It handles known type aliases and equivalent defaults, and retains
meaningful column order, composite-key order and InnoDB clustered-key selection.
Columns, indexes, constraints and table options are compared as normalized SQL
definitions.

This is an offline comparison of supported schema definitions. Execution guards
such as `IF NOT EXISTS` are outside that comparison. Unspecified database defaults
remain unknown, and differently written expressions are not generally proven
equivalent. Schemadiff and MySQL check schema equivalence in tests below; neither
is invoked by `diff`.

## Independent schema comparison

**Vitess schemadiff** compares independently supplied SQL fixtures with generated
SQL after import and JSON save/load. It parses the fixture definitions directly,
without Sciurus's schema model or comparison normalization. Comparisons run in
both directions with strict constraint names and AUTO_INCREMENT values. The
library is embedded in the Go tests; no external CLI or database is needed.

The installed library accepts circular foreign-key schemas. It shares Vitess's
parser with Sciurus, so **MySQL** supplies a separate check: create both schemas
and compare database defaults and complete ordered `SHOW CREATE TABLE` results.
These live tests remain required in CI.

Sciurus keeps its own report comparison because its contract includes unknown
server defaults and InnoDB clustered-key selection. Schemadiff assumes an
environment default collation and does not support strict index ordering. Its
schema normalization therefore cannot replace Sciurus's rules directly.

Sources:

- [Vitess schemadiff API](https://pkg.go.dev/vitess.io/vitess/go/vt/schemadiff)
- [Vitess schema validation](https://github.com/vitessio/vitess/blob/v0.24.2/go/vt/schemadiff/schema.go)
- [Vitess schema-diff design](https://vitess.io/blog/2023-04-24-schemadiff/)

## Semantics that must survive a round trip

The model and test fixtures distinguish properties that can otherwise
look interchangeable:

- An omitted default, explicit SQL NULL, an empty string and a SQL expression.
- ENUM/SET value order, column order and composite index/foreign-key part order.
- Invisible indexes and deliberate redundant indexes, which must remain named
  and present even if another index covers their columns.
- Primary-key nullability, which MySQL normalizes to NOT NULL.
- Inherited versus explicit character sets and collations.
- Quoted SQL option text versus the unquoted value stored in the model.
- Self references and multi-table foreign-key cycles.

The independent source SQL fixtures exercise generated columns, CHECKs,
expression and temporal defaults, quoted identifiers, index prefixes/directions,
FULLTEXT/SPATIAL indexes and invisibility. Seeded schemas vary foreign-key rings,
self references, types, defaults, nullability, collation, referential actions and
creation order. These fixtures are constructed without the generator under test.

## Workbench import

A `.mwb` file is a ZIP archive containing `document.mwb.xml`. Direct import maps
one physical MySQL schema into the typed model, including its tables, columns,
indexes and physical foreign keys. Model-only and commented-out objects are
excluded from the physical schema. Unsupported schema attributes and populated
views, routines or events fail explicitly.

Indexes maintained only for model-only foreign keys are omitted unless shared
with a physical relationship or used as a primary key. Foreign keys referencing
model-only tables are also omitted. These distinctions follow
[Workbench's SQL export rules](https://github.com/mysql/mysql-workbench/blob/8.0/modules/db.mysql/src/db_mysql_diffsqlgen.cpp#L171-L208).

Direct import accepts zero or one diagram. Without a diagram, tables receive
deterministic grid positions. A diagram supplies table positions, visible group
bounds, colors, membership and viewport. Workbench table positions can be
relative to a layer; the enclosing layer origin is added to get Sciurus's
absolute coordinates. The root layer represents the canvas and does not become
a visible group. Multiple diagrams are rejected.

With a SQL input and `--workbench` layout archive, SQL remains authoritative for
database semantics. Layout import reads exactly one diagram and matches tables
by SQL name. It must leave generated SQL byte-identical. Synthetic archives
exercise direct schema import and these separate layout contracts.

## Acceptance criteria

Determinism and equivalence are different requirements. Repeated output can be
consistently wrong, and semantically equivalent output can still produce noisy
file diffs. Sciurus therefore checks:

1. Byte equality across repeated generation and JSON round trips, allowing only
   the generated-at comment to change when the timestamp changes.
2. Bidirectional schemadiff comparisons of independently supplied source SQL and
   generated SQL, with no schema changes reported.
3. Exact live MySQL definitions from independently created source and generated
   databases, including database defaults and column/index order.

See [the testing guide](testing.md) for reproducible commands, test
coverage and prerequisite handling.
