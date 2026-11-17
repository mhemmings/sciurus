import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { assertLayout, layoutExample } from "./layout-examples.ts";
import type { ERDocument } from "../src/editor/types.ts";

/** Strip exactly the authorized drawing fields, retaining all schema data and membership. */
function schema(document: ERDocument) {
  const { viewport: _viewport, groups, tables, ...metadata } = document;
  return {
    ...metadata,
    groups: groups.map(
      ({
        position: _position,
        width: _width,
        height: _height,
        color: _color,
        ...group
      }) => group,
    ),
    tables: tables.map(
      ({ position: _position, width: _width, foreignKeys, ...table }) => ({
        ...table,
        foreignKeys: foreignKeys.map(({ points: _points, ...key }) => key),
      }),
    ),
  };
}

test("layout repairs stale routes, oversized names and unequal table heights", () => {
  const document: ERDocument = {
    version: 1,
    name: "Unequal tables",
    viewport: { x: 0, y: 0, zoom: 1 },
    groups: [
      {
        id: "group",
        name: "Group",
        color: "#ffffff",
        position: { x: 0, y: 0 },
        width: 10,
        height: 10,
      },
    ],
    tables: Array.from({ length: 12 }, (_, index) => ({
      id: `table_${index}`,
      name:
        index === 0
          ? "a_table_with_a_name_that_requires_a_wider_header_than_a_normal_card"
          : `table_${index}`,
      position: { x: 0, y: 0 },
      width: 1,
      ...(index < 9 ? { groupId: "group" } : {}),
      columns: Array.from({ length: index * 2 + 1 }, (_, column) => ({
        id: `column_${column}`,
        name:
          column === 0
            ? "a_long_column_name_that_must_never_be_truncated_in_the_diagram"
            : `column_${column}`,
        type: "VARCHAR(255)",
        nullable: true,
      })),
      indexes: [],
      foreignKeys:
        index === 1
          ? [
              {
                id: "fk",
                name: "fk",
                columns: ["column_0"],
                referencedTable: "table_0",
                referencedColumns: ["column_0"],
                points: [{ x: 1, y: 2 }],
              },
            ]
          : [],
    })),
  };
  const arranged = layoutExample(document);
  assertLayout(arranged);
  assert.deepEqual(schema(arranged), schema(document));
  assert.deepEqual(layoutExample(arranged), arranged);
  assert.equal(arranged.tables[1]!.foreignKeys[0]!.points, undefined);
  assert.ok(arranged.tables[0]!.width > 600);
  // The caller's model is untouched, even when it starts with every table overlapping.
  assert.equal(document.tables[0]!.width, 1);
  assert.deepEqual(document.tables[1]!.foreignKeys[0]!.points, [{ x: 1, y: 2 }]);
});

test("the independent geometry audit rejects collisions, overflow and insufficient name widths", async () => {
  const document = JSON.parse(
    await readFile(new URL("../../examples/starter.json", import.meta.url), "utf8"),
  ) as ERDocument;
  const overlapping = structuredClone(document);
  overlapping.tables[1]!.position = { ...overlapping.tables[0]!.position };
  assert.throws(() => assertLayout(overlapping), /overlaps/);
  const overflowing = structuredClone(document);
  overflowing.groups[0]!.height = 1;
  assert.throws(() => assertLayout(overflowing), /outside/);
  const clipped = structuredClone(document);
  clipped.tables[0]!.width = 50;
  assert.throws(() => assertLayout(clipped), /clipped names/);
});
