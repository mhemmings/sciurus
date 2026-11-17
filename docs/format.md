# ER model format, version 1

An ER model is one UTF-8 JSON file containing both the MySQL schema and its
diagram. [model.schema.json](../model.schema.json) is the JSON Schema 2020-12
authoring contract; [the research notes](research.md) explain the format choice.
The Go validator additionally checks SQL syntax, scoped identities, references
and supported database rules that JSON Schema cannot express.

Property names are case-sensitive. Unknown properties, duplicate JSON object
keys and unsupported `version` values are errors. Invalid UTF-8 and unpaired
Unicode surrogate escapes are rejected instead of silently repaired. Use explicit
required fields and `[]` for empty required collections. JSON null is supported
only for the optional `default` and `enforced` fields; omit other optional
properties instead. Do not add a `$schema` property to the model itself: configure
the schema association in your editor. Documents are
limited to 16 MiB. No includes or external files are needed to render a model.

## A complete model

Save this as `example.er.json`:

```json
{
  "version": 1,
  "name": "Accounts",
  "charset": "utf8mb4",
  "collation": "utf8mb4_bin",
  "tables": [
    {
      "id": "account",
      "name": "account",
      "position": { "x": 120, "y": 100 },
      "width": 300,
      "columns": [
        { "id": "id", "name": "id", "type": "BIGINT UNSIGNED", "nullable": false, "autoIncrement": true },
        { "id": "label", "name": "label", "type": "VARCHAR(80)", "nullable": false, "default": "'untitled'" }
      ],
      "indexes": [
        { "id": "pk", "name": "PRIMARY", "kind": "PRIMARY", "columns": [{ "column": "id" }] }
      ],
      "foreignKeys": []
    }
  ],
  "groups": [],
  "viewport": { "x": 60, "y": 60, "zoom": 1 }
}
```

Validate after editing, then generate SQL without starting a server:

```sh
sciurus validate example.er.json
sciurus generate --no-timestamp example.er.json > example.sql
```

Generated SQL creates tables in an already-created, empty database. `name` is a
model title, not an instruction to create or select a database. SQL never executes
inside the editor.

## Identity, names and safe edits

IDs are stable, case-sensitive references. They may initially resemble names,
but renaming a SQL object should change its `name`, not its `id`.

| Object | ID scope | SQL-name scope |
| --- | --- | --- |
| Table | Entire document | Entire document |
| Group | Entire document, independently of table IDs | No SQL meaning |
| Column | Its owning table | Its owning table |
| Index | Its owning table, independently of column IDs | Its owning table |
| Foreign key | Entire document, independently of other ID kinds | All foreign keys in the document |
| CHECK | No separate ID | All CHECK constraints in the document |

SQL-name duplicate checks are case-insensitive. SQL identifiers are raw strings,
up to 64 Unicode characters; do not wrap them in backticks. Generation quotes
them safely. Identifiers cannot contain NUL, CR, LF or TAB, consist entirely of
whitespace, or end in an ASCII space. IDs cannot be blank or contain those control
characters and are limited to 200 UTF-8 bytes. The JSON Schema's character limit
is supplemented by this byte limit in the Go validator.

- Rename a table or column by preserving its ID. Index and foreign-key references
  continue pointing to the same object.
- Generated-column and CHECK expressions contain **SQL column names**, not IDs.
  Update those expressions when renaming a referenced column. The validator
  catches references to names that no longer exist; it does not rewrite SQL text
  or infer the intended meaning of a name reused for a different column.
- Before deleting a column, remove or update its index parts, incoming/outgoing
  foreign-key references and named SQL-expression references. Before deleting a
  table, remove incoming foreign keys. Broken references prevent saving/generation.
- When deleting a group, clear its members' `groupId`; tables retain their
  absolute positions. Group membership alone never moves a table.

## Document and table fields

| Field | Meaning |
| --- | --- |
| `version` | Required integer `1`; other versions fail explicitly |
| `name` | Required nonblank model title |
| `charset`, `collation` | Optional defaults for tables, using unquoted MySQL names |
| `tables` | Required ordered array, maximum 4,096 tables |
| `groups` | Required ordered array of presentation-only layers |
| `viewport` | Required `{x, y, zoom}` view state |

