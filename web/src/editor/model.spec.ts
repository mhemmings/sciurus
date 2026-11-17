import { describe, expect, test } from "vitest";
import {
  clone,
  displayColumnType,
  groupForTable,
  moveGroup,
  moveRouteSegment,
  relationshipPath,
  removeColumn,
  removeTable,
} from "./model";
import type { ERDocument, Point } from "./types";

/** Every drawn span must stay horizontal or vertical, including endpoint repairs. */
function expectOrthogonal(path: Point[]): void {
  expect(path.length).toBeGreaterThanOrEqual(2);
  for (let index = 1; index < path.length; index++) {
    expect(
      path[index]!.x === path[index - 1]!.x || path[index]!.y === path[index - 1]!.y,
    ).toBe(true);
  }
}

/** A composite relationship and a self-reference expose destructive-edit mistakes. */
function fixture(): ERDocument {
  return {
    version: 1,
    name: "test",
    viewport: { x: 5, y: 8, zoom: 0.7 },
    groups: [
      {
        id: "group",
        name: "Accounts",
        color: "#507060",
        position: { x: 20, y: 30 },
        width: 600,
        height: 600,
      },
    ],
    tables: [
      {
        id: "parent",
        name: "account",
        position: { x: 40, y: 60 },
        width: 220,
        groupId: "group",
        columns: [
          { id: "tenant", name: "tenant_id", type: "BIGINT", nullable: false },
          { id: "id", name: "id", type: "BIGINT", nullable: false },
          { id: "other", name: "description", type: "TEXT", nullable: true },
        ],
        indexes: [
          {
            id: "pk",
            kind: "PRIMARY",
            name: "PRIMARY",
            columns: [{ column: "tenant" }, { column: "id" }],
          },
          {
            id: "independent",
            kind: "INDEX",
            name: "description",
            columns: [{ column: "other", length: 20 }],
          },
        ],
        foreignKeys: [
          {
            id: "self",
            name: "self",
            columns: ["tenant", "id"],
            referencedTable: "parent",
            referencedColumns: ["tenant", "id"],
            points: [{ x: 300, y: 100 }],
          },
        ],
      },
      {
        id: "child",
        name: "entry",
        position: { x: 340, y: 80 },
        width: 220,
        groupId: "group",
        columns: [
          { id: "child_tenant", name: "tenant_id", type: "BIGINT", nullable: false },
          { id: "child_parent", name: "account_id", type: "BIGINT", nullable: false },
        ],
        indexes: [],
        foreignKeys: [
          {
            id: "fk",
            name: "entry_account",
            columns: ["child_tenant", "child_parent"],
            referencedTable: "parent",
            referencedColumns: ["tenant", "id"],
            points: [
              { x: 310, y: 100 },
              { x: 310, y: 160 },
            ],
          },
        ],
      },
      {
        id: "outside",
        name: "audit",
        position: { x: 900, y: 60 },
        width: 200,
        columns: [{ id: "audit_id", name: "id", type: "BIGINT", nullable: false }],
        indexes: [],
        foreignKeys: [
          {
            id: "outside_fk",
            name: "audit_account",
            columns: ["audit_id"],
            referencedTable: "parent",
            referencedColumns: ["other"],
            points: [{ x: 800, y: 150 }],
          },
        ],
      },
    ],
  };
}

