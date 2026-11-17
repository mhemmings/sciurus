<template>
  <div ref="inspectorBody" class="inspector-body">
    <div class="inspector-heading">
      <span class="eyebrow">TABLE PROPERTIES</span>
      <button
        class="icon-button danger-hover"
        title="Delete table"
        aria-label="Delete table"
        @click="emit('deleteTable')"
      >
        <Icon name="trash" />
      </button>
    </div>
    <div class="inspector-title">
      <span class="table-symbol"><Icon name="table" /></span>
      <h2>{{ table.name }}</h2>
    </div>
    <div class="inspector-tabs" role="tablist" aria-label="Table properties">
      <button
        v-for="item in tabs"
        :key="item.id"
        :class="{ active: tab === item.id }"
        :aria-selected="tab === item.id"
        role="tab"
        @click="tab = item.id"
      >
        {{ item.name }}
        <span v-if="item.count !== undefined">{{ item.count }}</span>
      </button>
    </div>
    <div v-if="tab === 'table'" class="inspector-content" @input="apply" @change="apply">
      <label>
        Table name
        <input
          ref="tableNameInput"
          v-model="draft.name"
          aria-label="Table name"
          spellcheck="false"
        />
      </label>
      <label>
        Engine
        <select v-model="draft.engine">
          <option>InnoDB</option>
        </select>
      </label>
      <div class="field-pair">
        <label>
          Character set
          <input v-model="draft.charset" placeholder="Database default" list="charsets" />
        </label>
        <label>
          Collation
          <input v-model="draft.collation" placeholder="Database default" />
        </label>
      </div>
      <label>
        Group
        <select v-model="draft.groupId">
          <option value="">No group</option>
          <option v-for="group in document.groups" :key="group.id" :value="group.id">
            {{ group.name }}
          </option>
        </select>
      </label>
      <label>
        Comment
        <textarea v-model="draft.comment" rows="3" placeholder="Optional comment" />
      </label>
      <button
        class="button secondary"
        @click="
          draft.width = recommendedTableWidth(draft);
          apply();
        "
      >
        <Icon name="fit" />
        Fit to content
      </button>
    </div>
    <div v-if="tab === 'columns'" class="inspector-content">
      <div class="section-top">
        <span class="muted">Drag columns to reorder</span>
        <button class="small-button" @click="addColumn">
          <Icon name="plus" />
          Column
        </button>
      </div>
      <div
        :key="draft.id"
        ref="columnList"
        class="column-list"
        :data-owner-table-id="draft.id"
      >
        <details
          v-for="(column, index) in draft.columns"
          :key="column.id"
          class="property-card column-card"
          :data-column-id="column.id"
          :open="expandedColumn === column.id"
          @toggle="
            if (($event.target as HTMLDetailsElement).open) expandedColumn = column.id;
            else if (expandedColumn === column.id) expandedColumn = '';
          "
        >
          <summary>
            <button
              class="icon-button column-drag-handle"
              :aria-label="`Reorder column ${column.name}`"
              title="Drag to reorder column"
              @click.prevent.stop
            >
              <Icon name="grip" />
            </button>
            <span class="mono column-summary">
              <Icon
                :name="primary(column.id) ? 'key' : 'dot'"
                :class="{ gold: primary(column.id) }"
              />
              {{ column.name || "Unnamed column" }}
            </span>
            <span class="type-label">{{ displayColumnType(column.type) }}</span>
            <Icon name="down" />
          </summary>
          <div class="property-fields" @input="apply" @change="apply">
            <label>
              Name
              <input v-model="column.name" aria-label="Column name" spellcheck="false" />
            </label>
            <label>
              MySQL type
              <input
                :value="
                  editingColumnType === column.id
                    ? columnTypeDraft
                    : displayColumnType(column.type)
                "
                list="mysql-types"
                @focus="
                  editingColumnType = column.id;
                  columnTypeDraft = ($event.target as HTMLInputElement).value;
                "
                @input.stop="editColumnType(column, $event)"
                @compositionend.stop="editColumnType(column, $event)"
                @blur="
                  editColumnType(column, $event);
                  editingColumnType = '';
                "
                aria-label="Column type"
                placeholder="VARCHAR(255)"
                spellcheck="false"
              />
            </label>
            <div class="checkbox-grid">
              <label>
                <input
                  :checked="primary(column.id)"
                  type="checkbox"
                  @input.stop
                  @change.stop="togglePrimary(column, $event)"
                />
                Primary key
              </label>
              <label>
                <input
                  v-model="column.nullable"
                  :disabled="primary(column.id)"
                  type="checkbox"
                />
                Nullable
              </label>
              <label>
                <input
                  :checked="column.autoIncrement"
                  type="checkbox"
                  @input.stop
                  @change.stop="toggleAutoIncrement(column, $event)"
                />
                Auto increment
              </label>
            </div>
            <p v-if="columnIssue?.id === column.id" class="column-error" role="alert">
              {{ columnIssue.message }}
            </p>
            <label class="check-label">
              <input
                :checked="column.default != null"
                type="checkbox"
                @input.stop
                @change.stop="
                  if (($event.target as HTMLInputElement).checked)
                    column.default = 'NULL';
                  else delete column.default;
                  apply();
                "
              />
              Explicit default
            </label>
            <label v-if="column.default != null">
              Default SQL expression
              <input
                v-model="column.default"
                placeholder="NULL, 0, 'text', CURRENT_TIMESTAMP"
                aria-label="Default SQL expression"
                spellcheck="false"
              />
            </label>
            <details class="column-advanced">
              <summary>
                Advanced Settings
                <Icon name="down" />
              </summary>
              <div class="advanced-fields">
                <label class="check-label">
                  <input v-model="column.invisible" type="checkbox" />
                  Invisible
                </label>
                <label>
                  On update
                  <input
                    v-model="column.onUpdate"
                    placeholder="CURRENT_TIMESTAMP"
                    spellcheck="false"
                  />
                </label>
                <div class="field-pair">
                  <label>
                    Character set
                    <input
                      v-model="column.charset"
                      list="charsets"
                      placeholder="Inherit"
                    />
                  </label>
                  <label>
                    Collation
                    <input v-model="column.collation" placeholder="Inherit" />
                  </label>
                </div>
                <label>
                  Generated expression
                  <textarea
                    v-model="column.generated"
                    rows="2"
                    placeholder="e.g. `quantity` * `price`"
                    spellcheck="false"
                  />
                </label>
                <label v-if="column.generated" class="check-label">
                  <input v-model="column.stored" type="checkbox" />
                  Stored (otherwise virtual)
                </label>
                <label>
                  Comment
                  <textarea v-model="column.comment" rows="2" />
                </label>
              </div>
            </details>
            <div class="row-actions">
              <button
                class="icon-button"
                :disabled="index === 0"
                title="Move column up"
                aria-label="Move column up"
                @click="reorder(draft.columns, index, -1)"
              >
                <Icon name="up" />
              </button>
              <button
                class="icon-button"
                :disabled="index === draft.columns.length - 1"
                title="Move column down"
                aria-label="Move column down"
                @click="reorder(draft.columns, index, 1)"
              >
                <Icon name="down" />
              </button>
              <button
                class="text-button danger ml-auto"
                @click="emit('deleteColumn', column.id)"
              >
                <Icon name="trash" />
                Delete column
              </button>
            </div>
          </div>
        </details>
      </div>
      <div v-if="!draft.columns.length" class="empty-inspector">
        Add your first column to define this table.
      </div>
    </div>
    <div v-if="tab === 'indexes'" class="inspector-content">
      <div class="section-top">
        <span class="muted">Keys & indexes</span>
        <button class="small-button" @click="addIndex">
          <Icon name="plus" />
          Index
        </button>
      </div>
      <details
        v-for="(index, position) in draft.indexes"
        :key="index.id"
        class="property-card"
        open
      >
        <summary>
          <Icon name="key" />
          <span class="mono column-summary">{{ index.name || "Unnamed index" }}</span>
          <span class="type-label">{{ index.kind }}</span>
        </summary>
        <div class="property-fields" @input="apply" @change="apply">
          <label>
            Name
            <input v-model="index.name" aria-label="Index name" spellcheck="false" />
          </label>
          <label>
            Kind
            <select
              v-model="index.kind"
              @change="
                index.name =
                  index.kind === 'PRIMARY'
                    ? 'PRIMARY'
                    : index.name === 'PRIMARY'
                      ? `idx_${draft.name}_${position + 1}`
                      : index.name
              "
            >
              <option>PRIMARY</option>
              <option>UNIQUE</option>
              <option>INDEX</option>
              <option>FULLTEXT</option>
              <option>SPATIAL</option>
            </select>
          </label>
          <div class="part-label">COLUMN ORDER</div>
          <div
            v-for="(part, partIndex) in index.columns"
            :key="partIndex"
            class="index-part"
          >
            <select v-model="part.column" aria-label="Index column">
              <option v-for="column in draft.columns" :key="column.id" :value="column.id">
                {{ column.name }}
              </option>
            </select>
            <input
              v-model.number="part.length"
              type="number"
              min="0"
              placeholder="Prefix"
              title="Prefix length (0 = full column)"
              aria-label="Index prefix length"
            />
            <label class="tiny-check">
              <input v-model="part.descending" type="checkbox" />
              DESC
            </label>
            <button
              class="icon-button"
              :disabled="partIndex === 0"
              title="Move index column up"
              @click="reorder(index.columns, partIndex, -1)"
            >
              <Icon name="up" />
            </button>
            <button
              class="icon-button danger-hover"
              title="Remove index column"
              @click="
                index.columns.splice(partIndex, 1);
                apply();
              "
            >
              <Icon name="close" />
            </button>
          </div>
          <button
            class="text-button"
            :disabled="!draft.columns.length"
            @click="
              index.columns.push({ column: draft.columns[0]!.id });
              apply();
            "
          >
            <Icon name="plus" />
            Add column
          </button>
          <label class="check-label">
            <input v-model="index.invisible" type="checkbox" />
            Invisible index
          </label>
          <label>
            Comment
            <input v-model="index.comment" />
          </label>
          <button
            class="text-button danger"
            @click="
              draft.indexes.splice(position, 1);
              apply();
            "
          >
            <Icon name="trash" />
            Delete index
          </button>
        </div>
      </details>
      <p class="inspector-hint">
        Composite keys use the column order shown here. Prefix lengths apply to string
        indexes.
      </p>
    </div>
    <div v-if="tab === 'relationships'" class="inspector-content">
      <div class="section-top">
        <span class="muted">Foreign keys</span>
        <button
          class="small-button"
          :disabled="!draft.columns.length"
          @click="addForeignKey"
        >
          <Icon name="plus" />
          Foreign key
        </button>
      </div>
      <details
        v-for="(key, position) in draft.foreignKeys"
        :key="key.id"
        class="property-card"
        :data-foreign-key-id="key.id"
        :open="expandedForeignKey === key.id"
        @toggle="
          if (($event.target as HTMLDetailsElement).open) expandedForeignKey = key.id;
          else if (expandedForeignKey === key.id) expandedForeignKey = '';
        "
      >
        <summary>
          <Icon name="link" />
          <span class="mono column-summary">
            {{ key.name || "Unnamed foreign key" }}
          </span>
        </summary>
        <div class="property-fields" @input="apply" @change="apply">
          <label>
            Constraint name
            <input v-model="key.name" aria-label="Foreign key name" spellcheck="false" />
          </label>
          <label>
            References table
            <select
              v-model="key.referencedTable"
              aria-label="Referenced table"
              @change="
                key.referencedColumns = key.columns.map((_, index) =>
                  targetColumns(key.referencedTable).some(
                    (column) => column.id === key.referencedColumns[index],
                  )
                    ? key.referencedColumns[index]!
                    : '',
                )
              "
            >
              <option value="" disabled>Choose table…</option>
              <option
                v-for="target in document.tables"
                :key="target.id"
                :value="target.id"
              >
                {{ target.name }}
              </option>
            </select>
          </label>
          <div class="part-label">COLUMN PAIRS · SOURCE → TARGET</div>
          <div v-for="(_, partIndex) in key.columns" :key="partIndex" class="fk-part">
            <select v-model="key.columns[partIndex]" aria-label="Source column">
              <option v-for="column in draft.columns" :key="column.id" :value="column.id">
                {{ column.name }}
              </option>
            </select>
            <Icon name="arrow" />
            <select
              v-model="key.referencedColumns[partIndex]"
              aria-label="Referenced column"
              :disabled="!key.referencedTable"
            >
              <option value="" disabled>Choose column…</option>
              <option
                v-for="column in targetColumns(key.referencedTable)"
                :key="column.id"
                :value="column.id"
              >
                {{ column.name }}
              </option>
            </select>
            <button
              class="icon-button"
              :disabled="partIndex === 0"
              title="Move column pair up"
              aria-label="Move column pair up"
              @click="movePair(position, partIndex)"
            >
              <Icon name="up" />
            </button>
            <button
              class="icon-button danger-hover"
              title="Remove column pair"
              aria-label="Remove column pair"
              @click="
                key.columns.splice(partIndex, 1);
                key.referencedColumns.splice(partIndex, 1);
                apply();
              "
            >
              <Icon name="close" />
            </button>
          </div>
          <button
            class="text-button"
            :disabled="!draft.columns.some((column) => !key.columns.includes(column.id))"
            @click="
              key.columns.push(
                draft.columns.find((column) => !key.columns.includes(column.id))!.id,
              );
              key.referencedColumns.push('');
              apply();
            "
          >
            <Icon name="plus" />
            Add column pair
          </button>
          <div class="field-pair">
            <label>
              On delete
              <select v-model="key.onDelete">
                <option value="">Default</option>
                <option v-for="action in actions" :key="action">{{ action }}</option>
              </select>
            </label>
            <label>
              On update
              <select v-model="key.onUpdate">
                <option value="">Default</option>
                <option v-for="action in actions" :key="action">{{ action }}</option>
              </select>
            </label>
          </div>
          <div class="row-actions">
            <button class="text-button" @click="emit('selectRelationship', key.id)">
              <Icon name="move" />
              Show on canvas
            </button>
            <button
              class="text-button danger ml-auto"
              @click="
                draft.foreignKeys.splice(position, 1);
                apply();
              "
            >
              <Icon name="trash" />
              Delete
            </button>
          </div>
        </div>
      </details>
      <p v-if="!draft.foreignKeys.length" class="inspector-hint">
        No foreign keys defined for this table.
      </p>
      <p class="inspector-hint">
        Select a relationship on the canvas and drag a segment to reroute it.
      </p>
    </div>
    <div v-if="tab === 'checks'" class="inspector-content">
      <div class="section-top">
        <span class="muted">Check constraints</span>
        <button class="small-button" @click="addCheck">
          <Icon name="plus" />
          Check
        </button>
      </div>
      <div
        v-for="(check, index) in draft.checks"
        :key="index"
        class="property-card property-fields"
        @input="apply"
        @change="apply"
      >
        <label>
          Name
          <input v-model="check.name" aria-label="Check name" spellcheck="false" />
        </label>
        <label>
          SQL expression
          <textarea
            v-model="check.expression"
            rows="3"
            placeholder="`amount` >= 0"
            aria-label="Check expression"
            spellcheck="false"
          />
        </label>
        <label class="check-label">
          <input
            :checked="check.enforced !== false"
            type="checkbox"
            @change="check.enforced = ($event.target as HTMLInputElement).checked"
          />
          Enforced
        </label>
        <button
          class="text-button danger"
          @click="
            draft.checks?.splice(index, 1);
            apply();
          "
        >
          <Icon name="trash" />
          Delete check
        </button>
      </div>
      <p class="inspector-hint">
        Checks are validated by MySQL. Column renames inside SQL expressions must be
        updated manually.
      </p>
    </div>
    <datalist id="mysql-types">
      <option v-for="type in types" :key="type">{{ type }}</option>
    </datalist>
    <datalist id="charsets">
      <option>utf8mb4</option>
      <option>ascii</option>
      <option>latin1</option>
      <option>binary</option>
    </datalist>
  </div>
