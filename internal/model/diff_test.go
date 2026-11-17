package model

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mhemmings/sciurus/examples"
	"github.com/stretchr/testify/require"
)

func TestCompareIgnoresDiagramMetadataAndIDs(t *testing.T) {
	source, err := examples.Models.ReadFile("starter.json")
	require.NoError(t, err)
	before, err := Decode(source)
	require.NoError(t, err)
	generated, err := Generate(before, time.Time{})
	require.NoError(t, err)
	after, err := ImportSQL(string(generated), "Imported SQL")
	require.NoError(t, err)
	after.Name = "Different diagram title"
	after.Viewport = Viewport{X: 100, Y: 200, Zoom: 2}
	after.Groups = []Group{{ID: "new-group", Name: "New group", Color: "#abcdef", Width: 500, Height: 600}}
	for ti := range after.Tables {
		table := &after.Tables[ti]
		table.ID = "table-" + table.ID
		table.Position = Point{X: float64(ti) * 100, Y: 400}
		table.Width = 900
		table.GroupID = "new-group"
		for ci := range table.Columns {
			table.Columns[ci].ID = "column-" + table.Columns[ci].ID
		}
		for ii := range table.Indexes {
			index := &table.Indexes[ii]
			index.ID = "index-" + index.ID
			for pi := range index.Columns {
				index.Columns[pi].Column = "column-" + index.Columns[pi].Column
			}
		}
		for fi := range table.ForeignKeys {
			foreign := &table.ForeignKeys[fi]
			foreign.ID = "foreign-" + foreign.ID
			foreign.Points = []Point{{X: 300, Y: 400}}
			foreign.ReferencedTable = "table-" + foreign.ReferencedTable
			for ci := range foreign.Columns {
				foreign.Columns[ci] = "column-" + foreign.Columns[ci]
				foreign.ReferencedColumns[ci] = "column-" + foreign.ReferencedColumns[ci]
			}
		}
		slices.Reverse(table.Indexes)
		slices.Reverse(table.ForeignKeys)
		slices.Reverse(table.Checks)
	}
	slices.Reverse(after.Tables)
	saved, err := Encode(after)
	require.NoError(t, err)
	diff, err := Compare(before, after)
	require.NoError(t, err)
	require.False(t, diff.HasChanges())
	require.Equal(t, diff.BeforeSchema, diff.AfterSchema, "diagram context must also ignore saved IDs and geometry")
	unchanged, err := Encode(after)
	require.NoError(t, err)
	require.Equal(t, saved, unchanged, "comparison must not mutate a document")
}

func TestCompareNormalizesSupportedSQLSyntax(t *testing.T) {
	before, err := ImportSQL(`CREATE TABLE parent (id INTEGER NOT NULL, PRIMARY KEY (id));
CREATE TABLE child (
 id INTEGER NOT NULL,
 parent_id INT NULL,
 code VARCHAR(40) NULL DEFAULT NULL,
 flag BOOLEAN NOT NULL DEFAULT (1),
 total NUMERIC NOT NULL DEFAULT 0,
 created_at DATETIME(0) NOT NULL DEFAULT CURRENT_TIMESTAMP,
 PRIMARY KEY (id),
 KEY by_code (code(40) ASC),
 CONSTRAINT fk_parent FOREIGN KEY (parent_id) REFERENCES parent(id) ON DELETE RESTRICT ON UPDATE NO ACTION,
 CONSTRAINT positive CHECK (ID > 0) ENFORCED
);`, "ER")
	require.NoError(t, err)
	before.Charset, before.Collation = "UTF8MB4", "UTF8MB4_BIN"
	after, err := ImportSQL(`create table child (
 id int not null,
 parent_id integer null,
 code varchar(40) character set utf8mb4 collate utf8mb4_bin null,
 flag tinyint(1) not null default 1,
 total decimal(10,0) not null default 0,
 created_at datetime not null default current_timestamp(),
 key by_code (code),
 primary key (id),
 constraint positive check (id>0),
 constraint fk_parent foreign key (parent_id) references parent(id)
) engine=innodb default charset=utf8mb4 collate=utf8mb4_bin;
create table parent (id int not null, primary key(id)) default collate=utf8mb4_bin;`, "SQL")
	require.NoError(t, err)
	diff, err := Compare(before, after)
	require.NoError(t, err)
	require.False(t, diff.HasChanges(), "%+v", diff.Tables)
	require.Equal(t, diff.BeforeSchema, diff.AfterSchema, "equivalent SQL must produce identical diagram context")
}