describe("model edits preserve references and exact geometry", () => {
  test("deleting one composite column removes whole dependent keys without changing unrelated keys", () => {
    const document = fixture();
    removeColumn(document, "parent", "id");
    expect(document.tables[0]!.columns.map((column) => column.id)).toEqual([
      "tenant",
      "other",
    ]);
    expect(document.tables[0]!.indexes.map((index) => index.id)).toEqual(["independent"]);
    expect(document.tables[0]!.foreignKeys).toEqual([]);
    expect(document.tables[1]!.foreignKeys).toEqual([]);
    expect(document.tables[2]!.foreignKeys.map((key) => key.id)).toEqual(["outside_fk"]);
  });
  test("deleting a source column removes the entire composite foreign key", () => {
    const document = fixture();
    removeColumn(document, "child", "child_tenant");
    expect(document.tables[1]!.foreignKeys).toEqual([]);
    expect(document.tables[0]!.foreignKeys[0]!.referencedColumns).toEqual([
      "tenant",
      "id",
    ]);
  });
  test("table deletion resolves incoming references and preserves surviving geometry", () => {
    const document = fixture();
    removeTable(document, "parent");
    expect(document.tables.map((table) => table.id)).toEqual(["child", "outside"]);
    expect(document.tables.flatMap((table) => table.foreignKeys)).toEqual([]);
    expect(document.tables[0]!.position).toEqual({ x: 340, y: 80 });
  });
  test("moving a group translates each member and owned bend exactly once", () => {
    const document = fixture();
    moveGroup(document, "group", { x: -7.25, y: 18.5 });
    expect(document.groups[0]!.position).toEqual({ x: 12.75, y: 48.5 });
    expect(document.tables[0]!.position).toEqual({ x: 32.75, y: 78.5 });
    expect(document.tables[1]!.position).toEqual({ x: 332.75, y: 98.5 });
    expect(document.tables[1]!.foreignKeys[0]!.points).toEqual([
      { x: 302.75, y: 118.5 },
      { x: 302.75, y: 178.5 },
    ]);
    expect(document.tables[0]!.foreignKeys[0]!.points).toEqual([{ x: 292.75, y: 118.5 }]);
    expect(document.tables[2]!.position).toEqual({ x: 900, y: 60 });
    expect(document.tables[2]!.foreignKeys[0]!.points).toEqual([{ x: 800, y: 150 }]);
  });
  test("routes follow column IDs after reordering while retaining saved bends", () => {
    const document = fixture();
    const parent = document.tables[0]!;
    const child = document.tables[1]!;
    parent.columns.push(parent.columns.shift()!);
    const route = relationshipPath(child, parent, "child_parent", "id", [
      { x: 310.125, y: 200.5 },
    ]);
    expect(route[0]).toEqual({ x: 340, y: 163 });
    expect(route.at(-1)).toEqual({ x: 260, y: 115 });
    expect(route).toContainEqual({ x: 310.125, y: 200.5 });
    expectOrthogonal(route);
  });
  test("independent snapshots preserve null-default distinctions and cannot mutate undo state", () => {
    const document = fixture();
    document.tables[0]!.columns[0]!.default = "NULL";
    const snapshot = clone(document);
    document.tables[0]!.columns[0]!.default = "0";
    document.tables[0]!.position.x = 999;
    expect(snapshot.tables[0]!.columns[0]!.default).toBe("NULL");
    expect(snapshot.tables[0]!.columns[1]!.default).toBeUndefined();
    expect(snapshot.tables[0]!.position.x).toBe(40);
  });
});

test("group ownership uses the header center, nested area and actual paint order", () => {
  const document = fixture();
  const table = document.tables[0]!;
  document.groups.push({
    id: "nested",
    name: "Nested",
    color: "#000000",
    position: { x: 100, y: 100 },
    width: 300,
    height: 300,
  });
  table.position = { x: 110, y: 110 };
  expect(groupForTable(document, table)).toBe("group"); // The inner group header is not a drop target.
  table.position.y = 140;
  expect(groupForTable(document, table)).toBe("nested");
  document.groups.push({ ...document.groups[1]!, id: "above" });
  expect(groupForTable(document, table)).toBe("above");
  table.position.x = 270; // The right edge protrudes; membership follows the center as requested.
  expect(groupForTable(document, table)).toBe("above");
  table.position = { x: 1000, y: 1000 };
  expect(groupForTable(document, table)).toBeUndefined();
});

test("type presentation uppercases keywords while preserving quoted values and escapes", () => {
  expect(displayColumnType("bigint unsigned")).toBe("BIGINT UNSIGNED");
  expect(
    displayColumnType(
      String.raw`enum('Ready','can\'t','dou''ble',"Mixed") character set utf8mb4`,
    ),
  ).toBe(String.raw`ENUM('Ready','can\'t','dou''ble',"Mixed") CHARACTER SET UTF8MB4`);
  expect(displayColumnType("set('a','B','quote\\\\')")).toBe("SET('a','B','quote\\\\')");
});

