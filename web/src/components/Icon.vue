<template>
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.65"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="paths[name] ?? paths.table" />
  </svg>
</template>
<script setup lang="ts">
defineProps<{ name: string }>();
const paths: Record<string, string> = {
  "panel-left": "M3 4h18v16H3ZM9 4v16",
  "panel-right": "M3 4h18v16H3ZM15 4v16",
  table:
    "M4 3h16a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1ZM3 9h18M9 9v12M3 15h18",
  plus: "M12 5v14M5 12h14",
  close: "m6 6 12 12M6 18 18 6",
  search: "M21 21l-5-5M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0Z",
  folder:
    "M3 7V5a1 1 0 0 1 1-1h5l2 3h9a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7Z",
  save: "m19 3 2 2v15a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h15ZM7 3v6h10V3M7 21v-8h10v8",
  code: "m8 6-6 6 6 6m8-12 6 6-6 6m-3-15-2 18",
  down: "m6 9 6 6 6-6",
  up: "m6 15 6-6 6 6",
  arrow: "m9 5 7 7-7 7",
  undo: "M3 10h11a6 6 0 0 1 0 12M3 10l5-5m-5 5 5 5",
  redo: "M21 10H10a6 6 0 0 0 0 12m15-12-5-5m5 5-5 5",
  group: "m12 3 10 5-10 5L2 8l10-5Zm-10 9 10 5 10-5M2 16l10 5 10-5",
  link: "m10 13 4-4m-7 5-2 2a4 4 0 0 0 6 6l4-4a4 4 0 0 0 0-6m2-2 2-2a4 4 0 0 0-6-6L9 6a4 4 0 0 0 0 6",
  key: "M14 4a5 5 0 1 1-3 9L3 21H1v-4l8-8a5 5 0 0 1 5-5Zm1 3h.01",
  check: "m5 12 4 4L19 6",
  trash: "M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7",
  fit: "M3 9V3h6m6 0h6v6m0 6v6h-6M9 21H3v-6M8 8h8v8H8Z",
  download: "M12 3v12m-5-5 5 5 5-5M3 16v5h18v-5",
  file: "M14 2H5v20h14V7l-5-5ZM14 2v6h5M8 12h8m-8 4h6",
  grid: "M3 3h7v7H3Zm11 0h7v7h-7ZM3 14h7v7H3Zm11 0h7v7h-7Z",
  help: "M9 8a3 3 0 1 1 5 2c-2 1-2 2-2 3m0 4h.01M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0Z",
  dot: "M12 12h.01",
  grip: "M9 5h.01M15 5h.01M9 12h.01M15 12h.01M9 19h.01M15 19h.01",
  move: "M12 2v20M2 12h20m-6-4 4 4-4 4M8 4l4-2 4 2M8 20l4 2 4-2M4 8l-2 4 2 4",
  alert: "m12 3 10 18H2L12 3Zm0 6v5m0 3h.01",
  settings: "M4 6h16M4 12h16M4 18h16M8 3v6m8 0v6m-6 0v6",
};
</script>