func TestCompareIncludesUnchangedSchemaContext(t *testing.T) {
	const source = `CREATE TABLE parent (id BIGINT NOT NULL, PRIMARY KEY(id));
CREATE TABLE child (parent_id BIGINT, label VARCHAR(10) DEFAULT 'draft',
 CONSTRAINT child_parent FOREIGN KEY(parent_id) REFERENCES parent(id) ON DELETE CASCADE);
CREATE TABLE unrelated (id INT);`
	before, err := ImportSQL(source, "Before")
	require.NoError(t, err)
	before.Tables[0].ID = "saved-parent-id"
	before.Tables[0].Columns[0].ID = "saved-id-column"
	before.Tables[0].Indexes[0].Columns[0].Column = "saved-id-column"
	before.Tables[0].Position = Point{X: 234, Y: 567}
	before.Tables[1].Columns[0].ID = "saved-source-column"
	before.Tables[1].ForeignKeys[0].ReferencedTable = "saved-parent-id"
	before.Tables[1].ForeignKeys[0].ReferencedColumns[0] = "saved-id-column"
	before.Tables[1].ForeignKeys[0].Columns[0] = "saved-source-column"
	before.Tables[1].ForeignKeys[0].Points = []Point{{X: 123, Y: 456}}
	after, err := ImportSQL(strings.Replace(source, "VARCHAR(10)", "VARCHAR(20)", 1), "After")
	require.NoError(t, err)
	diff, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, diff.Tables, 1)
	require.Equal(t, "child", diff.Tables[0].Name)
	require.Len(t, diff.BeforeSchema.Tables, 3)
	require.Len(t, diff.AfterSchema.Tables, 3)
	require.Equal(t, "child", diff.BeforeSchema.Tables[0].Name)
	require.Equal(t, "parent", diff.BeforeSchema.Tables[1].Name)
	require.Equal(t, "unrelated", diff.BeforeSchema.Tables[2].Name)
	require.Equal(t, diff.BeforeSchema.Tables[1:], diff.AfterSchema.Tables[1:], "unchanged context stays available to the renderer")
	require.True(t, diff.BeforeSchema.Tables[1].Columns[0].Primary)
	child := diff.BeforeSchema.Tables[0]
	require.Equal(t, "parent_id", child.Columns[0].Name)
	require.Equal(t, "bigint", child.Columns[0].Type)
	require.True(t, child.Columns[0].Nullable)
	require.True(t, child.Columns[0].Foreign)
	require.Equal(t, "'draft'", child.Columns[1].Attributes["default"])
	require.Equal(t, "parent", child.ForeignKeys[0].ReferencedTable)
	require.Equal(t, []string{"parent_id"}, child.ForeignKeys[0].Columns)
	require.Equal(t, []string{"id"}, child.ForeignKeys[0].ReferencedColumns)
	require.Equal(t, "CASCADE", child.ForeignKeys[0].OnDelete)
	require.False(t, child.ForeignKeys[0].UniqueSource)
	// An index on the source makes the diagram's relationship one-to-one.
	after.Tables[1].Indexes = append(after.Tables[1].Indexes, Index{ID: "parent_unique", Name: "parent_unique", Kind: "UNIQUE", Columns: []IndexColumn{{Column: "parent_id"}}})
	diff, err = Compare(before, after)
	require.NoError(t, err)
	require.True(t, diff.AfterSchema.Tables[0].ForeignKeys[0].UniqueSource)
}

