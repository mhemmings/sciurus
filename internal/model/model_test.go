package model

import (
	"bytes"
	"io/fs"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/mhemmings/sciurus/examples"
	"github.com/stretchr/testify/require"
)

// TestExampleDeterminism exercises many independent serialization and SQL
// passes over the bundled models. Only the first timestamp comment
// may vary; layout and top-level table ordering must never affect SQL bytes.
func TestExampleDeterminism(t *testing.T) {
	files, err := fs.Glob(examples.Models, "*.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			source, err := examples.Models.ReadFile(name)
			require.NoError(t, err)
			document, err := Decode(source)
			require.NoError(t, err)
			baseline, err := Generate(document, time.Time{})
			require.NoError(t, err)
			encoded, err := Encode(document)
			require.NoError(t, err)
			random := rand.New(rand.NewPCG(42, 97))
			for iteration := range 60 {
				// Decode afresh each time to cover persistence rather than only
				// generating repeatedly from one shared in-memory object.
				loaded, decodeErr := Decode(encoded)
				require.NoError(t, decodeErr)
				resaved, encodeErr := Encode(loaded)
				require.NoError(t, encodeErr)
				require.Equal(t, encoded, resaved, "JSON must be byte-stable on save %d", iteration)
				random.Shuffle(len(loaded.Tables), func(a, b int) { loaded.Tables[a], loaded.Tables[b] = loaded.Tables[b], loaded.Tables[a] })
				loaded.Viewport = Viewport{X: float64(iteration), Y: -float64(iteration), Zoom: 0.5}
				for i := range loaded.Tables {
					loaded.Tables[i].Position = Point{X: random.Float64() * 5000, Y: random.Float64() * 5000}
				}
				generated, generateErr := Generate(loaded, time.Unix(int64(iteration), int64(iteration)))
				require.NoError(t, generateErr)
				_, withoutTimestamp, found := bytes.Cut(generated, []byte("\n"))
				require.True(t, found)
				require.Equal(t, baseline, withoutTimestamp, "SQL changed at pass %d", iteration)
			}
			// Importing our own output must not normalize away a schema field.
			roundTrip, err := ImportSQL(string(baseline), name)
			require.NoError(t, err)
			again, err := Generate(roundTrip, time.Time{})
			require.NoError(t, err)
			require.Equal(t, baseline, again)
		})
	}
}

// TestRejectAmbiguousDocuments verifies AI-edit mistakes produce actionable
// failures instead of silently changing or deleting part of a saved model.
func TestRejectAmbiguousDocuments(t *testing.T) {
	base := `{"version":1,"name":"Example","tables":[],"groups":[],"viewport":{"x":0,"y":0,"zoom":1}}`
	for _, source := range []string{
		base + base, `null`, strings.Replace(base, `"version":1`, `"version":2`, 1),
		strings.Replace(base, `"name":"Example"`, `"name":"One","name":"Two"`, 1),
		strings.Replace(base, `"name":"Example"`, `"name":"One","Name":"Two"`, 1),
		strings.Replace(base, `"name":"Example"`, `"Name":"One"`, 1),
		strings.Replace(base, `"zoom":1`, `"zoom":0`, 1),
		strings.Replace(base, `"zoom":1`, `"zoom":1,"futureField":true`, 1),
		strings.Replace(base, `"name":"Example"`, `"name":"\ud800"`, 1),
		strings.Replace(base, `"name":"Example"`, `"name":"\udfff"`, 1),
		strings.Replace(base, `"name":"Example"`, `"name":"`+string([]byte{0xff})+`"`, 1),
	} {
		_, err := Decode([]byte(source))
		require.Error(t, err, source)
	}
	_, err := Decode([]byte(base))
	require.NoError(t, err)
	_, err = Decode([]byte(strings.Replace(base, `"name":"Example"`, `"name":"\ud83d\ude00"`, 1)))
	require.NoError(t, err, "paired Unicode surrogates are valid")
}

