import { expect, test } from "@playwright/test";
import type { Page } from "@playwright/test";
import type { ERDocument } from "../src/editor/types";

/** Read through the user-visible model editor, without accessing Vue internals. */
async function model(page: Page): Promise<ERDocument> {
  await page
    .getByRole("banner")
    .getByRole("button", { name: "File", exact: true })
    .click();
  await page
    .locator(".dropdown")
    .getByRole("button", { name: "Edit model JSON", exact: true })
    .click();
  const source = await page
    .getByRole("textbox", { name: "Model JSON source" })
    .inputValue();
  await page.getByRole("button", { name: "Close dialog" }).click();
  return JSON.parse(source) as ERDocument;
}

/** Model downloads are the portable save fallback on browsers without native handles. */
async function downloadedModel(page: Page): Promise<ERDocument> {
  const download = await page.waitForEvent("download");
  const stream = await download.createReadStream();
  if (!stream) throw new Error("Download did not contain a model");
  const chunks: Buffer[] = [];
  for await (const chunk of stream) chunks.push(Buffer.from(chunk));
  return JSON.parse(Buffer.concat(chunks).toString("utf8")) as ERDocument;
}

/** Placement is an explicit canvas gesture, separate from choosing a tool. */
async function placeTable(page: Page) {
  await page.getByRole("button", { name: "Table", exact: true }).click();
  const canvas = (await page
    .getByLabel("ER diagram canvas", { exact: true })
    .boundingBox())!;
  await page.mouse.click(canvas.x + 120, canvas.y + 120);
  await expect(page.locator(".diagram-table")).toHaveCount(1);
}

test.beforeEach(async ({ page }) => {
  // The file-picker boundary is unavailable on Firefox/Safari; exercise its
  // supported download path deterministically in Chromium as well.
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
  await expect(
    page.getByRole("banner").getByRole("button", { name: "File", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".canvas-loading")).toHaveCount(0);
  await expect(page.locator(".operation-shield")).toHaveCount(0);
});

test("a focused table name is saved by keyboard without a blur, then SQL uses the rename", async ({
  page,
}) => {
  await placeTable(page);
  const name = page.getByRole("textbox", { name: "Table name", exact: true });
  await name.fill("customer");
  await expect(name).toBeFocused();
  await expect(page.getByLabel("Unsaved changes", { exact: true })).toBeVisible();
  const saved = downloadedModel(page);
  await name.press("ControlOrMeta+s");
  const document = await saved;
  expect(document.tables[0]!.name).toBe("customer");
  expect(document.tables[0]!.columns[0]!.autoIncrement).toBe(true);
  await expect(page.getByLabel("Unsaved changes", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "SQL preview" }).click();
  await expect(page.locator(".sql-code")).toContainText("CREATE TABLE `customer`");
  await expect(page.locator(".sql-code")).toContainText("PRIMARY KEY (`id`)");
});

test("dragging a group and table preserves geometry through undo and model export", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Commerce", exact: true }).click();
  await expect(page.locator(".diagram-table")).toHaveCount(4);
  const initial = await model(page);
  const header = page.locator(".diagram-group .group-drag-handle").first();
  const box = (await header.boundingBox())!;
  await page.mouse.move(box.x + 65, box.y + 12);
  await page.mouse.down();
  await page.mouse.move(box.x + 125, box.y + 42, { steps: 5 });
  await page.mouse.up();
  const moved = await model(page);
  const group = moved.groups[0]!;
  const oldGroup = initial.groups.find((item) => item.id === group.id)!;
  const dx = group.position.x - oldGroup.position.x;
  const dy = group.position.y - oldGroup.position.y;
  expect(dx).toBeGreaterThan(20);
  expect(dy).toBeGreaterThan(10);
  for (const table of moved.tables.filter((item) => item.groupId === group.id)) {
    const oldTable = initial.tables.find((item) => item.id === table.id)!;
    expect(table.position.x - oldTable.position.x).toBeCloseTo(dx);
    expect(table.position.y - oldTable.position.y).toBeCloseTo(dy);
  }
  await page.getByRole("button", { name: "Undo", exact: true }).click();
  const undone = await model(page);
  expect(undone.groups).toEqual(initial.groups);
  expect(undone.tables.map((table) => table.position)).toEqual(
    initial.tables.map((table) => table.position),
  );
  await page.getByRole("button", { name: "Redo", exact: true }).click();
  expect((await model(page)).groups).toEqual(moved.groups);

  const product = page.locator('[data-table-id="product"]');
  const tableHeader = (await product.locator(".table-header").boundingBox())!;
  await page.mouse.move(tableHeader.x + 80, tableHeader.y + 15);
  await page.mouse.down();
  await page.mouse.move(tableHeader.x + 110, tableHeader.y + 35, { steps: 4 });
  await page.mouse.up();
  const tableMoved = await model(page);
  expect(
    tableMoved.tables.find((table) => table.id === "product")!.position.x,
  ).toBeGreaterThan(moved.tables.find((table) => table.id === "product")!.position.x);

  // Panning from inside a card must move the viewport, never the table itself.
  const body = (await product.locator(".table-column").first().boundingBox())!;
  await page.keyboard.down("Space");
  await page.mouse.move(body.x + 40, body.y + 10);
  await page.mouse.down();
  await page.mouse.move(body.x + 60, body.y + 20, { steps: 4 });
  await page.mouse.up();
  await page.keyboard.up("Space");
  const panned = await model(page);
  expect(panned.tables.map((table) => table.position)).toEqual(
    tableMoved.tables.map((table) => table.position),
  );
  expect(panned.viewport.x - tableMoved.viewport.x).toBeCloseTo(20);
  expect(panned.viewport.y - tableMoved.viewport.y).toBeCloseTo(10);

  const download = downloadedModel(page);
  await page.getByRole("button", { name: "Save model", exact: false }).click();
  const saved = await download;
  expect(saved).toEqual(panned);
  await page.locator('input[type="file"]').setInputFiles({
    name: "roundtrip.json",
    mimeType: "application/json",
    buffer: Buffer.from(JSON.stringify(saved)),
  });
  expect(await model(page)).toEqual(saved);
});