Every table requires `id`, `name`, `position`, `width`, `columns`, `indexes` and
`foreignKeys`. The column array has 1–1,017 entries. Empty indexes and foreign
keys are represented by `[]`. Optional `checks` is an ordered CHECK array.
Optional `engine` is `InnoDB` (case-insensitive); omission or `""` also selects
InnoDB. Other engines are rejected. `comment` is a literal SQL comment, with no
SQL quotes around its JSON value. `groupId` is an optional existing group ID;
omission or `""` means ungrouped.

`position` is the absolute top-left `{x, y}` corner, including for grouped tables.
`width` is in diagram units. Table height is derived from its displayed contents;
there is no stored table `height` property.

Table `charset` and `collation` form an override pair. Only when **both** are
empty/omitted does generation inherit the document pair. If a table supplies
either one, generation emits only that table's explicit settings and lets MySQL
resolve the rest:

| Document defaults | Table settings | Result |
| --- | --- | --- |
| `utf8mb4` / `utf8mb4_bin` | Neither | Inherit both document values |
| `utf8mb4` / `utf8mb4_bin` | `collation: "latin1_swedish_ci"` | MySQL infers latin1; no utf8mb4 override is added |
| `utf8mb4` / `utf8mb4_bin` | `charset: "utf8mb4"` only | Use MySQL's default collation for utf8mb4, not the document collation |
| Neither | Neither | Inherit the existing database's defaults |

Column-level text settings follow MySQL's own inheritance rules. Charset and
collation strings may contain only ASCII letters, digits and underscores;
the server remains the authority on valid names and combinations.

## Columns and SQL expressions

Every column requires `id`, `name`, `type` and `nullable`.

| Field | Meaning when supplied |
| --- | --- |
| `type` | MySQL type only, including length/precision, `UNSIGNED`, or ordered ENUM/SET values |
| `nullable` | Whether SQL NULL is permitted; must be false for PRIMARY and AUTO_INCREMENT columns |
| `autoIncrement` | Integer AUTO_INCREMENT; at most one per table, leading a visible full index, without an explicit default |
| `default` | SQL expression string; JSON null is equivalent to omission |
| `onUpdate` | SQL ON UPDATE expression such as `CURRENT_TIMESTAMP(6)` |
| `charset`, `collation` | Optional explicit column text settings |
| `comment` | Literal SQL comment, without SQL quoting |
| `generated` | Generated-column expression, without `GENERATED ALWAYS AS` or its outer wrapper |
| `stored` | True means STORED; false/omitted means VIRTUAL; requires `generated` |
| `invisible` | MySQL column invisibility, independent of editor visibility |

Optional booleans default to false. Empty/omitted `onUpdate` and `generated`
mean none. A generated column cannot also have a default, AUTO_INCREMENT or ON
UPDATE expression. The `type` field must not hide structured attributes such as
`NOT NULL`, `DEFAULT`, keys or character settings. For example, use
`"type": "INT UNSIGNED", "nullable": false`, not `"type": "INT NOT NULL"`.

Defaults are SQL text inside JSON strings:

| JSON representation | Generated meaning |
| --- | --- |
| Property omitted, or `"default": null` | No explicit DEFAULT clause |
| `"default": "NULL"` | SQL `DEFAULT NULL`; column must be nullable |
| `"default": "''"` | Empty SQL string |
| `"default": "'NULL'"` | The four-character SQL string NULL |
| `"default": "0"` | Numeric zero |
| `"default": "CURRENT_TIMESTAMP(6)"` | Temporal default expression |
| `"default": "(JSON_OBJECT())"` | Parenthesized JSON expression default |

The default string `""` is invalid. Quote SQL strings inside the JSON value and
escape JSON quotes/backslashes as usual. ENUM/SET member order is preserved and
must not be alphabetized. Defaults do not accept column references. Generated
and CHECK expressions may refer to existing, unqualified SQL column names in the
same table. Subqueries and bind parameters are unsupported in schema expressions.
Parser validation does not emulate all MySQL restrictions on functions, row/index
sizes or server-version features.

## Indexes, foreign keys and CHECKs

