<template>
  <div
    class="app-shell"
    :class="{ 'is-panning': panning || spacePressed, 'is-resizing-panel': resizingPanel }"
    :style="shellStyle"
  >
    <header class="topbar">
      <button
        class="icon-button panel-toggle"
        :class="{ active: explorerOpen }"
        title="Toggle explorer"
        aria-label="Toggle explorer"
        :aria-expanded="explorerOpen"
        @click="togglePanel('explorer')"
      >
        <Icon name="panel-left" />
      </button>
      <div class="menu-anchor">
        <button
          class="toolbar-button"
          :aria-expanded="fileMenu"
          @click="fileMenu = !fileMenu"
        >
          <Icon name="folder" />
          File
          <Icon name="down" />
        </button>
        <div v-if="fileMenu" class="dropdown">
          <button @click="createNew">
            <Icon name="plus" />
            New diagram
          </button>
          <button @click="openFile">
            <Icon name="folder" />
            Open model or SQL
            <kbd>⌘ O</kbd>
          </button>
          <button @click="save(true)">
            <Icon name="download" />
            Save as…
            <kbd>⇧ ⌘ S</kbd>
          </button>
          <button @click="openModelEditor">
            <Icon name="code" />
            Edit model JSON
          </button>
          <div class="dropdown-label">Examples</div>
          <button
            v-for="example in examples"
            :key="example.file"
            @click="loadExample(example)"
          >
            <Icon name="table" />
            {{ example.name }}
          </button>
        </div>
      </div>
      <button
        class="document-title"
        :title="boundPath || fileName || document.name"
        aria-label="Project settings"
        @click="
          selection = null;
          if (!inspectorOpen) togglePanel('inspector');
        "
      >
        <strong>{{ fileName || document.name }}</strong>
        <span
          v-if="dirty"
          class="dirty-dot"
          title="Unsaved changes"
          aria-label="Unsaved changes"
        />
      </button>
      <div class="top-actions">
        <button
          class="toolbar-button"
          aria-label="Table"
          :aria-pressed="activeTool === 'table'"
          @click="armTool('table')"
        >
          <Icon name="plus" />
          <span>Table</span>
        </button>
        <button
          class="toolbar-button"
          aria-label="Group"
          :aria-pressed="activeTool === 'group'"
          @click="armTool('group')"
        >
          <Icon name="group" />
          <span>Group</span>
        </button>
        <div role="group" aria-label="Create relationship">
          <button
            v-for="item in relationshipTools"
            :key="item.kind"
            class="toolbar-button relation-tool"
            :aria-label="item.label"
            :title="item.label"
            :aria-pressed="activeTool === item.kind"
            @click="armTool(item.kind)"
          >
            {{ item.symbol }}
          </button>
        </div>
        <span class="toolbar-divider" />
        <button
          class="icon-button"
          :disabled="!undoStack.length"
          title="Undo (⌘ Z)"
          aria-label="Undo"
          @click="undo"
        >
          <Icon name="undo" />
        </button>
        <button
          class="icon-button"
          :disabled="!redoStack.length"
          title="Redo (⇧ ⌘ Z)"
          aria-label="Redo"
          @click="redo"
        >
          <Icon name="redo" />
        </button>
        <button
          class="icon-button"
          title="Fit diagram (F)"
          aria-label="Fit diagram"
          @click="fit"
        >
          <Icon name="fit" />
        </button>
        <span class="toolbar-divider" />
        <button
          class="toolbar-button"
          aria-label="SQL preview"
          :disabled="!!busy"
          @click="previewSQL"
        >
          <Icon name="code" />
          <span>SQL</span>
        </button>
        <button
          class="button primary"
          aria-label="Save model"
          :disabled="!!busy"
          @click="save(false)"
        >
          <Icon name="save" />
          <span>{{ busy === "save" ? "Saving…" : "Save" }}</span>
        </button>
        <button
          class="icon-button"
          title="Keyboard shortcuts"
          aria-label="Shortcuts & help"
          @click="modal = 'help'"
        >
          <Icon name="help" />
        </button>
        <button
          class="icon-button panel-toggle"
          :class="{ active: inspectorOpen }"
          title="Toggle inspector"
          aria-label="Toggle inspector"
          :aria-expanded="inspectorOpen"
          @click="togglePanel('inspector')"
        >
          <Icon name="panel-right" />
        </button>
      </div>
    </header>
    <aside v-if="explorerOpen" class="sidebar" aria-label="Explorer">
      <div class="panel-heading">
        <span>Explorer</span>
        <button
          class="icon-button"
          title="Close explorer"
          aria-label="Close explorer"
          @click="explorerOpen = false"
        >
          <Icon name="close" />
        </button>
      </div>
      <div class="sidebar-search">
        <Icon name="search" />
        <input v-model="query" placeholder="Find a table…" aria-label="Find a table" />
        <kbd>/</kbd>
      </div>
      <div class="explorer-heading">
        <span>Tables</span>
        <button
          class="icon-button"
          title="Add table"
          aria-label="Add table from explorer"
          @click="armTool('table')"
        >
          <Icon name="plus" />
        </button>
      </div>
      <div class="explorer-scroll">
        <button
          v-for="table in filteredTables"
          :key="table.id"
          class="explorer-table"
          :class="{ active: selectedTable?.id === table.id }"
          @pointerenter="hoveredTableId = table.id"
          @pointerleave="hoveredTableId = null"
          :title="table.name"
          @click="focusTable(table)"
        >
          <Icon name="table" />
          <span>{{ table.name }}</span>
        </button>
        <p v-if="!filteredTables.length" class="sidebar-empty">
          {{ query ? "No matching tables." : "No tables." }}
        </p>
        <div class="explorer-heading group-heading">
          <span>Groups</span>
          <button
            class="icon-button"
            title="Add group"
            aria-label="Add group from explorer"
            @click="armTool('group')"
          >
            <Icon name="plus" />
          </button>
        </div>
        <button
          v-for="group in document.groups"
          :key="group.id"
          class="explorer-table"
          :class="{ active: selection?.kind === 'group' && selection.id === group.id }"
          :title="group.name"
          @click="focusGroup(group)"
        >
          <span class="group-swatch" :style="{ backgroundColor: group.color }" />
          <span>{{ group.name }}</span>
        </button>
      </div>
    </aside>
    <div
      v-if="explorerOpen"
      class="panel-resizer"
      role="separator"
      aria-label="Resize explorer"
      aria-orientation="vertical"
      :aria-valuenow="explorerWidth"
      :aria-valuemin="180"
      :aria-valuemax="panelLimit('explorer')"
      tabindex="0"
      @pointerdown="resizePanel($event, 'explorer')"
      @keydown="resizePanelKey($event, 'explorer')"
    />
    <main class="workspace">
      <div v-if="error" class="error-banner" role="alert">
        <Icon name="alert" />
        <span>{{ error }}</span>
        <button class="icon-button" aria-label="Dismiss error" @click="error = ''">
          <Icon name="close" />
        </button>
      </div>

      <div
        ref="canvas"
        class="diagram-canvas"
        tabindex="0"
        aria-label="ER diagram canvas"
        :style="gridStyle"
        :class="{ 'tool-active': activeTool }"
        @pointerdown.capture="toolPointerDown"
        @pointermove="pointerPosition = canvasPoint($event)"
        @pointerleave="pointerPosition = null"
        @pointerdown="canvasPointerDown"
        @wheel.prevent="wheel"
        @contextmenu.prevent
      >
        <div v-if="loading" class="canvas-loading">
          <span class="loading-ring" />
          Opening model…
        </div>
        <div
          v-else-if="!activeTool && !document.tables.length && !document.groups.length"
          class="welcome"
        >
          <Icon name="table" class="empty-model-icon" />
          <h1>Empty model</h1>
          <p>Add a table or open an existing model.</p>
          <div class="welcome-actions">
            <button class="button primary" @pointerdown.stop @click="armTool('table')">
              <Icon name="plus" />
              Add table
            </button>
            <button class="button secondary" @pointerdown.stop @click="openFile">
              <Icon name="folder" />
              Open a model
            </button>
          </div>
          <div v-if="examples.length" class="welcome-examples">
            <span>Examples</span>
            <button
              v-for="example in examples"
              :key="example.file"
              @pointerdown.stop
              @click="loadExample(example)"
            >
              {{ example.name }}
              <Icon name="arrow" />
            </button>
          </div>
        </div>
        <div v-if="activeTool" class="tool-hint" role="status" @pointerdown.stop>
          <span>{{ toolHint }}</span>
          <button
            class="icon-button"
            aria-label="Cancel tool"
            title="Cancel (Esc)"
            @click="cancelTool"
          >
            <Icon name="close" />
          </button>
        </div>
        <div class="diagram-world" :style="worldStyle">
          <div
            v-if="pointerPosition && (activeTool === 'table' || activeTool === 'group')"
            class="placement-preview"
            :style="{
              left: `${pointerPosition.x}px`,
              top: `${pointerPosition.y}px`,
              width: activeTool === 'table' ? '304px' : '550px',
              height: activeTool === 'table' ? '70px' : '440px',
            }"
          >
            <Icon :name="activeTool" />
            {{ activeTool === "table" ? "Table" : "Group" }}
          </div>
          <svg v-if="connectionPreview" class="relationship-layer connection-preview">
            <polyline :points="connectionPreview" />
          </svg>
          <div
            v-for="group in document.groups"
            :key="group.id"
            class="diagram-group"
            :class="{
              selected: selection?.kind === 'group' && selection.id === group.id,
              'drop-target': dropGroupId === group.id,
            }"
            :style="groupStyle(group)"
            @pointerdown.stop="selectGroup(group, $event)"
          >
            <div
              class="group-drag-handle"
              @pointerdown.stop="beginDrag($event, 'group', group.id)"
            >
              <span class="group-swatch" :style="{ backgroundColor: group.color }" />
              <span>{{ group.name }}</span>
            </div>
            <button
              class="resize-handle"
              aria-label="Resize group"
              @pointerdown.stop="beginDrag($event, 'group-size', group.id)"
            />
          </div>
          <svg class="relationship-layer" aria-label="Table relationships">
            <defs>
              <marker
                id="crowfoot"
                markerWidth="14"
                markerHeight="16"
                refX="0"
                refY="8"
                orient="auto"
                markerUnits="userSpaceOnUse"
              >
                <path
                  d="M0 2 L12 8 L0 14 M0 8 L12 8"
                  fill="none"
                  stroke="context-stroke"
                  stroke-width="1.4"
                />
              </marker>
              <marker
                id="one-start"
                markerWidth="14"
                markerHeight="16"
                refX="0"
                refY="8"
                orient="auto"
                markerUnits="userSpaceOnUse"
              >
                <path d="M7 2v12M11 2v12" stroke="context-stroke" stroke-width="1.4" />
              </marker>
              <marker
                id="one"
                markerWidth="14"
                markerHeight="16"
                viewBox="-14 0 14 16"
                refX="0"
                refY="8"
                orient="auto"
                markerUnits="userSpaceOnUse"
              >
                <path d="M-7 2v12M-11 2v12" stroke="context-stroke" stroke-width="1.4" />
              </marker>
              <marker
                id="zero-or-one"
                markerWidth="26"
                markerHeight="16"
                viewBox="-26 0 26 16"
                refX="0"
                refY="8"
                orient="auto"
                markerUnits="userSpaceOnUse"
              >
                <path d="M-7 2v12" stroke="context-stroke" stroke-width="1.4" />
                <circle
                  cx="-20"
                  cy="8"
                  r="4.5"
                  fill="var(--canvas)"
                  stroke="context-stroke"
                  stroke-width="1.4"
                />
              </marker>
            </defs>
            <g
              v-for="edge in edges"
              :key="edge.key.id"
              :class="[
                'relationship',
                {
                  selected:
                    selection?.kind === 'relationship' && selection.id === edge.key.id,
                  highlighted:
                    hoveredTableId === edge.source.id ||
                    hoveredTableId === edge.target.id,
                  subdued:
                    selection &&
                    selection.kind !== 'group' &&
                    selection.id !== edge.key.id &&
                    selection.id !== edge.source.id &&
                    selection.id !== edge.target.id,
                },
              ]"
            >
              <polyline
                class="relationship-hit"
                :points="edge.line"
                :data-relationship-id="edge.key.id"
                @pointerdown.stop="beginRouteDrag($event, edge.key.id)"
                @dblclick.stop="editRelationship(edge.key.id)"
              >
                <title>
                  {{ edge.key.name }}: {{ edge.source.name }} → {{ edge.target.name }}
                </title>
              </polyline>
              <polyline
                class="relationship-line"
                :points="edge.line"
                :marker-start="edge.uniqueSource ? 'url(#one-start)' : 'url(#crowfoot)'"
                :marker-end="edge.optionalReference ? 'url(#zero-or-one)' : 'url(#one)'"
              />
            </g>
          </svg>
          <div
            v-for="table in document.tables"
            :key="table.id"
            class="diagram-table"
            :class="{
              selected: selectedTable?.id === table.id,
              'search-match':
                query && table.name.toLowerCase().includes(query.toLowerCase()),
              'join-source': relationshipStart === table.id,
              'join-target': isRelationshipTool && relationshipStart !== table.id,
            }"
            :style="tableStyle(table)"
            :data-table-id="table.id"
            @pointerenter="hoveredTableId = table.id"
            @pointerleave="hoveredTableId = null"
            @pointerdown.stop="selectTable(table, $event)"
            @dblclick.stop="
              selection = { kind: 'table', id: table.id };
              inspectorTab = 'columns';
            "
          >
            <div
              class="table-header"
              @pointerdown.stop="beginDrag($event, 'table', table.id)"
            >
              <Icon name="table" />
              <strong :title="table.name" @dblclick.stop="renameTable(table)">
                {{ table.name }}
              </strong>
            </div>
            <div>
              <div
                v-for="column in table.columns"
                :key="column.id"
                class="table-column"
                :data-column-id="column.id"
                :class="{ 'primary-column': isPrimary(table, column.id) }"
                @dblclick.stop="editColumn(table, column.id)"
              >
                <button
                  v-if="isForeign(table, column.id)"
                  class="foreign-key-button"
                  :aria-label="`Edit foreign key for ${column.name}`"
                  :title="`Edit foreign key for ${column.name}`"
                  @pointerdown.stop="selectTable(table, $event)"
                  @click.stop="editColumnForeignKey(table, column.id)"
                >
                  <Icon :name="isPrimary(table, column.id) ? 'key' : 'link'" />
                </button>
                <Icon v-else :name="isPrimary(table, column.id) ? 'key' : 'dot'" />
                <span class="column-name" :title="column.name">{{ column.name }}</span>
                <span class="not-null" :title="column.nullable ? 'Nullable' : 'Not null'">
                  {{ column.nullable ? "" : "*" }}
                </span>
                <span class="column-type" :title="displayColumnType(column.type)">
                  {{ displayColumnType(column.type) }}
                </span>
              </div>
            </div>
            <button
              class="resize-handle"
              aria-label="Resize table"
              @pointerdown.stop="beginDrag($event, 'table-size', table.id)"
            />
          </div>
          <svg
            v-if="selectedRelationship && !activeTool"
            class="relationship-layer route-handles-layer"
            aria-label="Relationship route segments"
          >
            <g
              v-for="segment in selectedSegments"
              :key="segment.index"
              :style="{ cursor: segment.vertical ? 'ew-resize' : 'ns-resize' }"
            >
              <line
                class="route-segment-hit"
                :data-segment-index="segment.index"
                :data-relationship-id="selectedRelationship.key.id"
                :x1="segment.a.x"
                :y1="segment.a.y"
                :x2="segment.b.x"
                :y2="segment.b.y"
                :stroke-width="12 / document.viewport.zoom"
                @pointerdown.stop="
                  beginRouteDrag($event, selectedRelationship.key.id, segment.index)
                "
              />
              <rect
                class="route-segment-handle"
                :data-segment-index="segment.index"
                :x="(segment.a.x + segment.b.x) / 2 - 4 / document.viewport.zoom"
                :y="(segment.a.y + segment.b.y) / 2 - 4 / document.viewport.zoom"
                :width="8 / document.viewport.zoom"
                :height="8 / document.viewport.zoom"
                :rx="1 / document.viewport.zoom"
                @pointerdown.stop="
                  beginRouteDrag($event, selectedRelationship.key.id, segment.index)
                "
              >
                <title>Drag segment to reroute</title>
              </rect>
            </g>
          </svg>
        </div>
        <div v-if="document.tables.length" class="canvas-overview" @pointerdown.stop>
          <svg
            :viewBox="`${diagramBounds.x - 60} ${diagramBounds.y - 60} ${diagramBounds.width + 120} ${diagramBounds.height + 120}`"
            @click="navigateOverview"
          >
            <rect
              v-for="group in document.groups"
              :key="group.id"
              :x="group.position.x"
              :y="group.position.y"
              :width="group.width"
              :height="group.height"
              :fill="group.color"
              fill-opacity="0.22"
            />
            <rect
              v-for="table in document.tables"
              :key="table.id"
              :x="table.position.x"
              :y="table.position.y"
              :width="table.width"
              :height="tableHeight(table)"
              :fill="selectedTable?.id === table.id ? 'var(--accent)' : '#94a3b8'"
              rx="15"
            />
            <rect
              :x="-document.viewport.x / document.viewport.zoom"
              :y="-document.viewport.y / document.viewport.zoom"
              :width="canvasSize.width / document.viewport.zoom"
              :height="canvasSize.height / document.viewport.zoom"
              fill="#2563eb14"
              stroke="var(--accent)"
              :stroke-width="Math.max(3, diagramBounds.width / 350)"
            />
          </svg>
        </div>
      </div>
    </main>

    <div
      v-if="inspectorOpen"
      class="panel-resizer"
      role="separator"
      aria-label="Resize inspector"
      aria-orientation="vertical"
      :aria-valuenow="inspectorWidth"
      :aria-valuemin="280"
      :aria-valuemax="panelLimit('inspector')"
      tabindex="0"
      @pointerdown="resizePanel($event, 'inspector')"
      @keydown="resizePanelKey($event, 'inspector')"
    />
    <aside v-if="inspectorOpen" class="inspector" aria-label="Inspector">
      <div class="panel-heading">
        <span>Inspector</span>
        <button
          class="icon-button"
          title="Close inspector"
          aria-label="Close inspector"
          @click="inspectorOpen = false"
        >
          <Icon name="close" />
        </button>
      </div>
      <TableInspector
        v-if="selectedTable"
        ref="tableInspector"
        :table="selectedTable"
        :document="document"
        :focused-foreign-key-id="focusedForeignKeyId"
        :focused-column-id="focusedColumnId"
        v-model:tab="inspectorTab"
        @update="updateTable"
        @delete-table="deleteSelected"
        @delete-column="deleteColumn"
        @select-relationship="selectRelationship"
      />
      <div v-else-if="selectedGroup" class="inspector-body">
        <div class="inspector-heading">
          <span class="eyebrow">GROUP PROPERTIES</span>
          <button
            class="icon-button danger-hover"
            title="Delete group"
            aria-label="Delete group"
            @click="deleteSelected"
          >
            <Icon name="trash" />
          </button>
        </div>
        <div class="inspector-title">
          <span class="table-symbol"><Icon name="group" /></span>
          <h2>{{ selectedGroup.name }}</h2>
        </div>
        <div class="inspector-content">
          <label>
            Group name
            <input
              :value="selectedGroup.name"
              aria-label="Group name"
              @input="changeGroup('name', ($event.target as HTMLInputElement).value)"
            />
          </label>
          <label>
            Color
            <div class="color-field">
              <input
                :value="selectedGroup.color"
                type="color"
                aria-label="Group color"
                @input="changeGroup('color', ($event.target as HTMLInputElement).value)"
              />
              <span class="mono">{{ selectedGroup.color }}</span>
            </div>
          </label>
          <div class="section-label">TABLE MEMBERSHIP</div>
          <div class="membership-list">
            <label v-for="table in sortedTables" :key="table.id" class="check-label">
              <input
                :checked="table.groupId === selectedGroup.id"
                type="checkbox"
                @change="
                  toggleMembership(table.id, ($event.target as HTMLInputElement).checked)
                "
              />
              <span>{{ table.name }}</span>
            </label>
          </div>
          <div class="annotation">
            <Icon name="move" />
            <span>
              Moving this group carries its tables and relationship bends with it.
              Removing a group keeps the tables.
            </span>
          </div>
        </div>
      </div>
      <div v-else-if="selectedRelationship" class="inspector-body">
        <div class="inspector-heading">
          <span class="eyebrow">RELATIONSHIP</span>
          <button
            class="icon-button danger-hover"
            aria-label="Delete relationship"
            @click="deleteSelected"
          >
            <Icon name="trash" />
          </button>
        </div>
        <div class="inspector-title">
          <span class="table-symbol"><Icon name="link" /></span>
          <h2>{{ selectedRelationship.key.name }}</h2>
        </div>
        <div class="inspector-content">
          <div class="relationship-summary">
            <strong>{{ selectedRelationship.source.name }}</strong>
            <Icon name="arrow" />
            <strong>{{ selectedRelationship.target.name }}</strong>
          </div>
          <div
            v-for="(column, index) in selectedRelationship.key.columns"
            :key="column"
            class="relationship-columns"
          >
            <code>
              {{ selectedRelationship.source.columns.find((c) => c.id === column)?.name }}
            </code>
            <Icon name="arrow" />
            <code>
              {{
                selectedRelationship.target.columns.find(
                  (c) => c.id === selectedRelationship?.key.referencedColumns[index],
                )?.name
              }}
            </code>
          </div>
          <div class="detail-row">
            <span>On delete</span>
            <strong>{{ selectedRelationship.key.onDelete || "NO ACTION" }}</strong>
          </div>
          <div class="detail-row">
            <span>On update</span>
            <strong>{{ selectedRelationship.key.onUpdate || "NO ACTION" }}</strong>
          </div>
          <button class="button secondary" @click="editRelationship()">
            <Icon name="settings" />
            Edit foreign key
          </button>
          <p class="inspector-hint">
            Drag any segment on the canvas to reroute this relationship. Connections stay
            at right angles.
          </p>
        </div>
      </div>
      <div v-else class="inspector-body">
        <div class="inspector-heading">
          <span class="eyebrow">MODEL</span>
          <Icon name="settings" />
        </div>
        <div class="inspector-title">
          <span class="table-symbol"><Icon name="grid" /></span>
          <h2>Model settings</h2>
        </div>
        <div class="inspector-content">
          <label>
            Project name
            <input
              :value="document.name"
              aria-label="Project name"
              @input="changeDocument('name', ($event.target as HTMLInputElement).value)"
            />
          </label>
          <label>
            Default character set
            <input
              :value="document.charset"
              placeholder="Server default"
              @input="
                changeDocument('charset', ($event.target as HTMLInputElement).value)
              "
            />
          </label>
          <label>
            Default collation
            <input
              :value="document.collation"
              placeholder="Server default"
              @input="
                changeDocument('collation', ($event.target as HTMLInputElement).value)
              "
            />
          </label>
          <button class="button secondary full-width" @click="openModelEditor">
            <Icon name="code" />
            Edit model JSON
          </button>

          <div v-if="boundPath" class="file-location">
            <Icon name="file" />
            <span>{{ boundPath }}</span>
          </div>
        </div>
      </div>
    </aside>
    <input
      ref="fileInput"
      type="file"
      accept=".json,.sql"
      hidden
      @change="inputFile"
      @cancel="busy = ''"
    />
    <div v-if="busy || loading" class="operation-shield" role="status" aria-live="polite">
      <div>
        <span class="loading-ring" />
        {{
          loading
            ? "Opening model…"
            : busy === "save"
              ? "Saving model…"
              : busy === "open"
                ? "Opening model…"
                : busy === "generate"
                  ? "Generating schema…"
                  : "Validating model…"
        }}
      </div>
    </div>
    <Transition name="toast">
      <div v-if="notice" class="toast-notice" role="status">
        <Icon name="check" />
        {{ notice }}
      </div>
    </Transition>

    <div
      v-if="modal"
      class="modal-backdrop"
      @pointerdown.self="closeModal"
      @keydown="trapFocus"
    >
      <section
        class="modal"
        :class="{ 'code-modal': modal !== 'help' }"
        role="dialog"
        aria-modal="true"
        :aria-label="
          modal === 'sql'
            ? 'Generated SQL'
            : modal === 'json'
              ? 'Model JSON'
              : 'Keyboard shortcuts'
        "
      >
        <header class="modal-header">
          <div>
            <h2>
              {{
                modal === "sql"
                  ? "Generated SQL"
                  : modal === "json"
                    ? "Model JSON"
                    : "Keyboard shortcuts"
              }}
            </h2>
          </div>
          <button class="icon-button" aria-label="Close dialog" @click="closeModal">
            <Icon name="close" />
          </button>
        </header>
        <template v-if="modal === 'sql'">
          <pre class="sql-code" tabindex="0"><code>{{ sql }}</code></pre>
          <footer class="modal-footer">
            <button class="button secondary" @click="copySQL">Copy SQL</button>
            <button class="button primary" @click="downloadSQL">
              <Icon name="download" />
              Export .sql
            </button>
          </footer>
        </template>
        <template v-else-if="modal === 'json'">
          <p class="code-caption">
            Edit the complete model. Changes are validated before they replace your
            diagram.
          </p>
          <textarea
            v-model="jsonDraft"
            class="json-code"
            aria-label="Model JSON source"
            spellcheck="false"
          />
          <div v-if="modalError" class="modal-error" role="alert">{{ modalError }}</div>
          <footer class="modal-footer">
            <button class="button secondary" @click="closeModal">Cancel</button>
            <button
              class="button primary"
              :disabled="busy === 'validate'"
              @click="applyJSON"
            >
              <Icon name="check" />
              Validate & apply
            </button>
          </footer>
        </template>
        <template v-else>
          <div class="shortcut-list">
            <div v-for="shortcut in shortcuts" :key="shortcut.action">
              <span>{{ shortcut.action }}</span>
              <kbd>{{ shortcut.key }}</kbd>
            </div>
          </div>
          <p class="help-note">
            Changes stay local. Save a model to keep its schema and exact layout. SQL
            export creates a separate schema file.
          </p>
        </template>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import Icon from "./components/Icon.vue";