test("columns, a unique index, and a check can be edited and generated through the inspector", async ({
  page,
}) => {
  await placeTable(page);
  await page.getByRole("textbox", { name: "Table name", exact: true }).fill("customer");
  await page.getByRole("tab", { name: /^Columns/ }).click();
  await page.getByRole("button", { name: "Column", exact: true }).click();
  await page.getByRole("textbox", { name: "Column name", exact: true }).fill("email");
  await page.getByRole("checkbox", { name: "Nullable", exact: true }).uncheck();
  await page.getByRole("tab", { name: /^Keys/ }).click();
  await page.getByRole("button", { name: "Index", exact: true }).click();
  await expect(page.locator(".property-card")).toHaveCount(2);
  const unique = page.locator(".property-card").nth(1);
  await unique.getByRole("textbox", { name: "Index name" }).fill("email_unique");
  await unique
    .getByRole("combobox", { name: "Kind", exact: true })
    .selectOption("UNIQUE");
  const current = await model(page);
  const email = current.tables[0]!.columns.find((column) => column.name === "email")!;
  await unique.getByRole("combobox", { name: "Index column" }).selectOption(email.id);
  await page.getByRole("tab", { name: /^Checks/ }).click();
  await page.getByRole("button", { name: "Check", exact: true }).click();
  await page.getByRole("textbox", { name: "Check name" }).fill("nonempty_email");
  await page
    .getByRole("textbox", { name: "Check expression" })
    .fill("CHAR_LENGTH(`email`) > 0");
  await page.getByRole("button", { name: "SQL preview" }).click();
  await expect(page.locator(".sql-code")).toContainText("`email` VARCHAR(255) NOT NULL");
  await expect(page.locator(".sql-code")).toContainText(
    "UNIQUE INDEX `email_unique` (`email`)",
  );
  await expect(page.locator(".sql-code")).toContainText(
    "CONSTRAINT `nonempty_email` CHECK",
  );
});

