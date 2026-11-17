import type { Column, ERDocument, Point, Table } from "./types";

/** Copy plain model data without retaining Vue proxies or nested references. */
export function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

/** Create opaque identity independently of an editable SQL name. */
export function id(prefix: string): string {
  return `${prefix}_${crypto.randomUUID()}`;
}

/** Start a version-one MySQL document with a comfortable initial viewport. */
export function newDocument(): ERDocument {
  return {
    version: 1,
    name: "Untitled schema",
    tables: [],
    groups: [],
    viewport: { x: 80, y: 80, zoom: 1 },
  };
}

/** Normalize optional collections returned by Go without changing schema meaning. */
export function normalize(document: ERDocument): ERDocument {
  const result = clone(document);
  result.tables ??= [];
  result.groups ??= [];
  for (const table of result.tables) {
    table.columns ??= [];
    table.indexes ??= [];
    table.foreignKeys ??= [];
    table.checks ??= [];
  }
  return result;
}

/** New columns intentionally have no default, which differs from DEFAULT NULL. */
export function newColumn(name: string): Column {
  return { id: id("column"), name, type: "VARCHAR(255)", nullable: true };
}

/** Seed a new table with a useful, valid auto-increment primary key. */
export function newTable(name: string, position: Point): Table {
  const column = {
    ...newColumn("id"),
    type: "BIGINT",
    nullable: false,
    autoIncrement: true,
  };
  return {
    id: id("table"),
    name,
    engine: "InnoDB",
    position,
    width: 304,
    columns: [column],
    indexes: [
      {
        id: id("index"),
        name: "PRIMARY",
        kind: "PRIMARY",
        columns: [{ column: column.id }],
      },
    ],
    foreignKeys: [],
    checks: [],
  };
}

/** Remove a table and foreign keys elsewhere that would otherwise dangle. */
export function removeTable(document: ERDocument, tableID: string): void {
  document.tables = document.tables.filter((table) => table.id !== tableID);
  for (const table of document.tables) {
    table.foreignKeys = table.foreignKeys.filter(
      (key) => key.referencedTable !== tableID,
    );
  }
}

/** Remove dependent whole constraints; silently shortening composite keys changes meaning. */
export function removeColumn(
  document: ERDocument,
  tableID: string,
  columnID: string,
): void {
  const owner = document.tables.find((table) => table.id === tableID);
  if (!owner) return;
  owner.columns = owner.columns.filter((column) => column.id !== columnID);
  owner.indexes = owner.indexes.filter(
    (index) => !index.columns.some((part) => part.column === columnID),
  );
  for (const table of document.tables) {
    table.foreignKeys = table.foreignKeys.filter(
      (key) =>
        !(table.id === tableID && key.columns.includes(columnID)) &&
        !(key.referencedTable === tableID && key.referencedColumns.includes(columnID)),
    );
  }
}

/** Translate absolute child coordinates and routes owned by the moved group together. */
export function moveGroup(document: ERDocument, groupID: string, delta: Point): void {
  const group = document.groups.find((item) => item.id === groupID);
  if (!group) return;
  group.position.x += delta.x;
  group.position.y += delta.y;
  for (const table of document.tables.filter((item) => item.groupId === groupID)) {
    table.position.x += delta.x;
    table.position.y += delta.y;
    for (const key of table.foreignKeys) {
      for (const point of key.points ?? []) {
        point.x += delta.x;
        point.y += delta.y;
      }
    }
  }
}

/** Table height follows content; width and position remain explicitly user-controlled. */
export function tableHeight(table: Table): number {
  return 42 + table.columns.length * 28;
}

/** Size cards for complete identifiers while bounding potentially enormous ENUM type labels.
 * This is an explicit edit, never applied during loading: stored geometry stays authoritative.
 * Eight pixels per ASCII character conservatively covers the 12px monospace canvas font;
 * wider Unicode glyphs receive twice that space. The reserve covers icons, gaps and padding.
 */