import TableInspector from "./components/TableInspector.vue";
import {
  bounds,
  clone,
  displayColumnType,
  groupForTable,
  moveRouteSegment,
  id,
  moveGroup,
  newDocument,
  newTable,
  normalize,
  relationshipPath,
  removeColumn,
  removeTable,
  tableHeight,
} from "./editor/model";
import { request } from "./editor/service";
import { createRelationship, type RelationshipKind } from "./editor/relationships";
import type {
  BrowserFileHandle,
  Envelope,
  ERDocument,
  Example,
  FilePickerWindow,
  Group,
  InspectorTab,
  Point,
  Selection,
  Table,
} from "./editor/types";

const maxDocumentBytes = 16 * 1024 * 1024;
const document = ref<ERDocument>(newDocument());
const selection = ref<Selection>(null);
const inspectorTab = ref<InspectorTab>("table");
const query = ref("");
const focusedForeignKeyId = ref<string>();
const focusedColumnId = ref<string>();
const tableInspector = ref<InstanceType<typeof TableInspector> | null>(null);
const activeTool = ref<"table" | "group" | RelationshipKind | null>(null);
const relationshipStart = ref<string | null>(null);
const hoveredTableId = ref<string | null>(null);
const pointerPosition = ref<Point | null>(null);
const dropGroupId = ref<string>();
const relationshipTools: { kind: RelationshipKind; label: string; symbol: string }[] = [
  { kind: "one-to-one", label: "One-to-one relationship", symbol: "1:1" },
  { kind: "one-to-many", label: "One-to-many relationship", symbol: "1:N" },
  { kind: "many-to-many", label: "Many-to-many relationship", symbol: "N:M" },
];
const isRelationshipTool = computed(
  () => !!activeTool.value && !["table", "group"].includes(activeTool.value),
);
const toolHint = computed(() => {
  if (activeTool.value === "table") return "Click to place a table · Esc to cancel";
  if (activeTool.value === "group") return "Click to place a group · Esc to cancel";
  if (activeTool.value === "many-to-many")
    return relationshipStart.value
      ? "Click the second table to create a junction table · Esc to cancel"
      : "Click the first table · Esc to cancel";
  return relationshipStart.value
    ? "Click the table that will receive the foreign key · Esc to cancel"
    : "Click the referenced table (1) · Esc to cancel";
});
const connectionPreview = computed(() => {
  const table = document.value.tables.find((item) => item.id === relationshipStart.value);
  if (!table || !pointerPosition.value || !isRelationshipTool.value) return "";
  const start = { x: table.position.x + table.width / 2, y: table.position.y + 20 };
  const end = pointerPosition.value;
  return `${start.x},${start.y} ${end.x},${start.y} ${end.x},${end.y}`;
});
const loading = ref(true);
const busy = ref("");
const error = ref("");
const notice = ref("");
const fileMenu = ref(false);
const modal = ref<"sql" | "json" | "help" | null>(null);
const sql = ref("");
const jsonDraft = ref("");
const modalError = ref("");
const examples = ref<Example[]>([]);
const boundPath = ref("");
const revision = ref("");
const fileName = ref("");
const fileHandle = ref<BrowserFileHandle | null>(null);
const fileFingerprint = ref<string | null>(null);
const savedSignature = ref(JSON.stringify(document.value));
const undoStack = ref<string[]>([]);
const redoStack = ref<string[]>([]);
const canvas = ref<HTMLElement | null>(null);
const canvasSize = ref({ width: 1000, height: 700 });
const fileInput = ref<HTMLInputElement | null>(null);
const panning = ref(false);
const spacePressed = ref(false);
const explorerOpen = ref(true);
const inspectorOpen = ref(true);
const explorerWidth = ref(232);
const inspectorWidth = ref(336);
const resizingPanel = ref(false);
const windowWidth = ref(window.innerWidth);
const shellStyle = computed(() => ({
  gridTemplateColumns: `${explorerOpen.value ? `${explorerWidth.value}px 4px ` : ""}minmax(0, 1fr)${inspectorOpen.value ? ` 4px ${inspectorWidth.value}px` : ""}`,
}));

