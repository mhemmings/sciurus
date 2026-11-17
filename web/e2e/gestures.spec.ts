import { expect, test } from "@playwright/test";
import type { Locator, Page } from "@playwright/test";
import type { ERDocument, Point, Table } from "../src/editor/types";

/** Fixtures use the public portable format, so setup follows the normal file boundary. */
function fixture(tables: Table[] = []): ERDocument {
  return {
    version: 1,
    name: "Gesture tests",
    tables,
    groups: [],
    viewport: { x: 35, y: 45, zoom: 0.75 },
  };
}

/** Separate identifiers and primary-key types make relationship inference observable. */
function table(id: string, position: Point): Table {
  return {
    id,
    name: id,
    position,
    width: 240,
    columns: [
      {
        id: `${id}_id`,
        name: "id",
        type: "BIGINT unsigned",
        nullable: false,
        autoIncrement: true,
      },
    ],
    indexes: [
      {
        id: `${id}_primary`,
        name: "PRIMARY",
        kind: "PRIMARY",
        columns: [{ column: `${id}_id` }],
      },
    ],
    foreignKeys: [],
  };
}

/** Open the model editor just as a user would; never reach into Vue component state. */
async function modelEditor(page: Page): Promise<Locator> {
  await page
    .getByRole("banner")
    .getByRole("button", { name: "File", exact: true })
    .click();
  await page
    .locator(".dropdown")
    .getByRole("button", { name: "Edit model JSON", exact: true })
    .click();
  return page.getByRole("textbox", { name: "Model JSON source" });
}

async function readModel(page: Page): Promise<ERDocument> {
  const source = await modelEditor(page);
  const result = JSON.parse(await source.inputValue()) as ERDocument;
  await page.getByRole("button", { name: "Close dialog" }).click();
  return result;
}

/** Applying JSON is also the supported way to arrange exact regression fixtures. */
async function applyModel(page: Page, document: ERDocument) {
  const source = await modelEditor(page);
  await source.fill(JSON.stringify(document));
  await page.getByRole("button", { name: "Validate & apply" }).click();
  await expect(source).toHaveCount(0);
  await expect(page.locator(".diagram-table")).toHaveCount(document.tables.length);
}

async function openModel(page: Page, document: ERDocument) {
  await page.locator('input[type="file"]').setInputFiles({
    name: "gestures.json",
    mimeType: "application/json",
    buffer: Buffer.from(JSON.stringify(document)),
  });
  await expect(page.locator(".operation-shield")).toHaveCount(0);
  await expect(page.locator(".diagram-table")).toHaveCount(document.tables.length);
  // A project name assertion also waits for asynchronous opening of empty models.
  expect((await readModel(page)).name).toBe(document.name);
}

/** Pointer movements operate in CSS pixels; persisted positions use diagram units. */
async function dragHeader(page: Page, id: string, dx: number, dy: number) {
  const box = (await page
    .locator(`[data-table-id="${id}"] .table-header`)
    .boundingBox())!;
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + dx, box.y + box.height / 2 + dy, {
    steps: 6,
  });
  await page.mouse.up();
}

/** Keep the pointer held so tests can observe Sortable's placeholder before committing. */
async function beginColumnDrag(page: Page, name: string, target: Locator, after = false) {
  const source = (await page
    .getByRole("button", {
      name: `Reorder column ${name}`,
      exact: true,
    })
    .boundingBox())!;
  const destination = (await target.boundingBox())!;
  await page.mouse.move(source.x + source.width / 2, source.y + source.height / 2);
  await page.mouse.down();
  await page.mouse.move(
    source.x + source.width / 2 + 8,
    source.y + source.height / 2 + 8,
    { steps: 3 },
  );
  await expect(page.locator(".column-sortable-ghost")).toBeVisible();
  await expect(page.locator(".column-sortable-fallback")).toBeVisible();
  await page.mouse.move(
    source.x + source.width / 2,
    after ? destination.y + destination.height - 4 : destination.y + 4,
    { steps: 12 },
  );
}

