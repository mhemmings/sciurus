// Package model defines Sciurus's versioned, portable MySQL ER document.
// IDs are independent from SQL names so visual edits and renames preserve links.
package model

// Document is the complete source of truth, including schema and diagram layout.
// Array order is deliberate: columns and index parts retain their SQL order.
type Document struct {
	Version   int      `json:"version"`
	Name      string   `json:"name"`
	Charset   string   `json:"charset,omitempty"`
	Collation string   `json:"collation,omitempty"`
	Tables    []Table  `json:"tables"`
	Groups    []Group  `json:"groups"`
	Viewport  Viewport `json:"viewport"`
}

// Point is an absolute coordinate in diagram space, unaffected by zoom.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Viewport records the user's view without affecting generated SQL.
type Viewport struct {
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Zoom float64 `json:"zoom"`
}

// Table describes a physical MySQL table and its persistent diagram placement.
type Table struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Engine      string       `json:"engine,omitempty"`
	Charset     string       `json:"charset,omitempty"`
	Collation   string       `json:"collation,omitempty"`
	Comment     string       `json:"comment,omitempty"`
	Position    Point        `json:"position"`
	Width       float64      `json:"width"`
	GroupID     string       `json:"groupId,omitempty"`
	Columns     []Column     `json:"columns"`
	Indexes     []Index      `json:"indexes"`
	ForeignKeys []ForeignKey `json:"foreignKeys"`
	Checks      []Check      `json:"checks,omitempty"`
}

// Column separates the MySQL type from optional attributes. Default is a SQL
// expression: nil means no explicit default; a pointer to "NULL" is explicit NULL.
type Column struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Nullable      bool    `json:"nullable"`
	AutoIncrement bool    `json:"autoIncrement,omitempty"`
	Default       *string `json:"default,omitempty"`
	OnUpdate      string  `json:"onUpdate,omitempty"`
	Charset       string  `json:"charset,omitempty"`
	Collation     string  `json:"collation,omitempty"`
	Comment       string  `json:"comment,omitempty"`
	Generated     string  `json:"generated,omitempty"`
	Stored        bool    `json:"stored,omitempty"`
	Invisible     bool    `json:"invisible,omitempty"`
}

// Index describes PRIMARY, UNIQUE, INDEX, FULLTEXT or SPATIAL keys.
type Index struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Kind      string        `json:"kind"`
	Columns   []IndexColumn `json:"columns"`
	Invisible bool          `json:"invisible,omitempty"`
	Comment   string        `json:"comment,omitempty"`
}

// IndexColumn references a column ID, with optional prefix length and direction.
type IndexColumn struct {
	Column     string `json:"column"`
	Length     int    `json:"length,omitempty"`
	Descending bool   `json:"descending,omitempty"`
}

// ForeignKey uses stable table/column IDs; points persist manually routed bends.
type ForeignKey struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Columns           []string `json:"columns"`
	ReferencedTable   string   `json:"referencedTable"`
	ReferencedColumns []string `json:"referencedColumns"`
	OnDelete          string   `json:"onDelete,omitempty"`
	OnUpdate          string   `json:"onUpdate,omitempty"`
	Points            []Point  `json:"points,omitempty"`
}

// Check is a named MySQL CHECK constraint. Nil enforcement means enforced.
type Check struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
	Enforced   *bool  `json:"enforced,omitempty"`
}

// Group is a movable and resizable Workbench-style layer containing tables.
// Child table coordinates stay absolute; moving a group translates its members.
type Group struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	Position Point   `json:"position"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
}

// New returns an empty, valid document ready for interactive editing.
func New(name string) *Document {
	return &Document{Version: 1, Name: name, Tables: []Table{}, Groups: []Group{}, Viewport: Viewport{X: 80, Y: 80, Zoom: 1}}
}