// Panel preferences belong to this browser; moving a divider never changes the model.
function panelLimit(panel: "explorer" | "inspector") {
  const other =
    panel === "explorer"
      ? inspectorOpen.value
        ? inspectorWidth.value + 4
        : 0
      : explorerOpen.value
        ? explorerWidth.value + 4
        : 0;
  return Math.max(
    panel === "explorer" ? 180 : 280,
    Math.min(panel === "explorer" ? 520 : 640, windowWidth.value - other - 324),
  );
}
function fitPanels() {
  windowWidth.value = window.innerWidth;
  if (windowWidth.value < 820 && explorerOpen.value && inspectorOpen.value)
    explorerOpen.value = false;
  if (windowWidth.value < 600) inspectorOpen.value = false;
  explorerWidth.value = Math.min(
    panelLimit("explorer"),
    Math.max(180, explorerWidth.value),
  );
  inspectorWidth.value = Math.min(
    panelLimit("inspector"),
    Math.max(280, inspectorWidth.value),
  );
}
function togglePanel(panel: "explorer" | "inspector") {
  if (panel === "explorer") {
    explorerOpen.value = !explorerOpen.value;
    if (explorerOpen.value && windowWidth.value < 820) inspectorOpen.value = false;
  } else {
    inspectorOpen.value = !inspectorOpen.value;
    if (inspectorOpen.value && windowWidth.value < 820) explorerOpen.value = false;
  }
  explorerWidth.value = Math.min(panelLimit("explorer"), explorerWidth.value);
  inspectorWidth.value = Math.min(panelLimit("inspector"), inspectorWidth.value);
}
function resizePanel(event: PointerEvent, panel: "explorer" | "inspector") {
  if (event.button !== 0) return;
  event.preventDefault();
  stopPointer?.();
  const initialX = event.clientX;
  const width = panel === "explorer" ? explorerWidth : inspectorWidth;
  const initialWidth = width.value;
  resizingPanel.value = true;
  trackPointer(
    (move) => {
      const delta = (move.clientX - initialX) * (panel === "explorer" ? 1 : -1);
      width.value = Math.round(
        Math.max(
          panel === "explorer" ? 180 : 280,
          Math.min(panelLimit(panel), initialWidth + delta),
        ),
      );
    },
    () => {
      resizingPanel.value = false;
    },
  );
}
function resizePanelKey(event: KeyboardEvent, panel: "explorer" | "inspector") {
  if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
  event.preventDefault();
  const width = panel === "explorer" ? explorerWidth : inspectorWidth;
  const minimum = panel === "explorer" ? 180 : 280;
  const delta = (event.key === "ArrowRight" ? 16 : -16) * (panel === "explorer" ? 1 : -1);
  width.value =
    event.key === "Home"
      ? minimum
      : event.key === "End"
        ? panelLimit(panel)
        : Math.max(minimum, Math.min(panelLimit(panel), width.value + delta));
}