test("duplicate JSON keys are rejected before parsing and leave the current model intact", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Commerce", exact: true }).click();
  await expect(page.locator(".diagram-table")).toHaveCount(4);
  await page
    .getByRole("banner")
    .getByRole("button", { name: "File", exact: true })
    .click();
  await page
    .locator(".dropdown")
    .getByRole("button", { name: "Edit model JSON", exact: true })
    .click();
  const source = page.getByRole("textbox", { name: "Model JSON source" });
  const original = await source.inputValue();
  await source.fill(
    original.replace('"name": "Commerce"', '"name": "Commerce", "name": "Corrupted"'),
  );
  const validation = page.waitForRequest((request) =>
    request.url().endsWith("/api/validate"),
  );
  await page.getByRole("button", { name: "Validate & apply" }).click();
  expect((await validation).postData()).toContain(
    '"name": "Commerce", "name": "Corrupted"',
  );
  await expect(page.locator(".modal-error")).toContainText(/duplicate/i);
  await source.fill(original);
  await page.getByRole("button", { name: "Close dialog" }).click();
  expect((await model(page)).name).toBe("Commerce");

  // Invalid UTF-8 is rejected at the browser's byte boundary, not repaired.
  await page.locator('input[type="file"]').setInputFiles({
    name: "broken.json",
    mimeType: "application/json",
    buffer: Buffer.from([0x7b, 0xff, 0x7d]),
  });
  await expect(page.getByRole("alert")).toContainText(/encoded|encoding|valid/i);
  await expect(page.locator(".diagram-table")).toHaveCount(4);
});

test("a slow save cannot switch documents or change its captured file target", async ({
  page,
}) => {
  await placeTable(page);
  await page.getByRole("textbox", { name: "Table name", exact: true }).fill("snapshot_a");
  let release: () => void = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/api/validate", async (route) => {
    await gate;
    await route.continue();
  });
  const validation = page.waitForRequest((request) =>
    request.url().endsWith("/api/validate"),
  );
  const saved = downloadedModel(page);
  await page.getByRole("button", { name: "Save model", exact: false }).click();
  await validation;
  await expect(page.locator(".operation-shield")).toBeVisible();
  await page.keyboard.press("ControlOrMeta+o");
  await page.keyboard.press("Delete");
  await expect(
    page.getByRole("textbox", { name: "Table name", exact: true }),
  ).toHaveValue("snapshot_a");
  release();
  expect((await saved).tables[0]!.name).toBe("snapshot_a");
  await expect(page.locator(".operation-shield")).toHaveCount(0);
});

test("a browser-open file detects external edits before overwriting them", async ({
  page,
}) => {
  const contents = JSON.stringify({
    version: 1,
    name: "Browser file",
    tables: [],
    groups: [],
    viewport: { x: 80, y: 80, zoom: 1 },
  });
  await page.evaluate((raw) => {
    const state = { contents: raw, writes: 0 };
    Object.defineProperty(window, "testFileState", { value: state });
    Object.defineProperty(window, "showOpenFilePicker", {
      configurable: true,
      value: async () => [
        {
          name: "browser.json",
          getFile: async () =>
            new File([state.contents], "browser.json", { type: "application/json" }),
          createWritable: async () => ({
            write: async (value: string) => {
              state.contents = value;
              state.writes++;
            },
            close: async () => {},
          }),
        },
      ],
    });
  }, contents);
  await page
    .getByRole("banner")
    .getByRole("button", { name: "File", exact: true })
    .click();
  await page.getByRole("button", { name: /Open model or SQL/ }).click();
  await expect(page.getByRole("textbox", { name: "Project name" })).toHaveValue(
    "Browser file",
  );
  await page.getByRole("textbox", { name: "Project name" }).fill("Local edits");
  await page.evaluate(() => {
    const state = (window as unknown as { testFileState: { contents: string } })
      .testFileState;
    state.contents = state.contents.replace("Browser file", "External edit");
  });
  await page.getByRole("button", { name: "Save model", exact: false }).click();
  await expect(page.getByRole("alert")).toContainText("changed on disk");
  await expect(page.getByLabel("Unsaved changes", { exact: true })).toBeVisible();
  expect(
    await page.evaluate(
      () =>
        (window as unknown as { testFileState: { writes: number } }).testFileState.writes,
    ),
  ).toBe(0);
});