An index requires `id`, `name`, `kind` and `columns`. `kind` is exactly `PRIMARY`,
`UNIQUE`, `INDEX`, `FULLTEXT` or `SPATIAL`. At most one PRIMARY is permitted; it
must be named `PRIMARY` and cannot be invisible. Optional `comment` is literal
text. Optional `invisible` defaults to false.

Each index has 1–16 ordered parts. A part is `{ "column": "column-id" }`, with
optional integer `length` and boolean `descending`. `length: 0` or omission
means the entire column; positive values request a MySQL prefix. Prefix units
depend on the string type. `descending: true` emits DESC; otherwise ASC is used.
Each column may appear only once within an index. Part order is significant.

A foreign key requires `id`, `name`, `columns`, `referencedTable` and
`referencedColumns`. `columns` lists source-column IDs in the owning table;
`referencedTable` is a table ID; `referencedColumns` lists column IDs in that
target table. Source/target arrays must be nonempty, equally sized, ordered and
contain distinct IDs. Target columns must match a full visible PRIMARY/UNIQUE
key in the same order. Types and signs must match, along with applicable decimal
precision/scale and explicit text settings. Self references and cycles are valid.

Optional `onDelete` and `onUpdate` accept `NO ACTION`, `RESTRICT`, `CASCADE` and
`SET NULL`, or `""` to omit the clause. SET NULL requires nullable source columns.
Optional `points` stores ordered, absolute `{x, y}` route bends from source to
target. The editor connects the table endpoints through those bends using only
horizontal and vertical segments. Dragging a segment saves its interior route
points. If saved points or moved endpoints would create a diagonal, rendering
inserts deterministic right-angle elbows without changing the stored positions.
Empty or omitted points use a deterministic default route; saved manual points
are not replaced by a layout pass.

A CHECK requires a SQL `name` and `expression`. Write the predicate without
`CHECK` or its surrounding wrapper. Optional `enforced: false` emits NOT ENFORCED;
true, null and omission all mean enforced. CHECK names have their own document-wide
namespace, separate from foreign-key names.

## Groups, view and deterministic output

A group requires `id`, nonblank `name`, six-digit hex `color`, absolute
`position`, `width` and `height`. Membership is exclusively the table's
`groupId`; there is no second list of member tables to keep synchronized.
Moving a group in the editor translates its members' absolute table positions.
Dragging a table assigns membership when the center of its header is inside a
group's body (below its 40-unit header). Overlapping groups prefer the smallest
area, then the last group in array order. Dropping outside all groups clears
membership. This inference occurs only on interactive placement or drag completion;
loading a model preserves its explicit membership.
When editing JSON directly, update both the group rectangle and member positions
for the same effect. Loading a different group rectangle does not infer movement.

One diagram unit is one CSS pixel at zoom 1. Coordinates may be fractional or
negative, bounded to ±10,000,000. Table/group widths are 80–10,000,000; group
heights are 60–10,000,000. NaN and infinities are invalid. Viewport `zoom` ranges
from 0.02 to 4; viewport `x`/`y` are screen translations:

```text
screen.x = diagram.x × viewport.zoom + viewport.x
screen.y = diagram.y × viewport.zoom + viewport.y
```

Normal loading never rearranges tables or groups. SQL imports receive
deterministic initial positions unless a matching Workbench archive supplies the
layout. Direct Workbench imports read schema and diagram geometry from the same
archive, using initial grid positions when no diagram is present. Subsequent
model loads use the saved coordinates.

Saving uses two-space JSON indentation and a final newline. Field order is fixed
by the model; collection order is preserved. Optional zero values may be omitted,
so saving hand-edited JSON can normalize its whitespace and optional fields.
Comments are explicit fields, never JSON comments.

SQL generation sorts tables by their SQL names without mutating the document.
It preserves column order, index order and index-part order, foreign-key order
and each FK's column order, followed by CHECK order. IDs, table array order,
layout, colors and viewport never affect the SQL. Repeated generation is
byte-identical except for the single generated-at comment; `--no-timestamp`
makes every byte deterministic.

Always finish manual or agent edits with `sciurus validate FILE`.
JSON Schema validation is useful in an editor, but cannot replace SQL parsing or
the validator's cross-object checks.