let observer: ResizeObserver | undefined;
let noticeTimer: ReturnType<typeof setTimeout> | undefined;
let stopPointer: (() => void) | null = null;
const shortcuts = [
  { action: "Save model", key: "⌘ / Ctrl S" },
  { action: "Save model as", key: "⇧ ⌘ / Ctrl S" },
  { action: "Open model or SQL", key: "⌘ / Ctrl O" },
  { action: "Undo / redo", key: "⌘ Z / ⇧ ⌘ Z" },
  { action: "Find a table", key: "/" },
  { action: "Fit diagram", key: "F" },
  { action: "Pan canvas", key: "Space + drag" },
  { action: "Zoom canvas", key: "Ctrl / ⌘ + scroll" },
  { action: "Delete selection", key: "Delete / Backspace" },
  { action: "Clear selection / close dialog", key: "Escape" },
];
const dirty = computed(() => JSON.stringify(document.value) !== savedSignature.value);
const sortedTables = computed(() =>
  [...document.value.tables].sort((a, b) => a.name.localeCompare(b.name)),
);
const filteredTables = computed(() =>
  sortedTables.value.filter((table) =>
    table.name.toLowerCase().includes(query.value.toLowerCase()),
  ),
);
const selectedTable = computed(() =>
  selection.value?.kind === "table"
    ? document.value.tables.find((table) => table.id === selection.value?.id)
    : undefined,
);
const selectedGroup = computed(() =>
  selection.value?.kind === "group"
    ? document.value.groups.find((group) => group.id === selection.value?.id)
    : undefined,
);
const edges = computed(() => {
  const tableMap = new Map(document.value.tables.map((table) => [table.id, table]));
  return document.value.tables.flatMap((source) =>
    source.foreignKeys.flatMap((key) => {
      const target = tableMap.get(key.referencedTable);
      if (!target) return [];
      const points = relationshipPath(
        source,
        target,
        key.columns[0] ?? "",
        key.referencedColumns[0] ?? "",
        key.points,
      );
      const uniqueSource = source.indexes.some(
        (index) =>
          ["PRIMARY", "UNIQUE"].includes(index.kind) &&
          index.columns.length > 0 &&
          index.columns.every((part) => key.columns.includes(part.column)),
      );
      const optionalReference = source.columns.some(
        (column) => column.nullable && key.columns.includes(column.id),
      );
      return [
        {
          key,
          source,
          target,
          points,
          uniqueSource,
          optionalReference,
          line: points.map((point) => `${point.x},${point.y}`).join(" "),
        },
      ];
    }),
  );
});
const selectedRelationship = computed(() =>
  selection.value?.kind === "relationship"
    ? edges.value.find((edge) => edge.key.id === selection.value?.id)
    : undefined,
);
const selectedSegments = computed(() => {
  const points = selectedRelationship.value?.points ?? [];
  return points.slice(0, -1).flatMap((a, index) => {
    const b = points[index + 1]!;
    return a.x === b.x && a.y === b.y ? [] : [{ a, b, index, vertical: a.x === b.x }];
  });
});
const diagramBounds = computed(() => bounds(document.value));
const worldStyle = computed(() => ({
  transform: `translate(${document.value.viewport.x}px, ${document.value.viewport.y}px) scale(${document.value.viewport.zoom})`,
}));
const gridStyle = computed(() => ({
  backgroundSize: `${24 * document.value.viewport.zoom}px ${24 * document.value.viewport.zoom}px`,
  backgroundPosition: `${document.value.viewport.x}px ${document.value.viewport.y}px`,
}));