</template>
<script setup lang="ts">
import { computed, nextTick, ref, toRef, useTemplateRef, watch } from "vue";
import { moveArrayElement, useSortable } from "@vueuse/integrations/useSortable";
import {
  clone,
  displayColumnType,
  id,
  newColumn,
  recommendedTableWidth,
} from "../editor/model";
import type { Column, ERDocument, InspectorTab, Table } from "../editor/types";
import Icon from "./Icon.vue";

const props = defineProps<{
  table: Table;
  document: ERDocument;
  focusedForeignKeyId?: string;
  focusedColumnId?: string;
}>();
const tab = defineModel<InspectorTab>("tab", { default: "table" });
const emit = defineEmits<{
  update: [table: Table];
  deleteTable: [];
  deleteColumn: [id: string];
  selectRelationship: [id: string];
}>();
const draft = ref<Table>(clone(props.table));
const expandedColumn = ref("");
// Normalize idle text only; rewriting the live input moves its caret and disrupts IME input.
const editingColumnType = ref("");
const columnTypeDraft = ref("");
const expandedForeignKey = ref(props.table.foreignKeys[0]?.id ?? "");
const inspectorBody = ref<HTMLElement | null>(null);
const tableNameInput = ref<HTMLInputElement | null>(null);
const columnList = useTemplateRef<HTMLDivElement>("columnList");
// Keep Sortable's temporary DOM movement separate from a single committed model edit.
useSortable(
  columnList,
  computed(() => draft.value.columns),
  {
    watchElement: true,
    draggable: ".column-card",
    handle: ".column-drag-handle",
    direction: "vertical",
    animation: 180,
    ghostClass: "column-sortable-ghost",
    chosenClass: "column-sortable-chosen",
    forceFallback: true,
    fallbackClass: "column-sortable-fallback",
    fallbackOnBody: true,
    fallbackTolerance: 4,
    async onUpdate(event) {
      const { oldIndex, newIndex } = event;
      if (oldIndex === undefined || newIndex === undefined || oldIndex === newIndex)
        return;
      const table = draft.value;
      if (event.from.dataset.ownerTableId !== table.id) return;
      // VueUse restores the original DOM order before letting Vue reconcile the new array.
      // Bind the queued update to this draft so changing tables cannot reorder another one.
      moveArrayElement(toRef(table, "columns"), oldIndex, newIndex, event);
      await nextTick();
      if (draft.value === table) apply();
    },
  },
);
const columnIssue = ref<{ id: string; message: string } | null>(null);
watch(
  () => [props.focusedColumnId, props.table.id] as const,
  async ([columnId, tableId], previous) => {
    if (tableId !== previous?.[1]) {
      expandedColumn.value = "";
      editingColumnType.value = "";
      columnIssue.value = null;
    }
    if (!columnId || !props.table.columns.some((column) => column.id === columnId))
      return;
    expandedColumn.value = columnId;
    tab.value = "columns";
    await nextTick();
    const cards = inspectorBody.value?.querySelectorAll<HTMLElement>("[data-column-id]");
    const card = [...(cards ?? [])].find((item) => item.dataset.columnId === columnId);
    card?.scrollIntoView({ block: "nearest" });
  },
  { immediate: true },
);
async function focusTableName() {
  tab.value = "table";
  await nextTick();
  tableNameInput.value?.focus();
  tableNameInput.value?.select();
}
defineExpose({ focusTableName });
// Selecting a wire opens its constraint here, without changing any schema values.
watch(
  () => [props.focusedForeignKeyId, props.table.id] as const,
  async ([keyId, tableId], previous) => {
    if (tableId !== previous?.[1])
      expandedForeignKey.value = props.table.foreignKeys[0]?.id ?? "";
    if (!keyId || !props.table.foreignKeys.some((key) => key.id === keyId)) return;
    expandedForeignKey.value = keyId;
    tab.value = "relationships";
    await nextTick();
    const cards = inspectorBody.value?.querySelectorAll<HTMLElement>(
      "[data-foreign-key-id]",
    );
    const card = [...(cards ?? [])].find((item) => item.dataset.foreignKeyId === keyId);
    card?.scrollIntoView({ block: "nearest" });
  },
  { immediate: true },
);
watch(
  () => props.table,
  (table) => {
    draft.value = clone(table);
  },
  { deep: true },
);
const tabs = computed(() => [
  { id: "table" as const, name: "Table" },
  { id: "columns" as const, name: "Columns", count: draft.value.columns.length },
  { id: "indexes" as const, name: "Keys", count: draft.value.indexes.length },
  {
    id: "relationships" as const,
    name: "Foreign keys",
    count: draft.value.foreignKeys.length,
  },
  { id: "checks" as const, name: "Checks", count: draft.value.checks?.length ?? 0 },
]);
const actions = ["NO ACTION", "RESTRICT", "CASCADE", "SET NULL"];
const types = [
  "BIGINT",
  "BIGINT UNSIGNED",
  "INT",
  "INT UNSIGNED",
  "SMALLINT",
  "TINYINT",
  "BOOLEAN",
  "VARCHAR(255)",
  "CHAR(20)",
  "TEXT",
  "MEDIUMTEXT",
  "LONGTEXT",
  "DECIMAL(10,2)",
  "DOUBLE",
  "FLOAT",
  "DATETIME",
  "DATETIME(6)",
  "TIMESTAMP",
  "DATE",
  "TIME",
  "JSON",
  "BLOB",
  "VARBINARY(255)",
  "BINARY(16)",
  "ENUM('one','two')",
  "POINT",
  "GEOMETRY",
];
function apply() {
  emit("update", clone(draft.value));
}
function primary(column: string) {
  return draft.value.indexes.some(
    (index) =>
      index.kind === "PRIMARY" && index.columns.some((part) => part.column === column),
  );
}
function editColumnType(column: Column, event: Event) {
  columnTypeDraft.value = (event.target as HTMLInputElement).value;
  if ((event as InputEvent).isComposing) return;
  column.type = columnTypeDraft.value;
  apply();
}
function togglePrimary(column: Column, event: Event) {
  const input = event.target as HTMLInputElement;
  if (input.checked && column.default?.replace(/[\s()]/g, "").toUpperCase() === "NULL") {
    input.checked = primary(column.id);
    columnIssue.value = {
      id: column.id,
      message: "Remove the explicit NULL default before enabling Primary key.",
    };
    return;
  }
  const nextIndexes = clone(draft.value.indexes);
  const primaryIndex = nextIndexes.find((index) => index.kind === "PRIMARY");
  if (input.checked) {
    if (primaryIndex) {
      if (!primaryIndex.columns.some((part) => part.column === column.id))
        primaryIndex.columns.push({ column: column.id });
    } else
      nextIndexes.unshift({
        id: id("index"),
        name: "PRIMARY",
        kind: "PRIMARY",
        columns: [{ column: column.id }],
      });
  } else if (primaryIndex) {
    primaryIndex.columns = primaryIndex.columns.filter(
      (part) => part.column !== column.id,
    );
    if (!primaryIndex.columns.length)
      nextIndexes.splice(nextIndexes.indexOf(primaryIndex), 1);
  }
  // An AUTO_INCREMENT column must continue to lead an ordinary visible index.
  // Refuse the operation rather than silently adding an index or disabling it.
  const unindexed = draft.value.columns.find(
    (item) =>
      item.autoIncrement &&
      !nextIndexes.some(
        (index) =>
          ["PRIMARY", "UNIQUE", "INDEX"].includes(index.kind) &&
          !index.invisible &&
          index.columns[0]?.column === item.id,
      ),
  );
  if (unindexed) {
    input.checked = primary(column.id);
    columnIssue.value = {
      id: column.id,
      message: `Keep an index beginning with ${unindexed.name} before removing it from the primary key, or turn off Auto increment.`,
    };
    return;
  }
  columnIssue.value = null;
  draft.value.indexes = nextIndexes;
  if (input.checked) column.nullable = false;
  apply();
}
function toggleAutoIncrement(column: Column, event: Event) {
  const input = event.target as HTMLInputElement;
  if (input.checked && column.default != null) {
    input.checked = !!column.autoIncrement;
    columnIssue.value = {
      id: column.id,
      message: "Remove the explicit default before enabling Auto increment.",
    };
    return;
  }
  if (
    input.checked &&
    !draft.value.indexes.some(
      (index) =>
        ["PRIMARY", "UNIQUE", "INDEX"].includes(index.kind) &&
        !index.invisible &&
        index.columns[0]?.column === column.id,
    )
  ) {
    input.checked = !!column.autoIncrement;
    columnIssue.value = {
      id: column.id,
      message:
        "Select Primary key or add an index beginning with this column before enabling Auto increment.",
    };
    return;
  }
  if (
    input.checked &&
    draft.value.columns.some((item) => item.id !== column.id && item.autoIncrement)
  ) {
    input.checked = !!column.autoIncrement;
    columnIssue.value = {
      id: column.id,
      message:
        "Only one column can use Auto increment. Turn it off on the other column first.",
    };
    return;
  }
  columnIssue.value = null;
  column.autoIncrement = input.checked;
  if (input.checked) column.nullable = false;
  apply();
}
function addColumn() {
  const column = newColumn(`column_${draft.value.columns.length + 1}`);
  draft.value.columns.push(column);
  expandedColumn.value = column.id;
  apply();
}
function reorder<T>(items: T[], index: number, direction: number) {
  const target = index + direction;
  if (target < 0 || target >= items.length) return;
  [items[index], items[target]] = [items[target]!, items[index]!];
  apply();
}
function addIndex() {
  draft.value.indexes.push({
    id: id("index"),
    name: `idx_${draft.value.name}_${draft.value.indexes.length + 1}`,
    kind: "INDEX",
    columns: draft.value.columns.length ? [{ column: draft.value.columns[0]!.id }] : [],
  });
  apply();
}
function targetColumns(tableID: string) {
  return tableID === draft.value.id
    ? draft.value.columns
    : (props.document.tables.find((table) => table.id === tableID)?.columns ?? []);
}
function addForeignKey() {
  if (!draft.value.columns.length) return;
  const keyId = id("fk");
  let number = draft.value.foreignKeys.length + 1;
  while (
    draft.value.foreignKeys.some((key) => key.name === `fk_${draft.value.name}_${number}`)
  )
    number++;
  draft.value.foreignKeys.push({
    id: keyId,
    name: `fk_${draft.value.name}_${number}`,
    columns: [draft.value.columns[0]!.id],
    referencedTable: "",
    referencedColumns: [""],
    onDelete: "NO ACTION",
    onUpdate: "NO ACTION",
  });
  expandedForeignKey.value = keyId;
  apply();
}
function movePair(keyIndex: number, index: number) {
  const key = draft.value.foreignKeys[keyIndex]!;
  if (index < 1) return;
  for (const parts of [key.columns, key.referencedColumns])
    [parts[index], parts[index - 1]] = [parts[index - 1]!, parts[index]!];
  apply();
}
function addCheck() {
  draft.value.checks ??= [];
  draft.value.checks.push({
    name: `chk_${draft.value.name}_${draft.value.checks.length + 1}`,
    expression: "",
    enforced: true,
  });
  apply();
}
</script>