func TestCompareReportsDatabaseChanges(t *testing.T) {
	const schema = `CREATE TABLE parent (id BIGINT NOT NULL, PRIMARY KEY(id));
CREATE TABLE item (
 id BIGINT NOT NULL AUTO_INCREMENT,
 parent_id BIGINT NULL,
 code VARCHAR(20) NOT NULL DEFAULT 'same',
 amount INT NOT NULL DEFAULT 1,
 derived INT GENERATED ALWAYS AS (amount + 1) VIRTUAL,
 updated_at TIMESTAMP NULL ON UPDATE CURRENT_TIMESTAMP,
 PRIMARY KEY(id),
 KEY by_code (code(10), amount),
 CONSTRAINT parent_fk FOREIGN KEY(parent_id) REFERENCES parent(id),
 CONSTRAINT positive CHECK(amount > 0)
) DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;`
	before, err := ImportSQL(schema, "Before")
	require.NoError(t, err)
	tests := []struct {
		name, kind, object string
		mutate             func(*Table)
	}{
		{"type", "column", "amount", func(t *Table) { t.Columns[3].Type = "INT UNSIGNED" }},
		{"nullability", "column", "code", func(t *Table) { t.Columns[2].Nullable = true }},
		{"default literal case", "column", "code", func(t *Table) { value := "'Same'"; t.Columns[2].Default = &value }},
		{"auto increment", "column", "id", func(t *Table) { t.Columns[0].AutoIncrement = false }},
		{"on update", "column", "updated_at", func(t *Table) { t.Columns[5].OnUpdate = "" }},
		{"generated expression", "column", "derived", func(t *Table) { t.Columns[4].Generated = "amount + 2" }},
		{"stored", "column", "derived", func(t *Table) { t.Columns[4].Stored = true }},
		{"column invisible", "column", "code", func(t *Table) { t.Columns[2].Invisible = true }},
		{"column comment", "column", "code", func(t *Table) { t.Columns[2].Comment = "New comment" }},
		{"column charset", "column", "code", func(t *Table) { t.Columns[2].Charset = "latin1" }},
		{"column collation", "column", "code", func(t *Table) { t.Columns[2].Collation = "utf8mb4_unicode_ci" }},
		{"column order", "column order", "columns", func(t *Table) { t.Columns[2], t.Columns[3] = t.Columns[3], t.Columns[2] }},
		{"index order", "index", "by_code", func(t *Table) { slices.Reverse(t.Indexes[1].Columns) }},
		{"index prefix", "index", "by_code", func(t *Table) { t.Indexes[1].Columns[0].Length = 5 }},
		{"index direction", "index", "by_code", func(t *Table) { t.Indexes[1].Columns[1].Descending = true }},
		{"index visibility", "index", "by_code", func(t *Table) { t.Indexes[1].Invisible = true }},
		{"index comment", "index", "by_code", func(t *Table) { t.Indexes[1].Comment = "New comment" }},
		{"foreign action", "foreign key", "parent_fk", func(t *Table) { t.ForeignKeys[0].OnDelete = "CASCADE" }},
		{"check expression", "check", "positive", func(t *Table) { t.Checks[0].Expression = "amount >= 0" }},
		{"check enforcement", "check", "positive", func(t *Table) { enforced := false; t.Checks[0].Enforced = &enforced }},
		{"table comment", "options", "table options", func(t *Table) { t.Comment = "New comment" }},
		{"table collation", "options", "table options", func(t *Table) { t.Collation = "utf8mb4_unicode_ci" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			after, err := ImportSQL(schema, "After")
			require.NoError(t, err)
			test.mutate(&after.Tables[1])
			diff, err := Compare(before, after)
			require.NoError(t, err)
			require.True(t, diff.HasChanges())
			require.Len(t, diff.Tables, 1)
			require.Equal(t, "item", diff.Tables[0].Name)
			require.Equal(t, "modified", diff.Tables[0].Kind)
			found := false
			for _, change := range diff.Tables[0].Changes {
				if change.Kind == test.kind && change.Name == test.object {
					found = true
					require.NotEqual(t, change.Before, change.After)
				}
			}
			require.True(t, found, "missing %s %s change: %+v", test.kind, test.object, diff.Tables[0].Changes)
		})
	}
}