function announce(message: string) {
  notice.value = message;
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => {
    notice.value = "";
  }, 3500);
}
function remember(before: string) {
  if (before === JSON.stringify(document.value)) return;
  undoStack.value.push(before);
  if (undoStack.value.length > 100) undoStack.value.shift();
  redoStack.value = [];
}
function change(action: () => void) {
  const before = JSON.stringify(document.value);
  action();
  remember(before);
}
function undo() {
  cancelTool();
  stopPointer?.();
  const previous = undoStack.value.pop();
  if (!previous) return;
  redoStack.value.push(JSON.stringify(document.value));
  document.value = normalize(JSON.parse(previous) as ERDocument);
}
function redo() {
  cancelTool();
  stopPointer?.();
  const next = redoStack.value.pop();
  if (!next) return;
  undoStack.value.push(JSON.stringify(document.value));
  document.value = normalize(JSON.parse(next) as ERDocument);
}
function tableStyle(table: Table) {
  const color =
    document.value.groups.find((group) => group.id === table.groupId)?.color ?? "#2563eb";
  return {
    left: `${table.position.x}px`,
    top: `${table.position.y}px`,
    width: `${table.width}px`,
    "--table-accent": color,
  };
}
function groupStyle(group: Group) {
  return {
    left: `${group.position.x}px`,
    top: `${group.position.y}px`,
    width: `${group.width}px`,
    height: `${group.height}px`,
    backgroundColor: `${group.color}33`,
    borderColor: `${group.color}b3`,
    "--group-color": group.color,
  };
}
function isPrimary(table: Table, columnID: string) {
  return table.indexes.some(
    (index) =>
      index.kind === "PRIMARY" && index.columns.some((part) => part.column === columnID),
  );
}
function isForeign(table: Table, columnID: string) {
  return table.foreignKeys.some((key) => key.columns.includes(columnID));
}
function canvasPoint(event: { clientX: number; clientY: number }): Point {
  const rect = canvas.value!.getBoundingClientRect();
  return {
    x:
      (event.clientX - rect.left - document.value.viewport.x) /
      document.value.viewport.zoom,
    y:
      (event.clientY - rect.top - document.value.viewport.y) /
      document.value.viewport.zoom,
  };
}
function cancelTool() {
  activeTool.value = null;
  relationshipStart.value = null;
  pointerPosition.value = null;
}
function armTool(tool: "table" | "group" | RelationshipKind) {
  stopPointer?.();
  const previous = activeTool.value;
  cancelTool();
  activeTool.value = previous === tool ? null : tool;
  fileMenu.value = false;
  error.value = "";
  canvas.value?.focus();
}
function toolPointerDown(event: PointerEvent) {
  if (
    !activeTool.value ||
    spacePressed.value ||
    event.button !== 0 ||
    (event.target as Element).closest(".tool-hint, .canvas-overview")
  )
    return;
  event.preventDefault();
  event.stopPropagation();
  const point = canvasPoint(event);
  if (activeTool.value === "table") {
    const existing = new Set(document.value.tables.map((table) => table.name));
    let number = document.value.tables.length + 1;
    while (existing.has(`table_${number}`)) number++;
    const table = newTable(`table_${number}`, {
      x: Math.round(point.x),
      y: Math.round(point.y),
    });
    table.groupId = groupForTable(document.value, table);
    change(() => document.value.tables.push(table));
    selection.value = { kind: "table", id: table.id };
    inspectorTab.value = "table";
  } else if (activeTool.value === "group") {
    const existing = new Set(document.value.groups.map((group) => group.name));
    let number = document.value.groups.length + 1;
    while (existing.has(`Group ${number}`)) number++;
    const group: Group = {
      id: id("group"),
      name: `Group ${number}`,
      color: ["#3b82f6", "#8b5cf6", "#f59e0b", "#14b8a6"][
        document.value.groups.length % 4
      ]!,
      position: { x: Math.round(point.x), y: Math.round(point.y) },
      width: 550,
      height: 440,
    };
    change(() => document.value.groups.push(group));
    selection.value = { kind: "group", id: group.id };
  } else {
    const tableID = (event.target as Element).closest<HTMLElement>("[data-table-id]")
      ?.dataset.tableId;
    if (!tableID) return;
    if (!relationshipStart.value) {
      relationshipStart.value = tableID;
      selection.value = { kind: "table", id: tableID };
      return;
    }
    const before = JSON.stringify(document.value);
    const result = createRelationship(
      document.value,
      activeTool.value,
      relationshipStart.value,
      tableID,
    );
    if ("error" in result) {
      error.value = result.error;
      relationshipStart.value = null;
      return;
    }
    remember(before);
    // Select the new key without moving the canvas or opening a closed inspector.
    selection.value = { kind: "table", id: result.tableId };
    focusedColumnId.value = undefined;
    focusedForeignKeyId.value = result.keyId;
    inspectorTab.value = "relationships";
    cancelTool();
    return;
  }
  cancelTool();
  if (!inspectorOpen.value) togglePanel("inspector");
}
function updateTable(table: Table) {
  change(() => {
    const index = document.value.tables.findIndex((item) => item.id === table.id);
    if (index >= 0) document.value.tables[index] = table;
  });
}
function changeDocument(field: "name" | "charset" | "collation", value: string) {
  change(() => {
    document.value[field] = value;
  });
}
function changeGroup(field: "name" | "color", value: string) {
  const group = selectedGroup.value;
  if (group)
    change(() => {
      group[field] = value;
    });
}
function toggleMembership(tableID: string, checked: boolean) {
  const table = document.value.tables.find((item) => item.id === tableID);
  if (!table) return;
  change(() => {
    table.groupId = checked ? selectedGroup.value?.id : undefined;
  });
}
function deleteColumn(columnID: string) {
  const table = selectedTable.value;
  if (!table) return;
  const column = table.columns.find((item) => item.id === columnID);
  const indexes = table.indexes.filter((index) =>
    index.columns.some((part) => part.column === columnID),
  ).length;
  const foreignKeys = document.value.tables.flatMap((owner) =>
    owner.foreignKeys.filter(
      (key) =>
        (owner.id === table.id && key.columns.includes(columnID)) ||
        (key.referencedTable === table.id && key.referencedColumns.includes(columnID)),
    ),
  ).length;
  if (
    !window.confirm(
      `Delete column “${column?.name}”?${indexes || foreignKeys ? ` This also removes ${indexes} whole index(es) and ${foreignKeys} foreign key(s) that depend on it.` : ""} Check and generated expressions may need updating.`,
    )
  )
    return;
  change(() => {
    removeColumn(document.value, table.id, columnID);
  });
}
function deleteSelected() {
  stopPointer?.();
  if (selectedTable.value) {
    const table = selectedTable.value;
    const count = document.value.tables
      .flatMap((owner) => owner.foreignKeys)
      .filter((key) => key.referencedTable === table.id).length;
    if (
      !window.confirm(
        `Delete table “${table.name}” and its columns, keys, and ${count} incoming relationship(s)?`,
      )
    )
      return;
    change(() => {
      removeTable(document.value, table.id);
    });
  } else if (selectedGroup.value) {
    const group = selectedGroup.value;
    if (!window.confirm(`Remove group “${group.name}”? Its tables will stay in place.`))
      return;
    change(() => {
      document.value.groups = document.value.groups.filter(
        (item) => item.id !== group.id,
      );
      for (const table of document.value.tables)
        if (table.groupId === group.id) delete table.groupId;
    });
  } else if (selectedRelationship.value) {
    const edge = selectedRelationship.value;
    if (!window.confirm(`Delete foreign key “${edge.key.name}”?`)) return;
    change(() => {
      edge.source.foreignKeys = edge.source.foreignKeys.filter(
        (key) => key.id !== edge.key.id,
      );
    });
  }
  selection.value = null;
}
function focusTable(table: Table) {
  focusedColumnId.value = undefined;
  focusedForeignKeyId.value = undefined;
  selection.value = { kind: "table", id: table.id };
  // Selecting a table restores a readable scale after fitting a large diagram.
  const zoom = Math.max(0.08, Math.min(1, (canvasSize.value.width - 48) / table.width));
  document.value.viewport = {
    zoom,
    x: canvasSize.value.width / 2 - (table.position.x + table.width / 2) * zoom,
    y: canvasSize.value.height / 2 - (table.position.y + tableHeight(table) / 2) * zoom,
  };
}
function focusGroup(group: Group) {
  focusedColumnId.value = undefined;
  focusedForeignKeyId.value = undefined;
  selection.value = { kind: "group", id: group.id };
  const zoom = Math.min(
    1,
    (canvasSize.value.width - 100) / group.width,
    (canvasSize.value.height - 100) / group.height,
  );
  document.value.viewport = {
    zoom: Math.max(0.08, zoom),
    x: canvasSize.value.width / 2 - (group.position.x + group.width / 2) * zoom,
    y: canvasSize.value.height / 2 - (group.position.y + group.height / 2) * zoom,
  };
}
function selectGroup(group: Group, event: PointerEvent) {
  if (spacePressed.value || event.button === 1) canvasPointerDown(event);
  else if (event.button === 0) {
    canvas.value?.focus({ preventScroll: true });
    focusedColumnId.value = undefined;
    focusedForeignKeyId.value = undefined;
    selection.value = { kind: "group", id: group.id };
  }
}
function selectTable(table: Table, event: PointerEvent) {
  if (spacePressed.value || event.button === 1) canvasPointerDown(event);
  else if (event.button === 0) {
    canvas.value?.focus({ preventScroll: true });
    focusedColumnId.value = undefined;
    focusedForeignKeyId.value = undefined;
    selection.value = { kind: "table", id: table.id };
  }
}
function fit() {
  const box = bounds(document.value);
  const zoom = Math.max(
    0.05,
    Math.min(
      1.25,
      (canvasSize.value.width - 120) / box.width,
      (canvasSize.value.height - 100) / box.height,
    ),
  );
  document.value.viewport = {
    zoom,
    x: canvasSize.value.width / 2 - (box.x + box.width / 2) * zoom,
    y: canvasSize.value.height / 2 - (box.y + box.height / 2) * zoom,
  };
}
function wheel(event: WheelEvent) {
  if (event.ctrlKey || event.metaKey) {
    const rect = canvas.value!.getBoundingClientRect();
    const x = event.clientX - rect.left;
    const y = event.clientY - rect.top;
    const view = document.value.viewport;
    const zoom = Math.min(
      2.5,
      Math.max(0.05, view.zoom * Math.exp(-event.deltaY * 0.008)),
    );
    const scale = zoom / view.zoom;
    view.x = x - (x - view.x) * scale;
    view.y = y - (y - view.y) * scale;
    view.zoom = zoom;
  } else {
    document.value.viewport.x -= event.deltaX;
    document.value.viewport.y -= event.deltaY;
  }
}
function canvasPointerDown(event: PointerEvent) {
  if (event.button !== 0 && event.button !== 1) return;
  if (
    (event.target as HTMLElement).closest("button, a, input, .welcome") &&
    !spacePressed.value
  )
    return;
  event.preventDefault();
  canvas.value?.focus({ preventScroll: true });
  fileMenu.value = false;
  if (!spacePressed.value && event.button === 0) selection.value = null;
  let previous = { x: event.clientX, y: event.clientY };
  panning.value = true;
  trackPointer(
    (move) => {
      document.value.viewport.x += move.clientX - previous.x;
      document.value.viewport.y += move.clientY - previous.y;
      previous = { x: move.clientX, y: move.clientY };
    },
    () => {
      panning.value = false;
    },
  );
}
function trackPointer(move: (event: PointerEvent) => void, end: () => void) {
  stopPointer?.();
  const finish = () => {
    window.removeEventListener("pointermove", move);
    window.removeEventListener("pointerup", finish);
    window.removeEventListener("pointercancel", finish);
    stopPointer = null;
    end();
  };
  window.addEventListener("pointermove", move);
  window.addEventListener("pointerup", finish);
  window.addEventListener("pointercancel", finish);
  stopPointer = finish;
}
function beginDrag(
  event: PointerEvent,
  kind: "table" | "group" | "table-size" | "group-size",
  objectID: string,
) {
  if (spacePressed.value || event.button === 1) {
    canvasPointerDown(event);
    return;
  }
  if (event.button !== 0) return;
  event.preventDefault();
  canvas.value?.focus({ preventScroll: true });
  focusedColumnId.value = undefined;
  focusedForeignKeyId.value = undefined;
  fileMenu.value = false;
  selection.value = {
    kind: kind.startsWith("table") ? "table" : "group",
    id: objectID,
  };
  const before = JSON.stringify(document.value);
  let previous = canvasPoint(event);
  let moved = false;
  trackPointer(
    (move) => {
      const point = canvasPoint(move);
      const delta = { x: point.x - previous.x, y: point.y - previous.y };
      previous = point;
      moved ||= delta.x !== 0 || delta.y !== 0;
      if (kind === "group") moveGroup(document.value, objectID, delta);
      else if (kind === "table" || kind === "table-size") {
        const table = document.value.tables.find((item) => item.id === objectID)!;
        if (kind === "table") {
          table.position.x += delta.x;
          table.position.y += delta.y;
          dropGroupId.value = groupForTable(document.value, table);
        } else table.width = Math.max(180, Math.min(1200, table.width + delta.x));
      } else if (kind === "group-size") {
        const group = document.value.groups.find((item) => item.id === objectID)!;
        group.width = Math.max(200, group.width + delta.x);
        group.height = Math.max(120, group.height + delta.y);
      }
    },
    () => {
      if (moved && kind === "table") {
        const table = document.value.tables.find((item) => item.id === objectID);
        if (table) table.groupId = groupForTable(document.value, table);
      }
      dropGroupId.value = undefined;
      remember(before);
    },
  );
}
function selectRelationship(keyID: string) {
  cancelTool();
  selection.value = { kind: "relationship", id: keyID };
}
async function editRelationship(keyID = selectedRelationship.value?.key.id) {
  const edge = edges.value.find((item) => item.key.id === keyID);
  if (!edge) return;
  selection.value = { kind: "table", id: edge.source.id };
  focusedColumnId.value = undefined;
  focusedForeignKeyId.value = undefined;
  inspectorTab.value = "relationships";
  if (!inspectorOpen.value) togglePanel("inspector");
  await nextTick();
  focusedForeignKeyId.value = edge.key.id;
}
function editColumnForeignKey(table: Table, columnID: string) {
  if (spacePressed.value) return;
  const key = table.foreignKeys.find((item) => item.columns.includes(columnID));
  if (key) editRelationship(key.id);
}
async function editColumn(table: Table, columnID: string) {
  if (activeTool.value || spacePressed.value) return;
  selection.value = { kind: "table", id: table.id };
  if (!inspectorOpen.value) togglePanel("inspector");
  inspectorTab.value = "columns";
  focusedForeignKeyId.value = undefined;
  // Clear first so double-clicking the same column opens it again after it was closed.
  focusedColumnId.value = undefined;
  await nextTick();
  focusedColumnId.value = columnID;
}
async function renameTable(table: Table) {
  if (activeTool.value || spacePressed.value) return;
  selection.value = { kind: "table", id: table.id };
  if (!inspectorOpen.value) togglePanel("inspector");
  inspectorTab.value = "table";
  await nextTick();
  tableInspector.value?.focusTableName();
}
function beginRouteDrag(event: PointerEvent, keyID: string, segmentIndex?: number) {
  if (spacePressed.value || event.button === 1) {
    canvasPointerDown(event);
    return;
  }
  if (event.button !== 0) return;
  event.preventDefault();
  canvas.value?.focus({ preventScroll: true });
  selection.value = { kind: "relationship", id: keyID };
  const edge = edges.value.find((item) => item.key.id === keyID);
  if (!edge) return;
  const route = clone(edge.points);
  const start = canvasPoint(event);
  let index = segmentIndex ?? 0;
  if (segmentIndex === undefined) {
    let distance = Infinity;
    for (let i = 0; i < route.length - 1; i++) {
      const a = route[i]!;
      const b = route[i + 1]!;
      const x = Math.max(Math.min(a.x, b.x), Math.min(Math.max(a.x, b.x), start.x));
      const y = Math.max(Math.min(a.y, b.y), Math.min(Math.max(a.y, b.y), start.y));
      const next = Math.hypot(start.x - x, start.y - y);
      if (next < distance) {
        distance = next;
        index = i;
      }
    }
  }
  const before = JSON.stringify(document.value);
  trackPointer(
    (move) => {
      const point = canvasPoint(move);
      if (
        Math.hypot(point.x - start.x, point.y - start.y) * document.value.viewport.zoom <
        2
      )
        return;
      edge.key.points = moveRouteSegment(route, index, {
        x: route[index]!.x + point.x - start.x,
        y: route[index]!.y + point.y - start.y,
      }).slice(1, -1);
    },
    () => remember(before),
  );
}
function navigateOverview(event: MouseEvent) {
  const svg = event.currentTarget as SVGSVGElement;
  const transform = svg.getScreenCTM();
  if (!transform) return;
  const point = new DOMPoint(event.clientX, event.clientY).matrixTransform(
    transform.inverse(),
  );
  document.value.viewport.x =
    canvasSize.value.width / 2 - point.x * document.value.viewport.zoom;
  document.value.viewport.y =
    canvasSize.value.height / 2 - point.y * document.value.viewport.zoom;
}