test("the complete Project Management model remains navigable and searchable", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Project Management", exact: true }).click();
  await expect(page.locator(".diagram-table")).toHaveCount(28);
  await expect(page.locator(".diagram-group")).toHaveCount(5);
  await page.getByRole("textbox", { name: "Find a table" }).fill("checklist_item");
  const table = page
    .locator(".explorer-table")
    .filter({ has: page.locator("span", { hasText: /^checklist_item$/ }) });
  await table.click();
  await expect(
    page.getByRole("textbox", { name: "Table name", exact: true }),
  ).toHaveValue("checklist_item");
  await expect(page.locator(".diagram-table.selected .table-header")).toBeVisible();
});

test("both sidebars close and reopen without losing the selected table or diagram", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Commerce", exact: true }).click();
  await expect(page.locator(".diagram-table")).toHaveCount(4);
  await page
    .locator(".explorer-table")
    .filter({ hasText: /^product$/ })
    .click();
  await expect(
    page.getByRole("textbox", { name: "Table name", exact: true }),
  ).toHaveValue("product");
  const initial = await model(page);

  // The header toggles remain available when both panels are absent. Closing
  // an inspector must not clear selection or mutate the underlying model.
  await page.getByRole("button", { name: "Close inspector" }).click();
  await expect(page.getByRole("complementary", { name: "Inspector" })).toHaveCount(0);
  await page.getByRole("button", { name: "Close explorer" }).click();
  await expect(page.getByRole("complementary", { name: "Explorer" })).toHaveCount(0);
  await expect(page.locator('[data-table-id="product"]')).toHaveClass(/selected/);
  await expect(page.getByRole("button", { name: "Toggle inspector" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );
  await expect(page.getByRole("button", { name: "Toggle explorer" })).toHaveAttribute(
    "aria-expanded",
    "false",
  );

  await page.getByRole("button", { name: "Toggle explorer" }).click();
  await expect(page.locator(".explorer-table.active")).toHaveText("product");
  await page.getByRole("button", { name: "Toggle inspector" }).click();
  await expect(
    page.getByRole("textbox", { name: "Table name", exact: true }),
  ).toHaveValue("product");
  const restored = await model(page);
  expect(restored.tables).toEqual(initial.tables);
  expect(restored.groups).toEqual(initial.groups);
});

test("drag-resized sidebars retain their widths and open preferences after reload", async ({
  page,
}) => {
  const explorer = page.getByRole("complementary", { name: "Explorer" });
  const inspector = page.getByRole("complementary", { name: "Inspector" });
  const originalExplorer = (await explorer.boundingBox())!.width;
  const originalInspector = (await inspector.boundingBox())!.width;
  const explorerHandle = (await page
    .getByRole("separator", { name: "Resize explorer" })
    .boundingBox())!;
  await page.mouse.move(
    explorerHandle.x + explorerHandle.width / 2,
    explorerHandle.y + explorerHandle.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    explorerHandle.x + explorerHandle.width / 2 + 64,
    explorerHandle.y + explorerHandle.height / 2,
    { steps: 5 },
  );
  await page.mouse.up();
  await expect
    .poll(async () => (await explorer.boundingBox())!.width)
    .toBeCloseTo(originalExplorer + 64, 0);

  // The right panel grows by dragging its left edge to the left, the opposite
  // sign from the explorer's divider. Exercise the actual pointer boundary.
  const inspectorHandle = (await page
    .getByRole("separator", { name: "Resize inspector" })
    .boundingBox())!;
  await page.mouse.move(
    inspectorHandle.x + inspectorHandle.width / 2,
    inspectorHandle.y + inspectorHandle.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    inspectorHandle.x + inspectorHandle.width / 2 - 72,
    inspectorHandle.y + inspectorHandle.height / 2,
    { steps: 5 },
  );
  await page.mouse.up();
  await expect
    .poll(async () => (await inspector.boundingBox())!.width)
    .toBeCloseTo(originalInspector + 72, 0);
  const resizedExplorer = (await explorer.boundingBox())!.width;
  const resizedInspector = (await inspector.boundingBox())!.width;
  await expect(page.getByLabel("Unsaved changes", { exact: true })).toHaveCount(0);

  await page.reload();
  await expect(page.locator(".operation-shield")).toHaveCount(0);
  await expect
    .poll(async () => (await explorer.boundingBox())!.width)
    .toBeCloseTo(resizedExplorer, 0);
  await expect
    .poll(async () => (await inspector.boundingBox())!.width)
    .toBeCloseTo(resizedInspector, 0);
  await page.getByRole("button", { name: "Close explorer" }).click();
  await page.getByRole("button", { name: "Close inspector" }).click();
  await page.reload();
  await expect(page.locator(".operation-shield")).toHaveCount(0);
  await expect(explorer).toHaveCount(0);
  await expect(inspector).toHaveCount(0);
  await page.getByRole("button", { name: "Toggle explorer" }).click();
  await page.getByRole("button", { name: "Toggle inspector" }).click();
  await expect
    .poll(async () => (await explorer.boundingBox())!.width)
    .toBeCloseTo(resizedExplorer, 0);
  await expect
    .poll(async () => (await inspector.boundingBox())!.width)
    .toBeCloseTo(resizedInspector, 0);

  // Keyboard users can reach the same resize boundaries without dragging.
  const leftDivider = page.getByRole("separator", { name: "Resize explorer" });
  const rightDivider = page.getByRole("separator", { name: "Resize inspector" });
  await leftDivider.press("Home");
  await expect(leftDivider).toHaveAttribute("aria-valuenow", "180");
  await leftDivider.press("ArrowRight");
  await expect
    .poll(async () => (await explorer.boundingBox())!.width)
    .toBeGreaterThan(180);
  await rightDivider.press("Home");
  await expect(rightDivider).toHaveAttribute("aria-valuenow", "280");
  await rightDivider.press("ArrowLeft");
  await expect
    .poll(async () => (await inspector.boundingBox())!.width)
    .toBeGreaterThan(280);
  await expect(page.getByLabel("Unsaved changes", { exact: true })).toHaveCount(0);
});

for (const example of [
  { name: "Commerce", tables: 4, groups: 2 },
  { name: "Project Management", tables: 28, groups: 5 },
]) {
  test(`${example.name} example has readable names, disjoint tables and groups, and enclosed members`, async ({
    page,
  }) => {
    await page.getByRole("button", { name: example.name, exact: true }).click();
    await expect(page.locator(".diagram-table")).toHaveCount(example.tables);
    await expect(page.locator(".diagram-group")).toHaveCount(example.groups);
    await page.evaluate(async () => {
      await document.fonts.ready;
    });
    const definition = await model(page);

    // Measure the rendered cards, not the same height estimate used by the
    // generator. offsetWidth/Height exclude canvas zoom; text Range bounds also
    // detect overflow beyond the card when a span itself does not clip text.
    const rendered = await page.locator(".diagram-table").evaluateAll((cards) =>
      cards.map((item) => {
        const card = item as HTMLElement;
        const cardBox = card.getBoundingClientRect();
        const zoom = cardBox.width / card.offsetWidth;
        const names = [
          ...card.querySelectorAll<HTMLElement>(".table-header strong, .column-name"),
        ].map((name) => {
          const range = document.createRange();
          range.selectNodeContents(name);
          const textBox = range.getBoundingClientRect();
          return {
            text: name.textContent ?? "",
            clipped: Math.max(0, name.scrollWidth - name.clientWidth),
            outside: Math.max(
              0,
              (cardBox.left - textBox.left) / zoom,
              (textBox.right - cardBox.right) / zoom,
            ),
          };
        });
        return {
          id: card.dataset.tableId!,
          width: card.offsetWidth,
          height: card.offsetHeight,
          names,
        };
      }),
    );
    const rectangles = definition.tables.map((table) => {
      const card = rendered.find((item) => item.id === table.id)!;
      expect(card, `missing rendered card ${table.name}`).toBeDefined();
      expect(
        Math.abs(card.width - table.width),
        `${table.name}: rendered and saved widths differ`,
      ).toBeLessThanOrEqual(1);
      expect(card.names.map((item) => item.text.trim())).toEqual([
        table.name,
        ...table.columns.map((column) => column.name),
      ]);
      return {
        id: table.id,
        name: table.name,
        groupId: table.groupId,
        x: table.position.x,
        y: table.position.y,
        width: card.width,
        height: card.height,
      };
    });
    const unreadable = rendered.flatMap((card) =>
      card.names
        .filter((name) => name.clipped > 1 || name.outside > 1)
        .map((name) => ({ table: card.id, ...name })),
    );
    expect(unreadable, "full table and column names must fit at the saved width").toEqual(
      [],
    );

    const overlaps: string[] = [];
    for (let i = 0; i < rectangles.length; i++) {
      for (let j = i + 1; j < rectangles.length; j++) {
        const a = rectangles[i]!;
        const b = rectangles[j]!;
        if (
          Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x) > 1 &&
          Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y) > 1
        )
          overlaps.push(`${a.name} overlaps ${b.name}`);
      }
    }
    expect(
      overlaps,
      "table rectangles must not intersect at their actual DOM height",
    ).toEqual([]);

    const groupDimensions = await page.locator(".diagram-group").evaluateAll((groups) =>
      groups.map((item) => {
        const group = item as HTMLElement;
        return {
          width: group.offsetWidth,
          height: group.offsetHeight,
          header: group.querySelector<HTMLElement>(".group-drag-handle")!.offsetHeight,
        };
      }),
    );
    const groups = definition.groups.map((group, index) => ({
      ...group,
      ...groupDimensions[index]!,
    }));
    const groupErrors: string[] = [];
    for (const table of rectangles) {
      for (const group of groups) {
        const x = group.position.x;
        const y = group.position.y;
        if (table.groupId === group.id) {
          if (
            table.x < x - 1 ||
            table.y < y + group.header - 1 ||
            table.x + table.width > x + group.width + 1 ||
            table.y + table.height > y + group.height + 1
          )
            groupErrors.push(`${table.name} escapes ${group.name} or covers its heading`);
        } else if (
          Math.min(table.x + table.width, x + group.width) - Math.max(table.x, x) > 1 &&
          Math.min(table.y + table.height, y + group.height) - Math.max(table.y, y) > 1
        )
          groupErrors.push(`${table.name} intrudes into unrelated group ${group.name}`);
      }
    }
    for (let i = 0; i < groups.length; i++) {
      for (let j = i + 1; j < groups.length; j++) {
        const a = groups[i]!;
        const b = groups[j]!;
        if (
          Math.min(a.position.x + a.width, b.position.x + b.width) -
            Math.max(a.position.x, b.position.x) >
            1 &&
          Math.min(a.position.y + a.height, b.position.y + b.height) -
            Math.max(a.position.y, b.position.y) >
            1
        )
          groupErrors.push(`${a.name} overlaps ${b.name}`);
      }
    }
    expect(
      groupErrors,
      "groups must enclose only their own members without overlapping",
    ).toEqual([]);
  });
}