export function recommendedTableWidth(table: Table): number {
  const textWidth = (value: string): number =>
    [...value].reduce(
      (width, character) => width + (character.codePointAt(0)! < 128 ? 8 : 16),
      0,
    );
  const nameWidth = Math.max(0, ...table.columns.map((column) => textWidth(column.name)));
  const typeWidth = Math.min(
    176,
    Math.max(0, ...table.columns.map((column) => textWidth(column.type))),
  );
  return (
    Math.ceil(Math.max(300, 80 + textWidth(table.name), 68 + nameWidth + typeWidth) / 8) *
    8
  );
}

/** Display SQL type keywords consistently without altering case-sensitive ENUM/SET values. */
export function displayColumnType(type: string): string {
  let result = "";
  let quote = "";
  for (let index = 0; index < type.length; index++) {
    const character = type[index]!;
    if (quote) {
      result += character;
      if (character === "\\" && index + 1 < type.length) result += type[++index];
      else if (character === quote) {
        if (type[index + 1] === quote) result += type[++index];
        else quote = "";
      }
    } else if (character === "'" || character === '"' || character === "`") {
      quote = character;
      result += character;
    } else result += character.toUpperCase();
  }
  return result;
}

/** Assign a dropped card by its header center, preferring the innermost, last-painted group. */
export function groupForTable(document: ERDocument, table: Table): string | undefined {
  const x = table.position.x + table.width / 2;
  const y = table.position.y + 20;
  let selected: string | undefined;
  let area = Infinity;
  for (const group of document.groups) {
    if (
      x >= group.position.x &&
      x <= group.position.x + group.width &&
      y >= group.position.y + 40 &&
      y <= group.position.y + group.height &&
      group.width * group.height <= area
    ) {
      selected = group.id;
      area = group.width * group.height;
    }
  }
  return selected;
}

/** Compute content bounds for fit-to-screen and the miniature overview. */
export function bounds(document: ERDocument): {
  x: number;
  y: number;
  width: number;
  height: number;
} {
  const rectangles = [
    ...document.tables.map((table) => ({
      x: table.position.x,
      y: table.position.y,
      width: table.width,
      height: tableHeight(table),
    })),
    ...document.groups.map((group) => ({
      ...group.position,
      width: group.width,
      height: group.height,
    })),
  ];
  if (!rectangles.length) return { x: 0, y: 0, width: 800, height: 600 };
  const x = Math.min(...rectangles.map((item) => item.x));
  const y = Math.min(...rectangles.map((item) => item.y));
  return {
    x,
    y,
    width: Math.max(...rectangles.map((item) => item.x + item.width)) - x,
    height: Math.max(...rectangles.map((item) => item.y + item.height)) - y,
  };
}

/** Keep saved bends orthogonal as tables move and discard redundant points without collapsing loops. */
function orthogonalPath(points: Point[], horizontalEndpoints = false): Point[] {
  const result: Point[] = [];
  for (let index = 0; index < points.length; index++) {
    const next = { ...points[index]! };
    const previous = result.at(-1);
    if (previous && previous.x !== next.x && previous.y !== next.y) {
      const before = result.at(-2);
      if (
        horizontalEndpoints &&
        index === points.length - 1 &&
        before?.x === previous.x
      ) {
        // Give the final bend a visible dogleg instead of doubling back on itself.
        const middle = (previous.x + next.x) / 2;
        result.push({ x: middle, y: previous.y }, { x: middle, y: next.y });
      } else {
        const horizontal =
          horizontalEndpoints && index === points.length - 1
            ? false
            : !before || before.x === previous.x;
        result.push(
          horizontal ? { x: next.x, y: previous.y } : { x: previous.x, y: next.y },
        );
      }
    }
    if (!previous || previous.x !== next.x || previous.y !== next.y) result.push(next);
  }
  for (let index = 1; index < result.length - 1;) {
    const a = result[index - 1]!;
    const b = result[index]!;
    const c = result[index + 1]!;
    if (
      (a.x === b.x && b.x === c.x && (b.y - a.y) * (c.y - b.y) >= 0) ||
      (a.y === b.y && b.y === c.y && (b.x - a.x) * (c.x - b.x) >= 0)
    )
      result.splice(index, 1);
    else index++;
  }
  return result;
}