function allowReplace() {
  if (busy.value || loading.value) return false;
  stopPointer?.();
  return (
    !dirty.value ||
    window.confirm("You have unsaved changes. Discard them and open another model?")
  );
}
function setDocument(
  value: ERDocument,
  name: string,
  handle: BrowserFileHandle | null = null,
) {
  stopPointer?.();
  hoveredTableId.value = null;
  cancelTool();
  focusedForeignKeyId.value = undefined;
  focusedColumnId.value = undefined;
  document.value = normalize(value);
  fileName.value = name;
  fileHandle.value = handle;
  fileFingerprint.value = null;
  boundPath.value = "";
  revision.value = "";
  selection.value = null;
  query.value = "";
  undoStack.value = [];
  redoStack.value = [];
  savedSignature.value = JSON.stringify(document.value);
  fileMenu.value = false;
  error.value = "";
}
function createNew() {
  if (!allowReplace()) return;
  setDocument(newDocument(), "");
}
async function loadExample(example: Example) {
  fileMenu.value = false;
  if (!allowReplace()) return;
  busy.value = "open";
  const result = await request<ERDocument>(
    `/api/examples/${encodeURIComponent(example.file)}`,
  );
  busy.value = "";
  if (result.error) {
    error.value = result.error;
    return;
  }
  setDocument(result.value!, example.file);
  await nextTick();
  fit();
  savedSignature.value = JSON.stringify(document.value);
  announce(`${example.name} example opened`);
}
async function openFile() {
  fileMenu.value = false;
  if (!allowReplace()) return;
  busy.value = "open";
  const picker = (window as Window & FilePickerWindow).showOpenFilePicker;
  if (!picker) {
    fileInput.value?.click();
    return;
  }
  try {
    const handles = await picker({
      multiple: false,
      types: [
        {
          description: "Sciurus model or MySQL schema",
          accept: { "application/json": [".json"], "application/sql": [".sql"] },
        },
      ],
    });
    if (handles[0]) await readFile(await handles[0].getFile(), handles[0]);
  } catch (caught) {
    if (!(caught instanceof DOMException && caught.name === "AbortError"))
      error.value =
        caught instanceof Error ? caught.message : "Unable to open the selected file.";
  } finally {
    busy.value = "";
  }
}
async function inputFile(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (file) await readFile(file, null);
  input.value = "";
}
async function readFile(file: File, handle: BrowserFileHandle | null) {
  if (file.size > maxDocumentBytes) {
    error.value = "Choose a model or SQL file smaller than 16 MB.";
    busy.value = "";
    return;
  }
  busy.value = "open";
  try {
    const bytes = await file.arrayBuffer();
    const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    const imported = file.name.toLowerCase().endsWith(".sql");
    let value: ERDocument;
    if (imported) {
      const result = await request<ERDocument>("/api/import", {
        sql: text,
        name: file.name.replace(/\.sql$/i, ""),
      });
      if (result.error) {
        error.value = result.error;
        return;
      }
      value = result.value!;
    } else {
      const result = await request<{ valid: boolean }>(
        "/api/validate",
        text,
        "POST",
        true,
      );
      if (result.error) {
        error.value = result.error;
        return;
      }
      value = JSON.parse(text) as ERDocument;
    }
    setDocument(
      value,
      imported ? file.name.replace(/\.sql$/i, ".schema.json") : file.name,
      imported ? null : handle,
    );
    if (handle && !imported) fileFingerprint.value = await fingerprint(bytes);
    if (imported) {
      await nextTick();
      fit();
      savedSignature.value = "";
    }
    announce(
      imported ? "SQL imported. Save your model to keep the diagram." : "Model opened",
    );
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : "Unable to read this model.";
  } finally {
    busy.value = "";
  }
}
function download(content: string, name: string, type: string) {
  const url = URL.createObjectURL(new Blob([content], { type }));
  const link = window.document.createElement("a");
  link.href = url;
  link.download = name;
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
function suggestedName(extension: string) {
  return `${
    document.value.name
      .replace(/[^a-zA-Z0-9_-]+/g, "-")
      .replace(/^-|-$/g, "")
      .toLowerCase() || "schema"
  }${extension}`;
}
async function fingerprint(bytes: ArrayBuffer): Promise<string> {
  const hash = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(hash), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
}
async function save(asNew: boolean) {
  fileMenu.value = false;
  if (busy.value) return;
  stopPointer?.();
  busy.value = "save";
  error.value = "";
  // Capture content and destination together before any asynchronous validation.
  // The operation shield prevents replacement while this snapshot is in flight.
  const snapshot = clone(document.value);
  const signature = JSON.stringify(snapshot);
  const target = {
    path: boundPath.value,
    revision: revision.value,
    handle: asNew ? null : fileHandle.value,
    fingerprint: fileFingerprint.value,
    name: fileName.value || suggestedName(".schema.json"),
  };
  try {
    const validation = await request<{ valid: boolean }>("/api/validate", snapshot);
    if (validation.error) {
      error.value = validation.error;
      return;
    }
    if (target.path && !asNew) {
      const result = await request<Envelope>(
        "/api/document",
        { document: snapshot, revision: target.revision },
        "PUT",
      );
      if (result.error) {
        error.value = result.error;
        return;
      }
      revision.value = result.value!.revision;
      savedSignature.value = signature;
      announce("Model saved to disk");
      return;
    }
    const content = `${JSON.stringify(snapshot, null, 2)}\n`;
    if (new Blob([content]).size > maxDocumentBytes) {
      error.value = "The saved model exceeds the 16 MiB limit.";
      return;
    }
    let handle = target.handle;
    const picker = (window as Window & FilePickerWindow).showSaveFilePicker;
    if (!handle && picker)
      handle = await picker({
        suggestedName: target.name,
        types: [
          {
            description: "Sciurus model",
            accept: { "application/json": [".json"] },
          },
        ],
      });
    if (handle) {
      if (target.handle && target.fingerprint) {
        const current = await fingerprint(await (await handle.getFile()).arrayBuffer());
        if (current !== target.fingerprint) {
          error.value =
            "This file changed on disk since you opened or saved it. Open the updated file, or use Save as to preserve your edits separately.";
          return;
        }
      }
      const writable = await handle.createWritable();
      await writable.write(content);
      await writable.close();
      fileHandle.value = handle;
      fileFingerprint.value = await fingerprint(new TextEncoder().encode(content).buffer);
      fileName.value = handle.name;
      boundPath.value = "";
      revision.value = "";
    } else {
      download(content, target.name, "application/json");
      if (asNew) {
        boundPath.value = "";
        revision.value = "";
        fileHandle.value = null;
        fileFingerprint.value = null;
      }
    }
    savedSignature.value = signature;
    announce(handle ? "Model saved to disk" : "Model downloaded");
  } catch (caught) {
    if (!(caught instanceof DOMException && caught.name === "AbortError"))
      error.value =
        caught instanceof Error ? caught.message : "The model could not be saved.";
  } finally {
    busy.value = "";
  }
}
async function previewSQL() {
  busy.value = "generate";
  const result = await request<{ sql: string }>("/api/generate", document.value);
  busy.value = "";
  if (result.error) {
    error.value = result.error;
    return;
  }
  sql.value = result.value!.sql;
  modal.value = "sql";
}
function downloadSQL() {
  download(sql.value, suggestedName(".sql"), "application/sql");
  announce("SQL schema exported");
}
async function copySQL() {
  try {
    await navigator.clipboard.writeText(sql.value);
    announce("SQL copied to clipboard");
  } catch {
    error.value = "Clipboard is unavailable. Select the SQL text or export the file.";
  }
}
function openModelEditor() {
  cancelTool();
  fileMenu.value = false;
  jsonDraft.value = JSON.stringify(document.value, null, 2);
  modalError.value = "";
  modal.value = "json";
}
async function applyJSON() {
  modalError.value = "";
  busy.value = "validate";
  const raw = jsonDraft.value;
  const result = await request<{ valid: boolean }>("/api/validate", raw, "POST", true);
  busy.value = "";
  if (result.error) {
    modalError.value = result.error;
    return;
  }
  const value = JSON.parse(raw) as ERDocument;
  change(() => {
    document.value = normalize(value);
  });
  selection.value = null;
  modal.value = null;
  announce("Model validated and applied");
}
function closeModal() {
  if (
    modal.value === "json" &&
    jsonDraft.value !== JSON.stringify(document.value, null, 2) &&
    !window.confirm("Discard unapplied JSON changes?")
  )
    return;
  modal.value = null;
}
function trapFocus(event: KeyboardEvent) {
  if (event.key !== "Tab") return;
  const items = [
    ...(event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>(
      'button:not([disabled]), textarea, [tabindex="0"]',
    ),
  ];
  const first = items[0];
  const last = items[items.length - 1];
  if (event.shiftKey && window.document.activeElement === first) {
    event.preventDefault();
    last?.focus();
  } else if (!event.shiftKey && window.document.activeElement === last) {
    event.preventDefault();
    first?.focus();
  }
}
function keydown(event: KeyboardEvent) {
  if (busy.value || loading.value) {
    event.preventDefault();
    return;
  }
  const typing = (event.target as HTMLElement).closest(
    "input, textarea, select, [contenteditable]",
  );
  const modifier = event.metaKey || event.ctrlKey;
  if (modifier && event.key.toLowerCase() === "s") {
    event.preventDefault();
    if (!modal.value) void save(event.shiftKey);
    return;
  }
  if (event.key === "Escape") {
    if (activeTool.value) {
      cancelTool();
      return;
    }
    if (modal.value) closeModal();
    else {
      selection.value = null;
      fileMenu.value = false;
    }
    return;
  }
  if (modal.value || typing) return;
  if (modifier && event.key.toLowerCase() === "o") {
    event.preventDefault();
    void openFile();
  } else if (modifier && event.key.toLowerCase() === "z") {
    event.preventDefault();
    if (event.shiftKey) redo();
    else undo();
  } else if (modifier && event.key.toLowerCase() === "y") {
    event.preventDefault();
    redo();
  } else if (event.code === "Space") {
    event.preventDefault();
    spacePressed.value = true;
  } else if (event.key === "Delete" || event.key === "Backspace") {
    event.preventDefault();
    deleteSelected();
  } else if (event.key.toLowerCase() === "f") {
    event.preventDefault();
    fit();
  } else if (event.key === "/") {
    event.preventDefault();
    if (!explorerOpen.value) togglePanel("explorer");
    void nextTick(() =>
      window.document
        .querySelector<HTMLInputElement>('[aria-label="Find a table"]')
        ?.focus(),
    );
  }
}
function keyup(event: KeyboardEvent) {
  if (event.code === "Space") spacePressed.value = false;
}
function beforeUnload(event: BeforeUnloadEvent) {
  if (
    dirty.value ||
    (modal.value === "json" &&
      jsonDraft.value !== JSON.stringify(document.value, null, 2))
  ) {
    event.preventDefault();
    event.returnValue = "";
  }
}
function blurWindow() {
  hoveredTableId.value = null;
  spacePressed.value = false;
  stopPointer?.();
}
watch([explorerOpen, inspectorOpen, explorerWidth, inspectorWidth], () => {
  try {
    localStorage.setItem(
      "sciurus.panels.v1",
      JSON.stringify({
        explorerOpen: explorerOpen.value,
        inspectorOpen: inspectorOpen.value,
        explorerWidth: explorerWidth.value,
        inspectorWidth: inspectorWidth.value,
      }),
    );
  } catch {
    /* Editing remains available when browser storage is disabled. */
  }
});
watch(modal, async (value) => {
  if (value) {
    cancelTool();
    await nextTick();
    window.document.querySelector<HTMLElement>(".modal button")?.focus();
  }
});
onMounted(async () => {
  try {
    const preferences = JSON.parse(localStorage.getItem("sciurus.panels.v1") || "{}");
    if (typeof preferences.explorerOpen === "boolean")
      explorerOpen.value = preferences.explorerOpen;
    if (typeof preferences.inspectorOpen === "boolean")
      inspectorOpen.value = preferences.inspectorOpen;
    if (Number.isFinite(preferences.explorerWidth))
      explorerWidth.value = preferences.explorerWidth;
    if (Number.isFinite(preferences.inspectorWidth))
      inspectorWidth.value = preferences.inspectorWidth;
  } catch {
    /* Invalid saved preferences reset to the defaults. */
  }
  fitPanels();
  window.addEventListener("resize", fitPanels);
  window.addEventListener("keydown", keydown);
  window.addEventListener("keyup", keyup);
  window.addEventListener("beforeunload", beforeUnload);
  window.addEventListener("blur", blurWindow);
  observer = new ResizeObserver((entries) => {
    const rect = entries[0]?.contentRect;
    if (rect) canvasSize.value = { width: rect.width, height: rect.height };
  });
  if (canvas.value) observer.observe(canvas.value);
  const [current, available] = await Promise.all([
    request<Envelope>("/api/document"),
    request<Example[]>("/api/examples"),
  ]);
  if (current.error) error.value = current.error;
  else {
    document.value = normalize(current.value!.document);
    boundPath.value = current.value!.path;
    revision.value = current.value!.revision;
    fileName.value = current.value!.path.split(/[\\/]/).pop() || "";
    savedSignature.value = JSON.stringify(document.value);
  }
  if (available.value) examples.value = available.value;
  loading.value = false;
});
onBeforeUnmount(() => {
  observer?.disconnect();
  window.removeEventListener("resize", fitPanels);
  stopPointer?.();
  clearTimeout(noticeTimer);
  window.removeEventListener("keydown", keydown);
  window.removeEventListener("keyup", keyup);
  window.removeEventListener("beforeunload", beforeUnload);
  window.removeEventListener("blur", blurWindow);
});
</script>
