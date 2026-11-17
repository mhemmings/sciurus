import { groupForTable, id, recommendedTableWidth, tableHeight } from "./model";
import type { Column, ERDocument, Table } from "./types";

export type RelationshipKind = "one-to-one" | "one-to-many" | "many-to-many";

/** Produce a unique quoted-identifier-compatible name without exceeding MySQL's character limit. */
function uniqueName(base: string, existing: string[]): string {
  const occupied = new Set(existing.map((value) => value.toLowerCase()));
  // Truncation can land on an interior space in an otherwise valid quoted name.
  let candidate = [...base].slice(0, 64).join("").trimEnd();
  for (let suffix = 2; occupied.has(candidate.toLowerCase()); suffix++) {
    const ending = `_${suffix}`;
    candidate =
      [...base]
        .slice(0, 64 - ending.length)
        .join("")
        .trimEnd() + ending;
  }
  return candidate;
}

/** Resolve a complete visible key, preferring the primary key in the model's existing order. */
function referencedColumns(table: Table): Column[] | undefined {
  const candidates = table.indexes
    .filter(
      (index) =>
        !index.invisible && (index.kind === "PRIMARY" || index.kind === "UNIQUE"),
    )
    .sort((a, b) => Number(b.kind === "PRIMARY") - Number(a.kind === "PRIMARY"));
  for (const index of candidates) {
    if (
      !index.columns.length ||
      index.columns.length > 16 ||
      index.columns.some((part) => part.length)
    )
      continue;
    const columns = index.columns.map((part) =>
      table.columns.find((column) => column.id === part.column),
    );
    if (
      columns.every((column) => column && !(column.generated && !column.stored)) &&
      new Set(index.columns.map((part) => part.column)).size === columns.length
    )
      return columns as Column[];
  }
  return undefined;
}

/** Resolve text defaults as a pair; a charset override resets an inherited collation. */
function textOptions(document: ERDocument, table: Table, column: Column) {
  if (
    !/^\s*(?:char|varchar|tinytext|text|mediumtext|longtext|enum|set)\b/i.test(
      column.type,
    )
  )
    return {};
  for (const scope of [column, table, document]) {
    if (scope.collation)
      return {
        charset: scope.charset || scope.collation.split("_")[0],
        collation: scope.collation,
      };
    if (scope.charset) return { charset: scope.charset };
  }
  return {};
}

