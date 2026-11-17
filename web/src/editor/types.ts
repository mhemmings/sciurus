export interface Point {
  x: number;
  y: number;
}
export interface Viewport extends Point {
  zoom: number;
}
export interface Column {
  id: string;
  name: string;
  type: string;
  nullable: boolean;
  autoIncrement?: boolean;
  default?: string | null;
  onUpdate?: string;
  charset?: string;
  collation?: string;
  comment?: string;
  generated?: string;
  stored?: boolean;
  invisible?: boolean;
}
export interface IndexColumn {
  column: string;
  length?: number;
  descending?: boolean;
}
export interface TableIndex {
  id: string;
  name: string;
  kind: string;
  columns: IndexColumn[];
  invisible?: boolean;
  comment?: string;
}
export interface ForeignKey {
  id: string;
  name: string;
  columns: string[];
  referencedTable: string;
  referencedColumns: string[];
  onDelete?: string;
  onUpdate?: string;
  points?: Point[];
}
export interface Check {
  name: string;
  expression: string;
  enforced?: boolean | null;
}
export interface Table {
  id: string;
  name: string;
  engine?: string;
  charset?: string;
  collation?: string;
  comment?: string;
  position: Point;
  width: number;
  groupId?: string;
  columns: Column[];
  indexes: TableIndex[];
  foreignKeys: ForeignKey[];
  checks?: Check[];
}
export interface Group {
  id: string;
  name: string;
  color: string;
  position: Point;
  width: number;
  height: number;
}
export interface ERDocument {
  version: number;
  name: string;
  charset?: string;
  collation?: string;
  tables: Table[];
  groups: Group[];
  viewport: Viewport;
}
export interface Envelope {
  document: ERDocument;
  path: string;
  revision: string;
}
export interface Example {
  name: string;
  file: string;
}
export type Selection = { kind: "table" | "group" | "relationship"; id: string } | null;
export type InspectorTab = "table" | "columns" | "indexes" | "relationships" | "checks";
export interface BrowserFileHandle {
  name: string;
  getFile(): Promise<File>;
  createWritable(): Promise<{
    write(data: string): Promise<void>;
    close(): Promise<void>;
  }>;
}
export interface FilePickerWindow {
  showOpenFilePicker?: (options: object) => Promise<BrowserFileHandle[]>;
  showSaveFilePicker?: (options: object) => Promise<BrowserFileHandle>;
}