<style scoped>
.column-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.column-drag-handle {
  width: 24px;
  height: 26px;
  flex-shrink: 0;
  padding: 4px;
  margin: -4px 0 -4px -4px;
  cursor: grab;
  touch-action: none;
  color: var(--subtle);
}
.column-drag-handle:hover {
  color: var(--accent);
}
.column-drag-handle svg {
  width: 16px;
  height: 16px;
  stroke-width: 2.8;
  pointer-events: none;
}
.column-sortable-chosen .column-drag-handle {
  cursor: grabbing;
}
.column-sortable-ghost {
  border: 1px dashed var(--accent);
  background: var(--accent-surface);
}
.column-sortable-ghost > summary {
  background: transparent;
  opacity: 0.35;
}
.column-sortable-ghost > .property-fields {
  opacity: 0.15;
}
.column-sortable-fallback {
  opacity: 1 !important;
  border-color: var(--accent);
  background: var(--surface);
  box-shadow:
    0 12px 28px #172b4d30,
    0 2px 6px #172b4d18;
  pointer-events: none;
}
.column-advanced {
  border-top: 1px solid var(--border-soft);
}
.column-advanced > summary {
  padding: 10px 0 0;
  background: transparent;
  color: var(--muted);
  font-size: 11px;
}
.column-advanced > summary > svg {
  transition: transform 0.15s;
}
.column-advanced[open] > summary > svg {
  transform: rotate(180deg);
}
.advanced-fields {
  display: flex;
  flex-direction: column;
  gap: 13px;
  padding-top: 13px;
}
.column-error {
  margin: 0;
  font-size: 11px;
  line-height: 1.6;
  color: var(--danger);
}
</style>