/** Conservatively bound a complete InnoDB key; unknown server character sets use four bytes. */
function keyBytes(column: Column): number {
  const parts = /^\s*(\w+)(?:\s*\(\s*(\d+)(?:\s*,\s*(\d+))?)?/i.exec(column.type);
  const type = parts?.[1]?.toLowerCase();
  const length = Number(parts?.[2] || 1);
  if (type === "char" || type === "varchar") {
    const charset = column.charset?.toLowerCase();
    const bytes =
      charset === "ascii" || charset === "latin1" || charset === "binary"
        ? 1
        : charset === "utf8" || charset === "utf8mb3"
          ? 3
          : charset === "ucs2"
            ? 2
            : 4;
    return length * bytes;
  }
  if (type === "binary" || type === "varbinary") return length;
  if (type === "bit") return Math.ceil(length / 8);
  if (type === "decimal" || type === "numeric" || type === "dec" || type === "fixed") {
    const precision = Number(parts?.[2] || 10);
    const scale = Number(parts?.[3] || 0);
    const integer = precision - scale;
    const remainder = [0, 1, 1, 2, 2, 3, 3, 4, 4];
    return (
      Math.floor(integer / 9) * 4 +
      remainder[integer % 9]! +
      Math.floor(scale / 9) * 4 +
      remainder[scale % 9]!
    );
  }
  if (["tinyint", "bool", "boolean", "year"].includes(type || "")) return 1;
  if (type === "smallint" || type === "enum") return 2;
  if (type === "mediumint" || type === "date") return 3;
  if (type === "float") return length > 24 ? 8 : 4;
  if (type === "int" || type === "integer") return 4;
  if (
    ["bigint", "double", "real", "datetime", "timestamp", "time", "set"].includes(
      type || "",
    )
  )
    return 8;
  return Infinity;
}

/** Create the requested relationship atomically, including copied columns and supporting keys. */
export function createRelationship(
  document: ERDocument,
  kind: RelationshipKind,
  parentID: string,
  childID: string,
): { tableId: string; keyId: string } | { error: string } {
  const parent = document.tables.find((table) => table.id === parentID);
  const child = document.tables.find((table) => table.id === childID);
  if (!parent || !child) return { error: "Both relationship tables must still exist." };
  if (kind === "many-to-many" && parentID === childID)
    return { error: "Choose two different tables for a many-to-many relationship." };
  if (kind === "many-to-many" && document.tables.length >= 4096)
    return { error: "The model already contains the maximum of 4096 tables." };
  const parents = kind === "many-to-many" ? [parent, child] : [parent];
  const keys: Column[][] = [];
  for (const table of parents) {
    const columns = referencedColumns(table);
    if (!columns)
      return {
        error: `${table.name} needs a complete visible PRIMARY or UNIQUE key before it can be referenced.`,
      };
    keys.push(columns);
  }
  if (kind === "many-to-many" && keys.flat().length > 16)
    return {
      error: "The junction primary key would exceed MySQL's 16-column index limit.",
    };
  if (kind !== "many-to-many" && child.columns.length + keys[0]!.length > 1017)
    return { error: `${child.name} would exceed MySQL's 1017-column table limit.` };

  // Work on a detached candidate until every preflight check has succeeded.
  const destination: Table =
    kind === "many-to-many"
      ? {
          id: id("table"),
          name: uniqueName(
            `${parent.name}_${child.name}`,
            document.tables.map((table) => table.name),
          ),
          engine: "InnoDB",
          position: {
            x: (parent.position.x + child.position.x) / 2,
            y:
              Math.max(
                parent.position.y + tableHeight(parent),
                child.position.y + tableHeight(child),
              ) + 96,
          },
          width: 304,
          columns: [],
          indexes: [],
          foreignKeys: [],
        }
      : {
          ...child,
          columns: [...child.columns],
          indexes: [...child.indexes],
          foreignKeys: [...child.foreignKeys],
        };
  const names = document.tables.flatMap((table) =>
    table.foreignKeys.map((key) => key.name),
  );
  for (let index = 0; index < parents.length; index++) {
    const source = parents[index]!;
    const key = keys[index]!;
    const added: Column[] = [];
    for (const original of key) {
      const options = textOptions(document, source, original);
      if (
        /^\s*(?:char|varchar|enum|set)\b/i.test(original.type) &&
        !options.charset &&
        !options.collation &&
        (destination.charset ||
          destination.collation ||
          document.charset ||
          document.collation)
      ) {
        return {
          error: `Set an explicit charset or collation on ${source.name}.${original.name} before referencing it from a table with different text defaults.`,
        };
      }
      const column: Column = {
        id: id("column"),
        name: uniqueName(
          `${source.name}_${original.name}`,
          destination.columns.map((item) => item.name),
        ),
        type: original.type,
        nullable: kind !== "many-to-many",
        ...options,
      };
      // A fresh value column must not inherit generation, defaults, AUTO_INCREMENT or invisibility.
      destination.columns.push(column);
      added.push(column);
    }
    const name = uniqueName(`fk_${destination.name}_${source.name}`, names);
    names.push(name);
    destination.foreignKeys.push({
      id: id("foreign"),
      name,
      columns: added.map((column) => column.id),
      referencedTable: source.id,
      referencedColumns: key.map((column) => column.id),
    });
    // The junction primary key covers the first side; the second side needs its own leading index.
    if (kind !== "many-to-many" || index > 0)
      destination.indexes.push({
        id: id("index"),
        name: uniqueName(
          `${kind === "one-to-one" ? "uq" : "idx"}_${source.name}`,
          destination.indexes.map((item) => item.name),
        ),
        kind: kind === "one-to-one" ? "UNIQUE" : "INDEX",
        columns: added.map((column) => ({ column: column.id })),
      });
  }
  if (kind === "many-to-many") {
    if (destination.columns.reduce((total, column) => total + keyBytes(column), 0) > 3072)
      return {
        error:
          "The combined junction key could exceed InnoDB's 3072-byte index limit. Use shorter parent keys.",
      };
    destination.indexes.unshift({
      id: id("index"),
      name: "PRIMARY",
      kind: "PRIMARY",
      columns: destination.columns.map((column) => ({ column: column.id })),
    });
    destination.width = recommendedTableWidth(destination);
    // Find clear space below the clicked tables without repositioning any existing content.
    while (true) {
      const overlapping = document.tables.find(
        (table) =>
          destination.position.x < table.position.x + table.width + 48 &&
          destination.position.x + destination.width + 48 > table.position.x &&
          destination.position.y < table.position.y + tableHeight(table) + 48 &&
          destination.position.y + tableHeight(destination) + 48 > table.position.y,
      );
      if (!overlapping) break;
      destination.position.y = overlapping.position.y + tableHeight(overlapping) + 48;
    }
    if (Math.abs(destination.position.y) > 1e7)
      return {
        error:
          "Move the selected tables closer to the canvas origin before adding a junction.",
      };
    destination.groupId = groupForTable(document, destination);
    document.tables.push(destination);
  } else {
    child.columns = destination.columns;
    child.indexes = destination.indexes;
    child.foreignKeys = destination.foreignKeys;
    child.width = Math.max(child.width, recommendedTableWidth(child));
  }
  return {
    tableId: destination.id,
    keyId:
      destination.foreignKeys[
        kind === "many-to-many" ? 0 : destination.foreignKeys.length - 1
      ]!.id,
  };
}