// TestRejectUnsupportedSQL ensures importing unknown MySQL features cannot yield
// a plausible-looking model from which those features would disappear on export.
func TestRejectUnsupportedSQL(t *testing.T) {
	for _, source := range []string{
		"CREATE VIEW v AS SELECT 1;",
		"CREATE TABLE t (id INT) ENGINE=MyISAM;",
		"CREATE TABLE t (id INT) PARTITION BY HASH(id) PARTITIONS 2;",
		"CREATE TABLE t (id INT) AUTO_INCREMENT=100;",
		"CREATE TABLE t (id INT ZEROFILL);",
		"CREATE TABLE t (id INT, KEY idx ((id+1)));",
		"CREATE TABLE t (id INT); INSERT INTO t VALUES (1);",
		"SET SQL_MODE='NO_BACKSLASH_ESCAPES'; CREATE TABLE t (id INT);",
		"CREATE TABLE t (id INT /*!80000 INVISIBLE */);",
		"CREATE TABLE t (id INT); CREATE TABLE t (other INT);",
	} {
		_, err := ImportSQL(source, "Unsupported")
		require.Error(t, err, source)
	}
}

// TestStableReferencesAndValidation renames SQL identifiers independently of IDs,
// then introduces broken references and fragment injection to exercise failures.
func TestStableReferencesAndValidation(t *testing.T) {
	source := "CREATE TABLE parent (id BIGINT NOT NULL, PRIMARY KEY(id)); CREATE TABLE child (id BIGINT NOT NULL, parent_id BIGINT NULL, PRIMARY KEY(id), KEY parent_idx(parent_id), CONSTRAINT child_parent FOREIGN KEY(parent_id) REFERENCES parent(id));"
	document, err := ImportSQL(source, "References")
	require.NoError(t, err)
	document.Tables[0].Name = "renamed_parent"
	document.Tables[0].Columns[0].Name = "renamed_id"
	generated, err := Generate(document, time.Time{})
	require.NoError(t, err)
	require.Contains(t, string(generated), "REFERENCES `renamed_parent` (`renamed_id`)")
	encoded, err := Encode(document)
	require.NoError(t, err)
	mutations := map[string]func(*Document){
		"missing table":  func(d *Document) { d.Tables = d.Tables[1:] },
		"missing column": func(d *Document) { d.Tables[1].ForeignKeys[0].ReferencedColumns[0] = "missing" },
		"duplicate name": func(d *Document) { d.Tables[1].Name = "RENAMED_PARENT" },
		"duplicate ID":   func(d *Document) { d.Tables[1].Columns[1].ID = d.Tables[1].Columns[0].ID },
		"set null nonnullable": func(d *Document) {
			d.Tables[1].ForeignKeys[0].OnDelete = "SET NULL"
			d.Tables[1].Columns[1].Nullable = false
		},
		"type fragment":         func(d *Document) { d.Tables[0].Columns[0].Type = "BIGINT, injected INT" },
		"type attributes":       func(d *Document) { d.Tables[0].Columns[0].Type = "BIGINT DEFAULT 42" },
		"default fragment":      func(d *Document) { value := "1 COMMENT 'changed'"; d.Tables[0].Columns[0].Default = &value },
		"subquery":              func(d *Document) { value := "(SELECT 1)"; d.Tables[0].Columns[0].Default = &value },
		"generated reference":   func(d *Document) { d.Tables[0].Columns[0].Generated = "missing + 1" },
		"incompatible FK sign":  func(d *Document) { d.Tables[0].Columns[0].Type = "BIGINT UNSIGNED" },
		"missing target key":    func(d *Document) { d.Tables[0].Indexes = []Index{} },
		"null default required": func(d *Document) { value := "(NULL)"; d.Tables[0].Columns[0].Default = &value },
		"reserved index name":   func(d *Document) { d.Tables[0].Indexes[0].Kind = "UNIQUE" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			loaded, decodeErr := Decode(encoded)
			require.NoError(t, decodeErr)
			mutate(loaded)
			_, generateErr := Generate(loaded, time.Time{})
			require.Error(t, generateErr)
		})
	}
}