/** Connect column rows with right angles, adapting saved bends when tables move. */
export function relationshipPath(
  source: Table,
  target: Table,
  columnID: string,
  targetColumnID: string,
  points?: Point[],
): Point[] {
  const sourceY =
    source.position.y +
    55 +
    Math.max(
      0,
      source.columns.findIndex((column) => column.id === columnID),
    ) *
      28;
  const targetY =
    target.position.y +
    55 +
    Math.max(
      0,
      target.columns.findIndex((column) => column.id === targetColumnID),
    ) *
      28;
  const toRight =
    target.position.x + target.width / 2 >= source.position.x + source.width / 2;
  // Bends may cross either card while being dragged. Attachment sides depend only
  // on the tables, so persisting those bends cannot move a column endpoint.
  const sourceRight = source.id === target.id || toRight;
  const targetRight = source.id === target.id || !toRight;
  const start = { x: source.position.x + (sourceRight ? source.width : 0), y: sourceY };
  const end = { x: target.position.x + (targetRight ? target.width : 0), y: targetY };
  if (points?.length) {
    const route = orthogonalPath([start, ...points, end], true);
    if (
      source.id !== target.id ||
      (new Set(route.map((point) => point.x)).size > 1 &&
        new Set(route.map((point) => point.y)).size > 1)
    )
      return route;
  }
  if (source.id === target.id) {
    start.x = source.position.x + source.width;
    end.x = start.x;
    const top = Math.min(start.y, end.y) - 42;
    return [
      start,
      { x: start.x + 56, y: start.y },
      { x: start.x + 56, y: top },
      { x: start.x + 28, y: top },
      { x: start.x + 28, y: end.y },
      end,
    ];
  }
  const middle = (start.x + end.x) / 2;
  return orthogonalPath([start, { x: middle, y: start.y }, { x: middle, y: end.y }, end]);
}

/** Move one segment perpendicular to itself while keeping both column endpoints fixed.
 * Call repeatedly with the path captured on pointer-down, so inserted doglegs do not change
 * the segment index during a drag. The returned interior points can be saved directly.
 */
export function moveRouteSegment(
  path: Point[],
  segmentIndex: number,
  point: Point,
): Point[] {
  const result = path.map((item) => ({ ...item }));
  if (
    segmentIndex < 0 ||
    segmentIndex >= result.length - 1 ||
    !Number.isInteger(segmentIndex)
  )
    return result;
  const a = path[segmentIndex]!;
  const b = path[segmentIndex + 1]!;
  const horizontal = a.y === b.y;
  if ((!horizontal && a.x !== b.x) || (a.x === b.x && a.y === b.y)) return result;
  const coordinate = horizontal ? "y" : "x";
  const along = horizontal ? "x" : "y";
  const value = point[coordinate];
  if (!Number.isFinite(value) || value === a[coordinate]) return result;
  result[segmentIndex]![coordinate] = value;
  result[segmentIndex + 1]![coordinate] = value;
  const length = b[along] - a[along];
  const stub = Math.sign(length) * Math.min(32, Math.abs(length) / 3);
  if (segmentIndex + 1 === path.length - 1) {
    const turn = { ...result[segmentIndex + 1]! };
    turn[along] -= stub;
    const attachment = { ...b };
    attachment[along] -= stub;
    result.splice(segmentIndex + 1, 1, turn, attachment, { ...b });
  }
  if (segmentIndex === 0) {
    const attachment = { ...a };
    attachment[along] += stub;
    const turn = { ...result[0]! };
    turn[along] += stub;
    result.splice(0, 1, { ...a }, attachment, turn);
  }
  return orthogonalPath(result);
}
