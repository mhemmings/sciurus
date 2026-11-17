package testing_test

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mhemmings/sciurus/internal/model"
	"github.com/stretchr/testify/require"
)

func TestWorkbenchSchemaImport(t *testing.T) {
	xml, err := os.ReadFile("testdata/workbench-schema.xml")
	require.NoError(t, err)
	source, err := os.ReadFile("testdata/workbench-schema.sql")
	require.NoError(t, err)
	expected, err := model.ImportSQL(string(source), "Ledger")
	require.NoError(t, err)
	document, err := model.ImportWorkbench(workbenchArchive(t, string(xml)), "Ledger")
	require.NoError(t, err)

	expectedSQL, err := model.Generate(expected, time.Time{})
	require.NoError(t, err)
	actualSQL, err := model.Generate(document, time.Time{})
	require.NoError(t, err)
	require.Equal(t, string(expectedSQL), string(actualSQL))
	require.Equal(t, "Ledger", document.Name)
	require.Equal(t, "utf8mb4", document.Charset)
	require.Equal(t, "utf8mb4_bin", document.Collation)
	require.Len(t, document.Tables, 2)
	require.Len(t, document.Tables[0].ForeignKeys, 1)
	require.Equal(t, []model.Group{{
		ID: "group_layerledger", Name: "Ledger", Color: "#34a8b0",
		Position: model.Point{X: 200, Y: 300}, Width: 1200, Height: 900,
	}}, document.Groups)
	require.Equal(t, model.Point{X: 230, Y: 340}, document.Tables[0].Position)
	require.Equal(t, model.Point{X: 800, Y: 340}, document.Tables[1].Position)
	require.Equal(t, model.Viewport{X: -100, Y: -60, Zoom: 0.8}, document.Viewport)

	encoded, err := model.Encode(document)
	require.NoError(t, err)
	reloaded, err := model.Decode(encoded)
	require.NoError(t, err)
	roundTripSQL, err := model.Generate(reloaded, time.Time{})
	require.NoError(t, err)
	require.Equal(t, actualSQL, roundTripSQL)

	schemaOnly := strings.SplitN(string(xml), `<value type="list" key="diagrams">`, 2)[0] +
		"</value></value></value></data>"
	withoutDiagram, err := model.ImportWorkbench(workbenchArchive(t, schemaOnly), "Ledger")
	require.NoError(t, err)
	schemaSQL, err := model.Generate(withoutDiagram, time.Time{})
	require.NoError(t, err)
	require.Equal(t, expectedSQL, schemaSQL)
	require.Empty(t, withoutDiagram.Groups)
	require.Equal(t, model.Point{X: 380, Y: 0}, withoutDiagram.Tables[1].Position)
}

func TestWorkbenchSchemaRejectsUnsupportedOrBrokenDefinitions(t *testing.T) {
	xml, err := os.ReadFile("testdata/workbench-schema.xml")
	require.NoError(t, err)
	for name, invalid := range map[string]string{
		"dangling index column": strings.Replace(string(xml), `key="referencedColumn">account-code`, `key="referencedColumn">missing-column`, 1),
		"partitioned table":     strings.Replace(string(xml), `<value key="comment">Accounts</value>`, `<value key="partitionType">HASH</value>`, 1),
		"view":                  strings.Replace(string(xml), `<value type="list" key="views"/>`, `<value type="list" key="views"><value struct-name="db.mysql.View" id="view"><value key="name">recent_accounts</value></value></value>`, 1),
		"multiple schemas":      strings.Replace(string(xml), `<value type="list" key="schemata">`, `<value type="list" key="schemata"><value struct-name="db.mysql.Schema" id="another-schema"><value key="name">another</value></value>`, 1),
		"multiple diagrams":     strings.Replace(string(xml), `<value type="list" key="diagrams">`, `<value type="list" key="diagrams"><value struct-name="workbench.physical.Diagram" id="another-diagram"/>`, 1),
		"type adds a column":    strings.Replace(string(xml), `<value key="datatypeExplicitParams">('new','it''s ready')</value>`, `<value key="datatypeExplicitParams">('new'), injected INT</value>`, 1),
		"default adds a column": strings.Replace(string(xml), `<value key="defaultValue">-1.250</value>`, `<value key="defaultValue">0, injected INT</value>`, 1),
		"default adds a key":    strings.Replace(string(xml), `<value key="defaultValue">-1.250</value>`, `<value key="defaultValue">0 UNIQUE</value>`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			document, err := model.ImportWorkbench(workbenchArchive(t, invalid), "Ledger")
			require.Error(t, err)
			require.Nil(t, document)
		})
	}
}