func TestComparePreservesImplicitClusteredKey(t *testing.T) {
	// The prefix and virtual UNIQUE indexes cannot become the clustered key.
	// Reordering subsequent eligible keys must therefore remain visible.
	const columns = "a INT NOT NULL, b INT NOT NULL, label VARCHAR(20) NOT NULL, derived INT AS (a + 1) VIRTUAL NOT NULL, UNIQUE KEY prefix (label(5)), UNIQUE KEY by_generated (derived), "
	before, err := ImportSQL("CREATE TABLE t ("+columns+"UNIQUE KEY z (a), UNIQUE KEY a (b));", "Before")
	require.NoError(t, err)
	after, err := ImportSQL("CREATE TABLE t ("+columns+"UNIQUE KEY a (b), UNIQUE KEY z (a));", "After")
	require.NoError(t, err)
	diff, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, diff.Tables, 1)
	require.Equal(t, []DefinitionDiff{{Kind: "clustered key", Name: "InnoDB clustered key", Before: "z", After: "a"}}, diff.Tables[0].Changes)
}

func TestCompareAddsRemovesAndRenames(t *testing.T) {
	before, err := ImportSQL("CREATE TABLE old_name (id INT NOT NULL, PRIMARY KEY(id));", "Before")
	require.NoError(t, err)
	after, err := ImportSQL("CREATE TABLE new_name (id INT NOT NULL, PRIMARY KEY(id));", "After")
	require.NoError(t, err)
	after.Tables[0].ID = before.Tables[0].ID // Persistent ER identity does not imply a SQL rename.
	diff, err := Compare(before, after)
	require.NoError(t, err)
	require.Len(t, diff.Tables, 2)
	require.Equal(t, "new_name", diff.Tables[0].Name)
	require.Equal(t, "added", diff.Tables[0].Kind)
	require.Empty(t, diff.Tables[0].Before)
	require.Equal(t, "removed", diff.Tables[1].Kind)
	require.Empty(t, diff.Tables[1].After)
	for _, table := range diff.Tables {
		require.Len(t, table.Changes, 3, "column, primary key and table options")
	}

	added, err := ImportSQL("CREATE TABLE old_name (id INT NOT NULL, extra INT, PRIMARY KEY(id));", "After")
	require.NoError(t, err)
	diff, err = Compare(before, added)
	require.NoError(t, err)
	require.Equal(t, DefinitionDiff{Kind: "column", Name: "extra", After: "extra int null"}, diff.Tables[0].Changes[0])
	require.Equal(t, "column order", diff.Tables[0].Changes[1].Kind)
}

func TestCompareEmptySchemasAndRejectUnsupportedSQL(t *testing.T) {
	empty := New("Empty")
	sql, err := Generate(empty, time.Time{})
	require.NoError(t, err)
	for _, source := range []string{"", "-- no tables", string(sql)} {
		imported, err := ImportSchemaSQL(source, "Empty SQL")
		require.NoError(t, err)
		diff, err := Compare(empty, imported)
		require.NoError(t, err)
		require.False(t, diff.HasChanges())
	}
	_, err = ImportSQL(string(sql), "Editor import")
	require.ErrorContains(t, err, "no CREATE TABLE")
	_, err = ImportSchemaSQL("DROP TABLE t;", "Invalid")
	require.Error(t, err)
	_, err = Compare(nil, empty)
	require.ErrorContains(t, err, "before schema")
	populated, err := ImportSchemaSQL("CREATE TABLE t (id INT);", "One table")
	require.NoError(t, err)
	diff, err := Compare(empty, populated)
	require.NoError(t, err)
	require.True(t, diff.HasChanges())
	require.Equal(t, "added", diff.Tables[0].Kind)
	require.True(t, strings.HasPrefix(diff.Tables[0].After, "CREATE TABLE `t`"))
}