test("saved bends stay orthogonal after either table moves and self references retain a visible loop", () => {
  const document = fixture();
  const source = document.tables[1]!;
  const target = document.tables[0]!;
  const original = relationshipPath(source, target, "child_parent", "id", [
    { x: 310.5, y: 240.25 },
    { x: 280.75, y: 90.5 },
  ]);
  expectOrthogonal(original);
  source.position = { x: 1000.25, y: 450.5 };
  target.position = { x: -700, y: -250 };
  const moved = relationshipPath(
    source,
    target,
    "child_parent",
    "id",
    original.slice(1, -1),
  );
  expectOrthogonal(moved);
  expect(moved[0]!.y).toBe(533.5);
  expect(moved.at(-1)!.y).toBe(-167);
  const self = relationshipPath(source, source, "child_parent", "child_parent");
  expectOrthogonal(self);
  expect(self[0]).toEqual(self.at(-1));
  expect(Math.max(...self.map((point) => point.x))).toBeGreaterThan(
    source.position.x + source.width,
  );
  expect(new Set(self.map((point) => point.y)).size).toBeGreaterThan(1);
  const collapsed = relationshipPath(source, source, "child_parent", "child_parent", [
    self[0]!,
  ]);
  expectOrthogonal(collapsed);
  expect(new Set(collapsed.map((point) => point.x)).size).toBeGreaterThan(1);
  expect(new Set(collapsed.map((point) => point.y)).size).toBeGreaterThan(1);
});

test("segment dragging fixes endpoints, constrains movement and inserts endpoint doglegs", () => {
  const path = [
    { x: 0, y: 10 },
    { x: 40, y: 10 },
    { x: 40, y: 80 },
    { x: 160, y: 80 },
  ];
  const snapshot = clone(path);
  expect(moveRouteSegment(path, 1, { x: 70, y: 500 })).toEqual([
    { x: 0, y: 10 },
    { x: 70, y: 10 },
    { x: 70, y: 80 },
    { x: 160, y: 80 },
  ]);
  for (let index = 0; index < path.length - 1; index++) {
    const dragged = moveRouteSegment(path, index, { x: 70, y: 130 });
    expectOrthogonal(dragged);
    expect(dragged[0]).toEqual(path[0]);
    expect(dragged.at(-1)).toEqual(path.at(-1));
    if (index !== 1)
      expect(
        dragged.some((point, i) => i > 0 && point.y === 130 && dragged[i - 1]!.y === 130),
      ).toBe(true);
  }
  const straight = [
    { x: 0, y: 12 },
    { x: 100, y: 12 },
  ];
  const dragged = moveRouteSegment(straight, 0, { x: 999, y: -40 });
  expectOrthogonal(dragged);
  expect(dragged[0]).toEqual(straight[0]);
  expect(dragged.at(-1)).toEqual(straight.at(-1));
  expect(dragged).toContainEqual({ x: 32, y: -40 });
  expect(dragged).toContainEqual({ x: 68, y: -40 });
  expect(path).toEqual(snapshot);
});

test("persisting a dragged middle segment across either card center cannot change attachment sides", () => {
  const document = fixture();
  const source = document.tables[1]!;
  const target = document.tables[0]!;
  const route = relationshipPath(source, target, "child_parent", "id");
  // Cross the target's center to the left, then the source's center to the right.
  for (const x of [-100, 600]) {
    const dragged = moveRouteSegment(route, 1, { x, y: 999 });
    const restored = relationshipPath(
      source,
      target,
      "child_parent",
      "id",
      dragged.slice(1, -1),
    );
    expectOrthogonal(restored);
    expect(restored[0]).toEqual(route[0]);
    expect(restored.at(-1)).toEqual(route.at(-1));
    expect(restored.some((point) => point.x === x)).toBe(true);
  }
  const self = relationshipPath(source, source, "child_parent", "child_parent");
  const dragged = moveRouteSegment(self, 1, { x: source.position.x - 100, y: 999 });
  const restored = relationshipPath(
    source,
    source,
    "child_parent",
    "child_parent",
    dragged.slice(1, -1),
  );
  expectOrthogonal(restored);
  expect(restored[0]).toEqual(self[0]);
  expect(restored.at(-1)).toEqual(self.at(-1));
});
