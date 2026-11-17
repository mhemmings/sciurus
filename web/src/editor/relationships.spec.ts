import { expect, test } from "vitest";
import { clone, newDocument, newTable } from "./model";
import { createRelationship } from "./relationships";

test("composite one-to-many keys copy storage properties without copying generated value behavior", () => {
  const document = newDocument();
  const parent = newTable("account", { x: 0, y: 0 });
  const child = newTable("entry", { x: 500, y: 0 });
  parent.charset = "utf8mb4";
  parent.collation = "utf8mb4_bin";
  parent.columns.push({
    id: "tenant",
    name: "tenant",
    type: "varchar(80)",
    nullable: false,
    default: "'test'",
    comment: "not inherited",
    invisible: true,
  });
  parent.columns[0]!.type = "bigint unsigned";
  parent.indexes[0]!.columns.unshift({ column: "tenant" });
  parent.indexes.push({
    id: "auto",
    name: "auto",
    kind: "INDEX",
    columns: [{ column: parent.columns[0]!.id }],
  });
  child.charset = "latin1";
  document.tables = [parent, child];
  const parentBefore = clone(parent);
  const result = createRelationship(document, "one-to-many", parent.id, child.id);
  expect(result).toEqual({ tableId: child.id, keyId: child.foreignKeys[0]!.id });
  expect(parent).toEqual(parentBefore);
  expect(child.columns.slice(1).map(({ id: _id, ...column }) => column)).toEqual([
    {
      name: "account_tenant",
      type: "varchar(80)",
      nullable: true,
      charset: "utf8mb4",
      collation: "utf8mb4_bin",
    },
    { name: "account_id", type: "bigint unsigned", nullable: true },
  ]);
  expect(child.foreignKeys[0]!.referencedColumns).toEqual([
    "tenant",
    parent.columns[0]!.id,
  ]);
  expect(child.indexes.at(-1)!.kind).toBe("INDEX");
  expect(child.indexes.at(-1)!.columns.map((part) => part.column)).toEqual(
    child.foreignKeys[0]!.columns,
  );
});

test("one-to-one self references use a separate unique key and collision-safe MySQL names", () => {
  const document = newDocument();
  const table = newTable("a".repeat(64), { x: 0, y: 0 });
  table.columns[0]!.name = "b".repeat(64);
  document.tables = [table];
  expect(
    createRelationship(document, "one-to-one", table.id, table.id),
  ).not.toHaveProperty("error");
  expect(
    createRelationship(document, "one-to-one", table.id, table.id),
  ).not.toHaveProperty("error");
  expect(table.indexes.slice(1).map((index) => index.kind)).toEqual(["UNIQUE", "UNIQUE"]);
  expect(table.foreignKeys.map((key) => key.referencedColumns)).toEqual([
    [table.columns[0]!.id],
    [table.columns[0]!.id],
  ]);
  for (const names of [
    table.columns.map((column) => column.name),
    table.indexes.map((index) => index.name),
    table.foreignKeys.map((key) => key.name),
  ]) {
    expect(new Set(names.map((name) => name.toLowerCase())).size).toBe(names.length);
    expect(names.every((name) => [...name].length <= 64)).toBe(true);
  }
});

test("many-to-many creates a nonoverlapping junction with a composite PK and both leading FK indexes", () => {
  const document = newDocument();
  const parent = newTable("account", { x: 0, y: 0 });
  const child = newTable("role", { x: 400, y: 0 });
  parent.columns.push({
    id: "tenant",
    name: "tenant",
    type: "int unsigned",
    nullable: false,
  });
  parent.indexes[0]!.columns.unshift({ column: "tenant" });
  parent.indexes.push({
    id: "auto",
    name: "auto",
    kind: "INDEX",
    columns: [{ column: parent.columns[0]!.id }],
  });
  document.tables = [parent, child, newTable("obstacle", { x: 200, y: 194 })];
  const before = clone(document.tables);
  const result = createRelationship(document, "many-to-many", parent.id, child.id);
  expect(result).not.toHaveProperty("error");
  expect(document.tables.slice(0, 3)).toEqual(before);
  const junction = document.tables[3]!;
  expect(result).toEqual({ tableId: junction.id, keyId: junction.foreignKeys[0]!.id });
  expect(junction.columns).toHaveLength(3);
  expect(
    junction.columns.every((column) => !column.nullable && !column.autoIncrement),
  ).toBe(true);
  expect(junction.indexes[0]!.kind).toBe("PRIMARY");
  expect(junction.indexes[0]!.columns.map((part) => part.column)).toEqual(
    junction.columns.map((column) => column.id),
  );
  expect(junction.indexes[1]!.columns.map((part) => part.column)).toEqual(
    junction.foreignKeys[1]!.columns,
  );
  expect(junction.foreignKeys.map((key) => key.referencedTable)).toEqual([
    parent.id,
    child.id,
  ]);
  expect(junction.position.y).toBeGreaterThan(document.tables[2]!.position.y + 70);
});