/** Inspect the rendered polyline so a diagonal route cannot hide behind correct JSON. */
async function rightAngleRoute(page: Page, id: string): Promise<Point[]> {
  const raw = await page
    .locator(`.relationship-hit[data-relationship-id="${id}"]`)
    .getAttribute("points");
  const points = raw!
    .trim()
    .split(/\s+/)
    .map((pair) => {
      const [x, y] = pair.split(",").map(Number);
      return { x: x!, y: y! };
    });
  expect(points.length).toBeGreaterThanOrEqual(2);
  for (let index = 1; index < points.length; index++) {
    const start = points[index - 1]!;
    const end = points[index]!;
    expect(
      Math.min(Math.abs(start.x - end.x), Math.abs(start.y - end.y)),
      `route segment ${index - 1} must be horizontal or vertical`,
    ).toBeLessThan(0.01);
  }
  return points;
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "showSaveFilePicker", {
      value: undefined,
      configurable: true,
    });
    Object.defineProperty(window, "showOpenFilePicker", {
      value: undefined,
      configurable: true,
    });
  });
  await page.goto("/");
  await expect(page.locator(".canvas-loading")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Table", exact: true })).toBeVisible();
});

test("table and group placement respect the saved zoom and Escape leaves the model intact", async ({
  page,
}) => {
  const initial = fixture();
  initial.viewport = { x: 80, y: 60, zoom: 0.5 };
  await openModel(page, initial);
  const canvas = (await page
    .getByLabel("ER diagram canvas", { exact: true })
    .boundingBox())!;
  const tool = page.getByRole("button", { name: "Table", exact: true });
  await tool.click();
  await expect(tool).toHaveAttribute("aria-pressed", "true");
  await expect(page.locator(".diagram-table")).toHaveCount(0);
  await page.mouse.move(canvas.x + 240, canvas.y + 160);
  await expect(page.locator(".placement-preview")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(tool).toHaveAttribute("aria-pressed", "false");
  await expect(page.locator(".placement-preview")).toHaveCount(0);
  expect(await readModel(page)).toEqual(initial);

  await tool.click();
  await page.mouse.click(canvas.x + 240, canvas.y + 160);
  await expect(page.locator(".diagram-table")).toHaveCount(1);
  await expect(tool).toHaveAttribute("aria-pressed", "false");
  const placed = await readModel(page);
  expect(placed.tables[0]!.position.x).toBeCloseTo((240 - 80) / 0.5, 0);
  expect(placed.tables[0]!.position.y).toBeCloseTo((160 - 60) / 0.5, 0);
  await expect(
    page.getByRole("spinbutton", { name: /^(X|Y|Width|Height)$/ }),
  ).toHaveCount(0);

  const groupTool = page.getByRole("button", { name: "Group", exact: true });
  await groupTool.click();
  await expect(page.locator(".diagram-group")).toHaveCount(0);
  await page.keyboard.press("Escape");
  expect(await readModel(page)).toEqual(placed);
  await groupTool.click();
  await page.mouse.click(canvas.x + 410, canvas.y + 350);
  await expect(page.locator(".diagram-group")).toHaveCount(1);
  const grouped = await readModel(page);
  expect(grouped.groups[0]!.position.x).toBeCloseTo((410 - 80) / 0.5, 0);
  expect(grouped.groups[0]!.position.y).toBeCloseTo((350 - 60) / 0.5, 0);
  await expect(
    page.getByRole("spinbutton", { name: /^(X|Y|Width|Height)$/ }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Undo", exact: true }).click();
  expect((await readModel(page)).groups).toEqual([]);
});

test("dragging a table into and out of a group updates membership and undo restores group movement", async ({
  page,
}) => {
  const initial = fixture([table("member", { x: 620, y: 170 })]);
  initial.groups = [
    {
      id: "domain",
      name: "Domain",
      color: "#b9d7f0",
      position: { x: 20, y: 40 },
      width: 470,
      height: 400,
    },
  ];
  await openModel(page, initial);
  await dragHeader(page, "member", -450 * initial.viewport.zoom, 0);
  const inside = await readModel(page);
  expect(inside.tables[0]!.groupId).toBe("domain");
  expect(inside.tables[0]!.position.x).toBeCloseTo(170);

  // Group membership is decided by the header center, not just any intersecting corner.
  await dragHeader(page, "member", 280 * initial.viewport.zoom, 0);
  const outside = await readModel(page);
  expect(outside.tables[0]!.groupId).toBeFalsy();
  await page.getByRole("button", { name: "Undo", exact: true }).click();
  expect((await readModel(page)).tables).toEqual(inside.tables);

  const header = (await page.locator(".group-drag-handle").boundingBox())!;
  await page.mouse.move(header.x + 45, header.y + 15);
  await page.mouse.down();
  await page.mouse.move(header.x + 90, header.y + 45, { steps: 6 });
  await page.mouse.up();
  const shifted = await readModel(page);
  expect(shifted.groups[0]!.position.x - inside.groups[0]!.position.x).toBeCloseTo(60);
  expect(shifted.groups[0]!.position.y - inside.groups[0]!.position.y).toBeCloseTo(40);
  expect(shifted.tables[0]!.position.x - inside.tables[0]!.position.x).toBeCloseTo(60);
  expect(shifted.tables[0]!.position.y - inside.tables[0]!.position.y).toBeCloseTo(40);
  expect(shifted.tables[0]!.groupId).toBe("domain");
  await page.getByRole("button", { name: "Undo", exact: true }).click();
  const undone = await readModel(page);
  expect(undone.tables).toEqual(inside.tables);
  expect(undone.groups).toEqual(inside.groups);
});

test("keyboard undo after dragging leaves the focused inspector edit intact", async ({
  page,
}) => {
  const initial = fixture([table("movable", { x: 140, y: 120 })]);
  await openModel(page, initial);
  const card = page.locator('[data-table-id="movable"]');
  await card.locator(".table-header").click();
  const name = page.getByRole("textbox", { name: "Table name", exact: true });
  await name.fill("renamed_before_drag");
  await expect(name).toBeFocused();
  const before = (await card.boundingBox())!;
  await dragHeader(page, "movable", 60, 45);
  const dragged = (await card.boundingBox())!;
  expect(dragged.x - before.x).toBeCloseTo(60);
  expect(dragged.y - before.y).toBeCloseTo(45);
  // Do not open a modal or click another control before this shortcut: either
  // action would hide a stale input focus left behind by the canvas gesture.
  await expect(page.getByLabel("ER diagram canvas", { exact: true })).toBeFocused();
  await page.keyboard.press("ControlOrMeta+z");
  await expect(name).toHaveValue("renamed_before_drag");
  const undone = await readModel(page);
  expect(undone.tables[0]!.position).toEqual(initial.tables[0]!.position);
  expect(undone.tables[0]!.name).toBe("renamed_before_drag");
});

test("an explicit column selection does not reopen that column on a different table", async ({
  page,
}) => {
  const first = table("first", { x: 40, y: 80 });
  const second = table("second", { x: 610, y: 360 });
  for (const item of [first, second]) {
    // Column IDs are table-local: both files and imported SQL legitimately reuse them.
    item.columns.push({ id: "shared", name: "shared", type: "INT", nullable: true });
  }
  await openModel(page, fixture([first, second]));
  await page
    .locator('[data-table-id="first"] .table-column')
    .filter({
      has: page.locator(".column-name", { hasText: /^shared$/ }),
    })
    .dblclick();
  await expect(
    page.locator('details.column-card[data-column-id="shared"]'),
  ).toHaveAttribute("open", "");
  const tableTab = page.getByRole("tab", { name: "Table", exact: true });
  await tableTab.click();
  await page.locator('[data-table-id="second"] .table-header').click();
  await expect(tableTab).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("tab", { name: /^Columns/ })).toHaveAttribute(
    "aria-selected",
    "false",
  );
  await expect(
    page.getByRole("textbox", { name: "Table name", exact: true }),
  ).toHaveValue("second");
});

for (const relationship of [
  { name: "One-to-one relationship", unique: true },
  { name: "One-to-many relationship", unique: false },
  { name: "Many-to-many relationship", junction: true },
]) {
  test(`${relationship.name} creates valid keys and can be edited from the table`, async ({
    page,
  }) => {
    const initial = fixture([
      table("parent", { x: 40, y: 80 }),
      table("child", { x: 610, y: 360 }),
    ]);
    await openModel(page, initial);
    // A junction exercises the closed-inspector path as well as the ordinary
    // open-inspector joins: creating a relationship must preserve the view.
    if (relationship.junction)
      await page.getByRole("button", { name: "Close inspector" }).click();
    const canvas = page.getByLabel("ER diagram canvas", { exact: true });
    const canvasBefore = await canvas.boundingBox();
    const button = page.getByRole("button", { name: relationship.name, exact: true });
    await button.click();
    await expect(button).toHaveAttribute("aria-pressed", "true");
    await page.locator('[data-table-id="parent"] .table-header').click();
    // Choosing the first endpoint must not prematurely mutate the schema.
    await expect(page.locator(".relationship-hit")).toHaveCount(0);
    await page.locator('[data-table-id="child"] .table-header').click();
    await expect(button).toHaveAttribute("aria-pressed", "false");
    await expect.poll(() => canvas.boundingBox()).toEqual(canvasBefore);
    if (relationship.junction) {
      await expect(page.getByRole("complementary", { name: "Inspector" })).toHaveCount(0);
      await expect(
        page.getByRole("button", { name: "Toggle inspector" }),
      ).toHaveAttribute("aria-expanded", "false");
    }
    const created = await readModel(page);
    expect(created.viewport).toEqual(initial.viewport);
    const source = relationship.junction
      ? created.tables.find((item) => !["parent", "child"].includes(item.id))!
      : created.tables.find((item) => item.id === "child")!;
    expect(created.tables).toHaveLength(relationship.junction ? 3 : 2);
    expect(source.foreignKeys).toHaveLength(relationship.junction ? 2 : 1);
    const key = source.foreignKeys.find((item) => item.referencedTable === "parent")!;
    expect(key).toBeDefined();
    expect(key.referencedColumns).toEqual(["parent_id"]);
    const childColumn = source.columns.find((column) => column.id === key.columns[0])!;
    expect(childColumn.type.toUpperCase()).toBe("BIGINT UNSIGNED");
    expect(childColumn.autoIncrement).toBeFalsy();
    expect(
      source.indexes.some((index) => index.columns[0]?.column === childColumn.id),
    ).toBe(true);

    if (relationship.junction) {
      expect(source.foreignKeys.map((item) => item.referencedTable).sort()).toEqual([
        "child",
        "parent",
      ]);
      const primary = source.indexes.find((index) => index.kind === "PRIMARY")!;
      expect(primary.columns.map((part) => part.column).sort()).toEqual(
        source.foreignKeys.flatMap((item) => item.columns).sort(),
      );
      expect(
        source.columns.every((column) => !column.nullable && !column.autoIncrement),
      ).toBe(true);
    } else {
      expect(
        source.indexes.some(
          (index) =>
            ["PRIMARY", "UNIQUE"].includes(index.kind) &&
            index.columns.length === key.columns.length &&
            index.columns.every((part, index) => part.column === key.columns[index]),
        ),
      ).toBe(relationship.unique);
    }

    // The FK indicator should open the table's actual foreign-key editor directly.
    if (relationship.junction)
      await page.getByRole("button", { name: "Toggle inspector" }).click();
    await page
      .locator(`[data-table-id="${source.id}"]`)
      .getByRole("button", {
        name: `Edit foreign key for ${childColumn.name}`,
        exact: true,
      })
      .click();
    await expect(page.getByRole("tab", { name: /^Foreign keys/ })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    const card = page.locator(`[data-foreign-key-id="${key.id}"]`);
    await expect(card).toHaveAttribute("open", "");
    await card
      .getByRole("textbox", { name: "Foreign key name" })
      .fill("fk_edited_from_table");
    await card
      .getByRole("combobox", { name: "On delete", exact: true })
      .selectOption("CASCADE");
    await page.getByRole("button", { name: "SQL preview" }).click();
    await expect(page.locator(".sql-code")).toContainText(
      "CONSTRAINT `fk_edited_from_table`",
    );
    await expect(page.locator(".sql-code")).toContainText("ON DELETE CASCADE");
    await expect(page.locator(".sql-code")).toContainText("REFERENCES `parent` (`id`)");
    await expect(page.locator(".sql-code")).toContainText(/BIGINT UNSIGNED/i);
  });
}

test("double-click editing, column drag order, and primary-key controls produce the edited SQL", async ({
  page,
}) => {
  const editable = table("editable", { x: 140, y: 120 });
  editable.width = 320;
  editable.columns.push(
    { id: "title", name: "title", type: "varchar(80)", nullable: true },
    { id: "score", name: "score", type: "int", nullable: true, default: "NULL" },
  );
  await openModel(page, fixture([editable]));
  const card = page.locator('[data-table-id="editable"]');
  await expect(card.locator(".column-type")).toHaveText([
    "BIGINT UNSIGNED",
    "VARCHAR(80)",
    "INT",
  ]);
  await card.locator(".table-header strong").dblclick();
  const tableName = page.getByRole("textbox", { name: "Table name", exact: true });
  await expect(tableName).toBeFocused();
  await tableName.fill("ranked_item");
  await card
    .locator(".table-column")
    .filter({
      has: page.locator(".column-name", { hasText: /^title$/ }),
    })
    .dblclick();
  const title = page.locator('details.column-card[data-column-id="title"]');
  await expect(title).toHaveAttribute("open", "");
  await expect(
    title.getByRole("textbox", { name: "Column name", exact: true }),
  ).toHaveValue("title");
  const columnType = title.getByRole("combobox", { name: "Column type", exact: true });
  await columnType.focus();
  await columnType.evaluate((input: HTMLInputElement) => input.setSelectionRange(0, 0));
  await columnType.pressSequentially("bi");
  expect((await columnType.inputValue()).toUpperCase()).toBe("BIVARCHAR(80)");
  expect(
    await columnType.evaluate((input: HTMLInputElement) => input.selectionStart),
  ).toBe(2);
  // Restore a valid SQL type before continuing: this probes caret behavior,
  // not acceptance of the deliberately incomplete identifier typed above.
  await columnType.fill("VARCHAR(80)");
  const advanced = title.locator("details.column-advanced");
  await expect(advanced).not.toHaveAttribute("open", "");
  await expect(
    title.getByRole("textbox", { name: "Comment", exact: true }),
  ).not.toBeVisible();
  await advanced.locator("summary").click();
  await title
    .getByRole("textbox", { name: "Comment", exact: true })
    .fill("Readable label");
  await title.locator(":scope > summary").click();

  await card
    .locator(".table-column")
    .filter({
      has: page.locator(".column-name", { hasText: /^score$/ }),
    })
    .dblclick();
  const score = page.locator('details.column-card[data-column-id="score"]');
  await expect(score).toHaveAttribute("open", "");
  const beforeRejectedPrimary = await readModel(page);
  await score.getByRole("checkbox", { name: "Primary key", exact: true }).click();
  await expect(score.getByRole("alert")).toContainText(
    "Remove the explicit NULL default",
  );
  await expect(
    score.getByRole("checkbox", { name: "Primary key", exact: true }),
  ).not.toBeChecked();
  expect(await readModel(page)).toEqual(beforeRejectedPrimary);
  await score.getByRole("checkbox", { name: "Explicit default", exact: true }).uncheck();
  await score.getByRole("checkbox", { name: "Primary key", exact: true }).check();
  await expect(
    score.getByRole("checkbox", { name: "Nullable", exact: true }),
  ).not.toBeChecked();
  await score.locator(":scope > summary").click();
  await beginColumnDrag(page, "score", title.locator(":scope > summary"));
  await page.mouse.up();
  await expect(page.locator(".column-sortable-ghost")).toHaveCount(0);
  const edited = (await readModel(page)).tables[0]!;
  expect(edited.columns.map((column) => column.name)).toEqual(["id", "score", "title"]);
  expect(
    edited.indexes
      .find((index) => index.kind === "PRIMARY")!
      .columns.map((part) => part.column),
  ).toEqual(["editable_id", "score"]);
  expect(edited.columns.find((column) => column.id === "title")!.comment).toBe(
    "Readable label",
  );
  // Removing the only leading index from an AUTO_INCREMENT column would make
  // the schema invalid. The guard must preserve the model, not silently repair it.
  await card
    .locator(".table-column")
    .filter({
      has: page.locator(".column-name", { hasText: /^id$/ }),
    })
    .dblclick();
  const identity = page.locator('details.column-card[data-column-id="editable_id"]');
  const beforeRejectedUnindex = await readModel(page);
  await identity.getByRole("checkbox", { name: "Primary key", exact: true }).click();
  await expect(identity.getByRole("alert")).toContainText(
    "Keep an index beginning with id",
  );
  await expect(
    identity.getByRole("checkbox", { name: "Primary key", exact: true }),
  ).toBeChecked();
  expect(await readModel(page)).toEqual(beforeRejectedUnindex);
  await page.getByRole("button", { name: "SQL preview" }).click();
  const sql = page.locator(".sql-code");
  await expect(sql).toContainText("CREATE TABLE `ranked_item`");
  await expect(sql).toContainText("PRIMARY KEY (`id`, `score`)");
  await expect(sql).toContainText("COMMENT 'Readable label'");
  const generated = await sql.textContent();
  expect(generated!.search(/`score` INT NOT NULL/i)).toBeGreaterThan(-1);
  expect(generated!.search(/`score` INT NOT NULL/i)).toBeLessThan(
    generated!.search(/`title` VARCHAR\(80\)/i),
  );
});

test("pointer column sorting previews before drop and preserves expansion and undo in both directions", async ({
  page,
}) => {
  const editable = table("sortable", { x: 140, y: 120 });
  editable.width = 320;
  for (const name of ["alpha", "beta", "gamma"]) {
    editable.columns.push({ id: name, name, type: "VARCHAR(80)", nullable: true });
  }
  const other = table("other", { x: 600, y: 400 });
  other.columns.push(...editable.columns.slice(1).map((column) => ({ ...column })));
  await openModel(page, fixture([editable, other]));
  const original = await readModel(page);
  const tableCard = page.locator('[data-table-id="sortable"]');
  const canvasNames = tableCard.locator(".column-name");
  await tableCard
    .locator(".table-column")
    .filter({
      has: page.locator(".column-name", { hasText: /^gamma$/ }),
    })
    .dblclick();
  const inspector = page.getByRole("complementary", { name: "Inspector" });
  const columns = inspector.locator("details.column-card:not(.column-sortable-fallback)");
  const inspectorOrder = async () =>
    columns.evaluateAll((items) =>
      items.map((item) => (item as HTMLElement).dataset.columnId),
    );
  const gamma = inspector.locator(
    'details.column-card[data-column-id="gamma"]:not(.column-sortable-fallback)',
  );
  const alpha = inspector.locator('details.column-card[data-column-id="alpha"]');
  const beta = inspector.locator('details.column-card[data-column-id="beta"]');
  await expect(gamma).toHaveAttribute("open", "");

  // Text-selection gestures belong to the form input. Sortable may only start
  // from its six-dot handle, even inside an expanded draggable column card.
  const nameInput = gamma.getByRole("textbox", { name: "Column name", exact: true });
  const inputBounds = (await nameInput.boundingBox())!;
  await page.mouse.move(inputBounds.x + 12, inputBounds.y + inputBounds.height / 2);
  await page.mouse.down();
  await page.mouse.move(inputBounds.x + 80, inputBounds.y + inputBounds.height / 2, {
    steps: 6,
  });
  await expect(
    page.locator(".column-sortable-ghost, .column-sortable-fallback"),
  ).toHaveCount(0);
  await page.mouse.up();
  await expect(nameInput).toBeFocused();
  expect(
    await nameInput.evaluate(
      (input: HTMLInputElement) =>
        (input.selectionEnd ?? 0) - (input.selectionStart ?? 0),
    ),
  ).toBeGreaterThan(0);
  expect(await readModel(page)).toEqual(original);

  // The list is unmounted outside Columns. Returning must bind a fresh Sortable
  // instance to the live element without losing which column was expanded.
  await page.getByRole("tab", { name: /^Keys/ }).click();
  await expect(columns).toHaveCount(0);
  await page.getByRole("tab", { name: /^Columns/ }).click();
  await expect(gamma).toHaveAttribute("open", "");

  await beginColumnDrag(page, "gamma", alpha.locator(":scope > summary"));
  await expect.poll(inspectorOrder).toEqual(["sortable_id", "gamma", "alpha", "beta"]);
  // Sortable can rearrange its DOM preview, but the model-backed canvas and
  // dirty state must remain untouched until the pointer is released.
  await expect(canvasNames).toHaveText(["id", "alpha", "beta", "gamma"]);
  await expect(page.getByLabel("Unsaved changes", { exact: true })).toHaveCount(0);
  await expect(page.locator(".column-sortable-ghost")).toHaveAttribute(
    "data-column-id",
    "gamma",
  );
  await page.mouse.up();
  await expect(
    page.locator(".column-sortable-ghost, .column-sortable-fallback"),
  ).toHaveCount(0);
  await expect(canvasNames).toHaveText(["id", "gamma", "alpha", "beta"]);
  await expect(gamma).toHaveAttribute("open", "");
  const movedUp = await readModel(page);
  expect(movedUp.tables[0]!.columns.map((column) => column.id)).toEqual([
    "sortable_id",
    "gamma",
    "alpha",
    "beta",
  ]);

  await page.getByRole("button", { name: "Undo", exact: true }).click();
  expect(await readModel(page)).toEqual(original);
  await expect(canvasNames).toHaveText(["id", "alpha", "beta", "gamma"]);
  await expect.poll(inspectorOrder).toEqual(["sortable_id", "alpha", "beta", "gamma"]);
  await expect(gamma).toHaveAttribute("open", "");
  await page.getByRole("button", { name: "Redo", exact: true }).click();
  expect(await readModel(page)).toEqual(movedUp);
  await expect(canvasNames).toHaveText(["id", "gamma", "alpha", "beta"]);
  await expect.poll(inspectorOrder).toEqual(["sortable_id", "gamma", "alpha", "beta"]);

  await beginColumnDrag(page, "gamma", beta.locator(":scope > summary"), true);
  await expect.poll(inspectorOrder).toEqual(["sortable_id", "alpha", "beta", "gamma"]);
  await expect(canvasNames).toHaveText(["id", "gamma", "alpha", "beta"]);
  await page.mouse.up();
  await expect(
    page.locator(".column-sortable-ghost, .column-sortable-fallback"),
  ).toHaveCount(0);
  await expect(canvasNames).toHaveText(["id", "alpha", "beta", "gamma"]);
  await expect(gamma).toHaveAttribute("open", "");
  expect(await readModel(page)).toEqual(original);
  await page.getByRole("button", { name: "Undo", exact: true }).click();
  expect(await readModel(page)).toEqual(movedUp);
  await expect(canvasNames).toHaveText(["id", "gamma", "alpha", "beta"]);
  await expect.poll(inspectorOrder).toEqual(["sortable_id", "gamma", "alpha", "beta"]);
  await page.getByRole("button", { name: "Redo", exact: true }).click();
  expect(await readModel(page)).toEqual(original);
  await expect(canvasNames).toHaveText(["id", "alpha", "beta", "gamma"]);
  await expect.poll(inspectorOrder).toEqual(["sortable_id", "alpha", "beta", "gamma"]);

  // A second table deliberately reuses local column IDs. The list's new
  // Sortable instance must commit to that table, never its previous draft.
  const otherCard = page.locator('[data-table-id="other"]');
  await otherCard
    .locator(".table-column")
    .filter({
      has: page.locator(".column-name", { hasText: /^gamma$/ }),
    })
    .dblclick();
  await beginColumnDrag(page, "gamma", alpha.locator(":scope > summary"));
  await page.mouse.up();
  await expect(otherCard.locator(".column-name")).toHaveText([
    "id",
    "gamma",
    "alpha",
    "beta",
  ]);
  await expect(canvasNames).toHaveText(["id", "alpha", "beta", "gamma"]);
  const changedOther = await readModel(page);
  expect(changedOther.tables[0]).toEqual(original.tables[0]);
  expect(changedOther.tables[1]!.columns.map((column) => column.id)).toEqual([
    "other_id",
    "gamma",
    "alpha",
    "beta",
  ]);
  await page.getByRole("button", { name: "Undo", exact: true }).click();
  expect(await readModel(page)).toEqual(original);
  await expect(otherCard.locator(".column-name")).toHaveText([
    "id",
    "alpha",
    "beta",
    "gamma",
  ]);
  await expect.poll(inspectorOrder).toEqual(["other_id", "alpha", "beta", "gamma"]);
});

test("orthogonal segments drag above overlapping cards and persist after table movement and file reload", async ({
  page,
}) => {
  const parent = table("parent", { x: 40, y: 80 });
  const child = table("child", { x: 610, y: 360 });
  child.columns.push({
    id: "parent_ref",
    name: "parent_id",
    type: "BIGINT unsigned",
    nullable: false,
  });
  child.indexes.push({
    id: "parent_index",
    name: "parent_index",
    kind: "INDEX",
    columns: [{ column: "parent_ref" }],
  });
  child.foreignKeys.push({
    id: "child_parent",
    name: "child_parent",
    columns: ["parent_ref"],
    referencedTable: "parent",
    referencedColumns: ["parent_id"],
  });
  const initial = fixture([parent, child, table("obstacle", { x: 50, y: 650 })]);
  await openModel(page, initial);
  const originalRoute = await rightAngleRoute(page, "child_parent");
  // Choose a real, non-terminal route segment; its center is clear of both endpoints.
  const segmentIndex = originalRoute
    .slice(0, -1)
    .findIndex(
      (point, index) =>
        index > 0 &&
        index < originalRoute.length - 2 &&
        Math.hypot(
          originalRoute[index + 1]!.x - point.x,
          originalRoute[index + 1]!.y - point.y,
        ) > 40,
    );
  expect(segmentIndex).toBeGreaterThan(0);
  const start = originalRoute[segmentIndex]!;
  const end = originalRoute[segmentIndex + 1]!;
  const midpoint = { x: (start.x + end.x) / 2, y: (start.y + end.y) / 2 };

  // Put an unrelated card underneath the segment using portable model editing.
  // Selected route handles must remain above cards, including their header hit areas.
  initial.tables[2]!.position = { x: midpoint.x - 120, y: midpoint.y - 20 };
  await applyModel(page, initial);
  const canvas = (await page
    .getByLabel("ER diagram canvas", { exact: true })
    .boundingBox())!;
  const exposed = {
    x: (originalRoute[0]!.x + originalRoute[1]!.x) / 2,
    y: (originalRoute[0]!.y + originalRoute[1]!.y) / 2,
  };
  await page.mouse.click(
    canvas.x + initial.viewport.x + exposed.x * initial.viewport.zoom,
    canvas.y + initial.viewport.y + exposed.y * initial.viewport.zoom,
  );
  const handle = page.locator(
    `.route-segment-handle[data-segment-index="${segmentIndex}"]`,
  );
  await expect(handle).toBeVisible();
  const bounds = (await handle.boundingBox())!;
  const center = { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
  expect(
    await page.evaluate(({ x, y }) => {
      const element = document.elementFromPoint(x, y);
      return element?.matches(".route-segment-hit, .route-segment-handle") ?? false;
    }, center),
  ).toBe(true);
  const horizontal = start.y === end.y;
  await page.mouse.move(center.x, center.y);
  await page.mouse.down();
  await page.mouse.move(
    center.x + (horizontal ? 0 : 34),
    center.y + (horizontal ? 34 : 0),
    { steps: 6 },
  );
  await page.mouse.up();
  const routed = await readModel(page);
  const key = routed.tables.find((item) => item.id === "child")!.foreignKeys[0]!;
  expect(key.points?.length).toBeGreaterThan(0);
  const draggedRoute = await rightAngleRoute(page, key.id);
  expect(draggedRoute).not.toEqual(originalRoute);
  const movedCoordinate =
    (horizontal ? midpoint.y : midpoint.x) + 34 / initial.viewport.zoom;
  expect(
    draggedRoute.some((point, index) => {
      const next = draggedRoute[index + 1];
      if (!next) return false;
      return horizontal
        ? Math.abs(point.y - movedCoordinate) < 0.1 &&
            Math.abs(next.y - movedCoordinate) < 0.1
        : Math.abs(point.x - movedCoordinate) < 0.1 &&
            Math.abs(next.x - movedCoordinate) < 0.1;
    }),
    "the grabbed segment follows the pointer perpendicular to its original direction",
  ).toBe(true);
  expect(routed.tables.find((item) => item.id === "obstacle")!.position).toEqual(
    initial.tables[2]!.position,
  );

  await dragHeader(page, "child", 20, 50);
  const moved = await readModel(page);
  expect(moved.tables.find((item) => item.id === "child")!.position.y).toBeGreaterThan(
    child.position.y,
  );
  const movedRoute = await rightAngleRoute(page, key.id);
  expect(movedRoute).not.toEqual(originalRoute);
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Save model" }).click();
  const stream = await (await download).createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(Buffer.from(chunk));
  const saved = JSON.parse(Buffer.concat(chunks).toString("utf8")) as ERDocument;
  expect(saved).toEqual(moved);
  await page.reload();
  await expect(page.locator(".canvas-loading")).toHaveCount(0);
  await openModel(page, saved);
  expect(await readModel(page)).toEqual(saved);
  expect(await rightAngleRoute(page, key.id)).toEqual(movedRoute);
});
