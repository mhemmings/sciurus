import { readFile, readdir, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";
import { recommendedTableWidth, tableHeight } from "../src/editor/model.ts";
import type { ERDocument, Point } from "../src/editor/types.ts";

interface Rectangle {
  id: string;
  width: number;
  height: number;
}

interface Placement extends Rectangle {
  position: Point;
}

const tableGap = 80;
const groupGap = 144;
const groupPadding = 32;
const groupTop = 72;
const groupColors = [
  "#3b82f6",
  "#14b8a6",
  "#8b5cf6",
  "#f59e0b",
  "#f43f5e",
  "#22c55e",
  "#06b6d4",
  "#d946ef",
  "#f97316",
  "#84cc16",
  "#6366f1",
  "#ec4899",
];

/** Pack unequal rectangles onto shelves, comparing a fixed set of compact layouts. */
function pack(rectangles: Rectangle[], gap: number, aspect: number) {
  if (!rectangles.length) return { width: 0, height: 0, placements: [] as Placement[] };
  // Explicit string comparison avoids locale-dependent ordering on different hosts.
  const sorted = [...rectangles].sort(
    (a, b) =>
      b.height - a.height ||
      b.width - a.width ||
      (a.id < b.id ? -1 : a.id > b.id ? 1 : 0),
  );
  const widest = Math.max(...sorted.map((item) => item.width));
  const area = sorted.reduce(
    (sum, item) => sum + (item.width + gap) * (item.height + gap),
    0,
  );
  let best:
    | { width: number; height: number; placements: Placement[]; score: number }
    | undefined;
  // A bounded search gives reproducible compact groups without a random layout engine.
  for (const factor of [0.75, 0.9, 1, 1.15, 1.3, 1.5, 1.75, 2]) {
    const capacity = Math.max(widest, Math.ceil(Math.sqrt(area * aspect) * factor));
    const shelves: { y: number; height: number; used: number }[] = [];
    const placements: Placement[] = [];
    for (const rectangle of sorted) {
      // Heights are descending, so every remaining item fits an earlier shelf vertically.
      const available = shelves
        .filter((shelf) => shelf.used + gap + rectangle.width <= capacity)
        .sort((a, b) => b.used - a.used || a.y - b.y);
      let shelf = available[0];
      const x = shelf ? shelf.used + gap : 0;
      if (!shelf) {
        const previous = shelves.at(-1);
        shelf = {
          y: previous ? previous.y + previous.height + gap : 0,
          height: rectangle.height,
          used: 0,
        };
        shelves.push(shelf);
      }
      placements.push({ ...rectangle, position: { x, y: shelf.y } });
      shelf.used = x + rectangle.width;
    }
    const width = Math.max(...shelves.map((shelf) => shelf.used));
    const last = shelves.at(-1)!;
    const height = last.y + last.height;
    const score =
      width * height * (1 + 1.2 * Math.abs(Math.log(width / height / aspect)));
    if (!best || score < best.score) best = { width, height, placements, score };
  }
  return best!;
}

/** Regenerate only layout fields; schema names, IDs, ordering, and group membership stay intact. */
export function layoutExample(source: ERDocument): ERDocument {
  const document = structuredClone(source);
  const blocks: Rectangle[] = [];
  const interiors = new Map<string, ReturnType<typeof pack>>();
  for (const table of document.tables) {
    table.width = recommendedTableWidth(table);
    // Manual bends describe the old positions and must not survive a fresh arrangement.
    for (const key of table.foreignKeys) delete key.points;
  }
  for (const [index, group] of document.groups.entries()) {
    group.color = groupColors[index % groupColors.length]!;
    const members = document.tables.filter((table) => table.groupId === group.id);
    const interior = pack(
      members.map((table) => ({
        id: table.id,
        width: table.width,
        height: tableHeight(table),
      })),
      tableGap,
      1.4,
    );
    interiors.set(group.id, interior);
    group.width = Math.max(
      interior.width + 2 * groupPadding,
      64 + group.name.length * 9,
      240,
    );
    group.height = Math.max(interior.height + groupTop + groupPadding, 160);
    blocks.push({ id: group.id, width: group.width, height: group.height });
  }
  // Unassigned tables share an invisible packing region without introducing a fake model group.
  const ungrouped = document.tables.filter((table) => !table.groupId);
  const ungroupedID = "\u0000ungrouped";
  if (ungrouped.length) {
    const interior = pack(
      ungrouped.map((table) => ({
        id: table.id,
        width: table.width,
        height: tableHeight(table),
      })),
      tableGap,
      1.4,
    );
    interiors.set(ungroupedID, interior);
    blocks.push({ id: ungroupedID, width: interior.width, height: interior.height });
  }
  // Small diagrams should fill the almost-square canvas left between the two panels.
  // Larger models benefit from a wider overview and are navigated at reading zoom.
  const arranged = pack(blocks, groupGap, document.tables.length <= 8 ? 1 : 1.5);
  for (const block of arranged.placements) {
    const group = document.groups.find((item) => item.id === block.id);
    const position = { x: block.position.x + 80, y: block.position.y + 80 };
    if (group) group.position = position;
    const interior = interiors.get(block.id)!;
    for (const placement of interior.placements) {
      const table = document.tables.find((item) => item.id === placement.id)!;
      table.position = {
        x: position.x + placement.position.x + (group ? groupPadding : 0),
        y: position.y + placement.position.y + (group ? groupTop : 0),
      };
    }
  }
  // The editor fits examples on opening; retain a useful origin at ordinary reading zoom too.
  document.viewport = { x: 32, y: 32, zoom: 1 };
  assertLayout(document);
  return document;
}

/** Independently check every pair and group boundary against the actual card geometry. */
export function assertLayout(document: ERDocument): void {
  const tables = document.tables.map((table) => ({
    ...table,
    height: tableHeight(table),
  }));
  const separated = (a: Placement, b: Placement, gap = 0) =>
    a.position.x + a.width + gap <= b.position.x ||
    b.position.x + b.width + gap <= a.position.x ||
    a.position.y + a.height + gap <= b.position.y ||
    b.position.y + b.height + gap <= a.position.y;
  for (let i = 0; i < tables.length; i++) {
    const table = tables[i]!;
    if (table.width < recommendedTableWidth(table))
      throw new Error(`${table.name}: clipped names`);
    for (const other of tables.slice(i + 1)) {
      if (!separated(table, other, tableGap))
        throw new Error(`${table.name} overlaps ${other.name}`);
    }
    for (const group of document.groups) {
      if (table.groupId === group.id) {
        if (
          table.position.x < group.position.x + groupPadding ||
          table.position.y < group.position.y + groupTop ||
          table.position.x + table.width >
            group.position.x + group.width - groupPadding ||
          table.position.y + table.height > group.position.y + group.height - groupPadding
        )
          throw new Error(`${table.name} falls outside ${group.name}`);
      } else if (!separated(table, group))
        throw new Error(`${table.name} overlaps group ${group.name}`);
    }
    if (table.groupId && !document.groups.some((group) => group.id === table.groupId)) {
      throw new Error(`${table.name}: missing group ${table.groupId}`);
    }
  }
  for (let i = 0; i < document.groups.length; i++) {
    for (const other of document.groups.slice(i + 1)) {
      if (!separated(document.groups[i]!, other, groupGap)) {
        throw new Error(`Group ${document.groups[i]!.name} overlaps ${other.name}`);
      }
    }
  }
}

// Importing this file in the focused tests never writes examples.
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const check = process.argv.includes("--check");
  const directory = new URL("../../examples/", import.meta.url);
  for (const name of (await readdir(directory))
    .filter((name) => name.endsWith(".json"))
    .sort()) {
    const path = new URL(name, directory);
    const current = await readFile(path, "utf8");
    const document = JSON.parse(current) as ERDocument;
    const formatted = JSON.stringify(layoutExample(document), null, 2) + "\n";
    if (check) {
      assertLayout(document);
      if (current !== formatted)
        throw new Error(`${name}: regenerate with node scripts/layout-examples.ts`);
    } else await writeFile(path, formatted);
    console.log(
      `${name}: ${document.tables.length} tables, ${document.groups.length} groups; no overlaps`,
    );
  }
}