test("key selection skips invisible and prefix keys, uses complete UNIQUE keys, and preflights failures atomically", () => {
  const document = newDocument();
  const parent = newTable("account", { x: 0, y: 0 });
  const child = newTable("entry", { x: 400, y: 0 });
  parent.columns[0]!.autoIncrement = false;
  parent.indexes = [
    {
      id: "hidden",
      name: "hidden",
      kind: "UNIQUE",
      invisible: true,
      columns: [{ column: parent.columns[0]!.id }],
    },
    {
      id: "prefix",
      name: "prefix",
      kind: "UNIQUE",
      columns: [{ column: parent.columns[0]!.id, length: 4 }],
    },
  ];
  document.tables = [parent, child];
  const before = clone(document);
  expect(createRelationship(document, "one-to-many", parent.id, child.id)).toHaveProperty(
    "error",
  );
  expect(document).toEqual(before);
  parent.indexes.push({
    id: "complete",
    name: "complete",
    kind: "UNIQUE",
    columns: [{ column: parent.columns[0]!.id }],
  });
  expect(
    createRelationship(document, "one-to-many", parent.id, child.id),
  ).not.toHaveProperty("error");
  const complete = clone(document);
  expect(
    createRelationship(document, "many-to-many", parent.id, parent.id),
  ).toHaveProperty("error");
  expect(createRelationship(document, "one-to-one", "deleted", child.id)).toHaveProperty(
    "error",
  );
  expect(document).toEqual(complete);
});

test("junction index limits and unresolved text defaults reject before adding any schema objects", () => {
  const document = newDocument();
  const parent = newTable("large_parent", { x: 0, y: 0 });
  const child = newTable("large_child", { x: 400, y: 0 });
  for (const table of [parent, child]) {
    table.columns[0]!.type = "varchar(500)";
    table.columns[0]!.autoIncrement = false;
    table.charset = "utf8mb4";
  }
  document.tables = [parent, child];
  const before = clone(document);
  expect(
    createRelationship(document, "many-to-many", parent.id, child.id),
  ).toHaveProperty("error", expect.stringContaining("3072-byte"));
  expect(document).toEqual(before);
  delete parent.charset;
  child.charset = "latin1";
  const unknown = clone(document);
  expect(createRelationship(document, "one-to-many", parent.id, child.id)).toHaveProperty(
    "error",
    expect.stringContaining("charset"),
  );
  expect(document).toEqual(unknown);
});

test("truncated names containing spaces remain valid and unique across columns, constraints and junction tables", () => {
  const document = newDocument();
  const parent = newTable("p".repeat(30), { x: 0, y: 0 });
  parent.columns[0]!.name = "c".repeat(32) + " x";
  const child = newTable("c".repeat(60) + " x", { x: 400, y: 0 });
  const peer = newTable("q".repeat(32) + " y", { x: 800, y: 0 });
  document.tables = [parent, child, peer];
  // Each source name is valid; each generated base lands on a space at the 64th character.
  expect(
    createRelationship(document, "one-to-many", parent.id, child.id),
  ).not.toHaveProperty("error");
  expect(
    createRelationship(document, "one-to-one", parent.id, child.id),
  ).not.toHaveProperty("error");
  expect(
    createRelationship(document, "many-to-many", parent.id, peer.id),
  ).not.toHaveProperty("error");
  expect(child.columns[1]!.name).toBe("p".repeat(30) + "_" + "c".repeat(32));
  expect(child.foreignKeys[0]!.name).toBe("fk_" + "c".repeat(60));
  expect(document.tables[3]!.name).toBe("p".repeat(30) + "_" + "q".repeat(32));
  const names = document.tables.flatMap((table) => [
    table.name,
    ...table.columns.map((column) => column.name),
    ...table.indexes.map((index) => index.name),
    ...table.foreignKeys.map((key) => key.name),
  ]);
  expect(names.every((name) => [...name].length <= 64 && !name.endsWith(" "))).toBe(true);
  expect(new Set(child.columns.map((column) => column.name.toLowerCase())).size).toBe(
    child.columns.length,
  );
  expect(
    new Set(
      document.tables.flatMap((table) =>
        table.foreignKeys.map((key) => key.name.toLowerCase()),
      ),
    ).size,
  ).toBe(4);
});