// TestWorkbenchLayout uses a self-authored archive to verify layer-relative
// offsets, stable serialization, and separation of schema from presentation.
func TestWorkbenchLayout(t *testing.T) {
	source := "CREATE TABLE project (id INT NOT NULL, PRIMARY KEY (id));" +
		"CREATE TABLE task (id INT NOT NULL, project_id INT NOT NULL, PRIMARY KEY (id), " +
		"KEY task_project (project_id), CONSTRAINT task_project_fk FOREIGN KEY (project_id) REFERENCES project(id));" +
		"CREATE TABLE label (id INT NOT NULL, PRIMARY KEY (id));"
	doc, err := model.ImportSQL(source, "Planning")
	require.NoError(t, err)
	before, err := model.Generate(doc, time.Time{})
	require.NoError(t, err)
	xml, err := os.ReadFile("testdata/workbench.xml")
	require.NoError(t, err)
	archive := workbenchArchive(t, string(xml))
	err = model.ApplyWorkbenchLayout(doc, archive)
	require.NoError(t, err)
	require.Equal(t, []model.Group{{
		ID: "group_layerplanning", Name: "Planning", Color: "#34a8b0",
		Position: model.Point{X: 200, Y: 300}, Width: 900, Height: 600,
	}}, doc.Groups)
	require.Equal(t, model.Viewport{X: -100, Y: -60, Zoom: 0.8}, doc.Viewport)
	require.Len(t, doc.Tables, 3)
	for i, expected := range []struct {
		name     string
		position model.Point
		width    float64
		group    string
	}{
		{name: "project", position: model.Point{X: 230, Y: 340}, width: 300, group: "group_layerplanning"},
		{name: "task", position: model.Point{X: 580, Y: 380}, width: 340, group: "group_layerplanning"},
		{name: "label", position: model.Point{X: 860, Y: 100}, width: 280},
	} {
		table := doc.Tables[i]
		require.Equal(t, expected.name, table.Name)
		require.Equal(t, expected.position, table.Position, table.Name)
		require.Equal(t, expected.width, table.Width, table.Name)
		require.Equal(t, expected.group, table.GroupID, table.Name)
	}
	// The XML contains an extra table and an FK absent from SQL. Importing its
	// presentation must neither create them nor change the real FK's semantics.
	require.Len(t, doc.Tables[1].ForeignKeys, 1)
	after, err := model.Generate(doc, time.Time{})
	require.NoError(t, err)
	require.Equal(t, before, after)
	encoded, err := model.Encode(doc)
	require.NoError(t, err)
	reloaded, err := model.Decode(encoded)
	require.NoError(t, err)
	require.Equal(t, doc, reloaded)
	// Reapplying the same archive preserves group IDs and ordering.
	err = model.ApplyWorkbenchLayout(reloaded, archive)
	require.NoError(t, err)
	require.Equal(t, doc, reloaded)
}

// TestInvalidWorkbenchIsAtomic checks both archive errors and failures after
// valid groups and figures have been read cannot partially replace a model.
func TestInvalidWorkbenchIsAtomic(t *testing.T) {
	doc, err := model.ImportSQL("CREATE TABLE task (id INT NOT NULL, PRIMARY KEY (id));", "example")
	require.NoError(t, err)
	doc.Groups = []model.Group{{ID: "existing", Name: "Existing", Color: "#334455", Position: model.Point{X: 20, Y: 30}, Width: 500, Height: 400}}
	doc.Tables[0].GroupID = "existing"
	before, err := model.Encode(doc)
	require.NoError(t, err)
	xml, err := os.ReadFile("testdata/workbench.xml")
	require.NoError(t, err)
	for name, invalid := range map[string][]byte{
		"not a zip":          []byte("not a zip"),
		"missing XML":        workbenchArchive(t),
		"malformed XML":      workbenchArchive(t, "<data><value>broken"),
		"duplicate XML":      workbenchArchive(t, string(xml), string(xml)),
		"missing layer":      workbenchArchive(t, strings.ReplaceAll(string(xml), `key="layer">layer-planning`, `key="layer">missing`)),
		"invalid viewport":   workbenchArchive(t, strings.Replace(string(xml), `key="zoom">0.8`, `key="zoom">NaN`, 1)),
		"invalid table size": workbenchArchive(t, strings.Replace(string(xml), `key="width">340`, `key="width">-1`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, model.ApplyWorkbenchLayout(doc, invalid))
			after, encodeErr := model.Encode(doc)
			require.NoError(t, encodeErr)
			require.Equal(t, before, after)
		})
	}
}

// workbenchArchive constructs an in-memory archive, including deliberate missing
// or duplicate document entries, without a dependency on the Workbench desktop app.
func workbenchArchive(t *testing.T, documents ...string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, document := range documents {
		entry, err := archive.Create("document.mwb.xml")
		require.NoError(t, err)
		_, err = entry.Write([]byte(document))
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	return buffer.Bytes()
}
