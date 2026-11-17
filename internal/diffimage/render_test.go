package diffimage

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/mhemmings/sciurus/internal/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/font"
)

func TestRenderPortablePNG(t *testing.T) {
	before, err := model.ImportSQL(`
CREATE TABLE owners (id int NOT NULL PRIMARY KEY);
CREATE TABLE accounts (id int NOT NULL PRIMARY KEY, owner_id int, email varchar(100), obsolete int,
  CONSTRAINT fk_owner FOREIGN KEY (owner_id) REFERENCES owners(id));
CREATE TABLE legacy_tokens (token varchar(32) NOT NULL PRIMARY KEY);
CREATE TABLE unrelated (id int NOT NULL PRIMARY KEY);`, "before")
	require.NoError(t, err)
	after, err := model.ImportSQL(`
CREATE TABLE owners (id int NOT NULL PRIMARY KEY);
CREATE TABLE accounts (id int NOT NULL PRIMARY KEY, owner_id int, email varchar(255), last_active timestamp,
  CONSTRAINT fk_owner FOREIGN KEY (owner_id) REFERENCES owners(id) ON DELETE CASCADE);
CREATE TABLE sessions (token varchar(255) NOT NULL PRIMARY KEY, account_id int,
  CONSTRAINT fk_account FOREIGN KEY (account_id) REFERENCES accounts(id));
CREATE TABLE unrelated (id int NOT NULL PRIMARY KEY);`, "after")
	require.NoError(t, err)
	diff, err := model.Compare(before, after)
	require.NoError(t, err)
	first, err := Render(diff, "before.er.json", "after.sql")
	require.NoError(t, err)
	decoded, err := png.Decode(bytes.NewReader(first))
	require.NoError(t, err)
	require.Greater(t, decoded.Bounds().Dx(), 1000)
	require.Greater(t, decoded.Bounds().Dy(), 600)
	second, err := Render(diff, "before.er.json", "after.sql")
	require.NoError(t, err)
	require.Equal(t, first, second, "CI reports must not vary between identical runs")

	empty, err := Render(&model.Diff{}, "before.er.json", "after.sql")
	require.NoError(t, err)
	emptyDecoded, err := png.Decode(bytes.NewReader(empty))
	require.NoError(t, err)
	require.Less(t, emptyDecoded.Bounds().Dy(), decoded.Bounds().Dy())

	fonts, err := loadFonts()
	require.NoError(t, err)
	tables, edges, err := buildDiagram(diff, fonts)
	require.NoError(t, err)
	byName := map[string]*diagramTable{}
	for _, table := range tables {
		byName[table.Name] = table
	}
	require.Contains(t, byName, "owners", "unchanged relationship targets provide context")
	require.NotContains(t, byName, "unrelated", "unrelated unchanged schema must not overwhelm the diff")
	require.Empty(t, byName["owners"].Status)
	require.Len(t, edges, 2)
	require.Equal(t, "modified", edges[0].Status)
	columns := map[string]diagramColumn{}
	for _, column := range byName["accounts"].Columns {
		columns[column.Name] = column
	}
	require.Equal(t, "removed", columns["obsolete"].Status)
	require.Equal(t, "added", columns["last_active"].Status)
	require.Contains(t, strings.Join(columns["email"].TypeLines, ""), "varchar(100) → varchar(255)")
}

func TestRelationshipAndKeyChangesRemainVisible(t *testing.T) {
	before, err := model.ImportSQL(`
CREATE TABLE owners (id int NOT NULL PRIMARY KEY);
CREATE TABLE teams (id int NOT NULL PRIMARY KEY);
CREATE TABLE accounts (id int NOT NULL PRIMARY KEY, owner_id int, team_id int,
  CONSTRAINT fk_owner FOREIGN KEY (owner_id) REFERENCES owners(id),
  CONSTRAINT fk_team FOREIGN KEY (team_id) REFERENCES teams(id));`, "before")
	require.NoError(t, err)
	after, err := model.ImportSQL(`
CREATE TABLE owners (id int NOT NULL PRIMARY KEY);
CREATE TABLE teams (id int NOT NULL PRIMARY KEY);
CREATE TABLE accounts (id int NOT NULL PRIMARY KEY, owner_id int, team_id int,
  UNIQUE KEY uq_owner(owner_id),
  CONSTRAINT fk_owner FOREIGN KEY (owner_id) REFERENCES owners(id),
  CONSTRAINT fk_team FOREIGN KEY (team_id) REFERENCES owners(id));`, "after")
	require.NoError(t, err)
	diff, err := model.Compare(before, after)
	require.NoError(t, err)
	fonts, err := loadFonts()
	require.NoError(t, err)
	tables, edges, err := buildDiagram(diff, fonts)
	require.NoError(t, err)
	require.Len(t, edges, 3, "a moved relationship needs both the old and new edge")
	require.Equal(t, "modified", edges[0].Status)
	require.True(t, edges[0].UniqueSource, "unique indexes must update relationship cardinality")
	require.Equal(t, "removed", edges[1].Status)
	require.Equal(t, "teams", edges[1].To)
	require.Equal(t, "added", edges[2].Status)
	require.Equal(t, "owners", edges[2].To)
	notes := []string{}
	for _, note := range tables[0].Notes {
		notes = append(notes, note.Lines...)
	}
	require.Contains(t, strings.Join(notes, ""), "1:N → 1:1")
}

func TestNullableCompositeRelationshipIsOptional(t *testing.T) {
	beforeSQL := `CREATE TABLE parents (id int NOT NULL, revision int NOT NULL, PRIMARY KEY(id, revision));
CREATE TABLE children (parent_id int NOT NULL, revision int NOT NULL,
  CONSTRAINT fk_parent FOREIGN KEY (parent_id, revision) REFERENCES parents(id, revision));`
	before, err := model.ImportSQL(beforeSQL, "before")
	require.NoError(t, err)
	after, err := model.ImportSQL(strings.Replace(beforeSQL, "parent_id int NOT NULL", "parent_id int NULL", 1), "after")
	require.NoError(t, err)
	diff, err := model.Compare(before, after)
	require.NoError(t, err)
	fonts, err := loadFonts()
	require.NoError(t, err)
	tables, edges, err := buildDiagram(diff, fonts)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	require.True(t, edges[0].OptionalReference, "one nullable component makes a MATCH SIMPLE reference optional")
	require.Equal(t, "modified", edges[0].Status, "changing participation must highlight the relationship even when FK SQL is unchanged")
	notes := []string{}
	for _, note := range tables[0].Notes {
		notes = append(notes, note.Lines...)
	}
	require.Contains(t, strings.Join(notes, ""), "reference required → optional")
}

func TestWrapPreservesLongDefinitionsAndUnsupportedCharacters(t *testing.T) {
	fonts, err := loadFonts()
	require.NoError(t, err)
	defer func() {
		for _, face := range []font.Face{fonts.title, fonts.table, fonts.body, fonts.bold, fonts.small, fonts.mono} {
			_ = face.Close()
		}
	}()
	definition := "`value` varchar(255) DEFAULT '" + strings.Repeat("no_spaces_", 60) + "' COMMENT 'customer supplied value'"
	lines, err := wrapText(definition, fonts.mono, 300)
	require.NoError(t, err)
	require.Greater(t, len(lines), 10)
	require.Equal(t, definition, strings.Join(lines, ""), "wrapping must retain every character, including spaces")
	for _, line := range lines {
		require.LessOrEqual(t, font.MeasureString(fonts.mono, line).Ceil(), 300)
	}

	lines, err = wrapText("`客户` varchar(255) COMMENT '😀'", fonts.mono, 900)
	require.NoError(t, err)
	require.Equal(t, "`\\u5BA2\\u6237` varchar(255) COMMENT '\\U0001F600'", strings.Join(lines, ""), "unsupported identifiers must remain distinguishable")
	lines, err = wrapText(`literal \u5BA2`, fonts.mono, 900)
	require.NoError(t, err)
	require.Equal(t, `literal \\u5BA2`, strings.Join(lines, ""), "literal escape sequences must remain distinct from font fallbacks")
	lines, err = wrapText("first\n\nlast", fonts.mono, 900)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "", "last"}, lines)
}

func TestRenderRejectsOversizedReportsRatherThanCropping(t *testing.T) {
	for _, definition := range []string{
		strings.Repeat("long_value", 10000),
		strings.Repeat("\n", 20000),
	} {
		diff := &model.Diff{
			Tables:      []model.TableDiff{{Name: "large", Kind: "added"}},
			AfterSchema: model.SchemaGraph{Tables: []model.SchemaTable{{Name: "large", Columns: []model.SchemaColumn{{Name: "value", Type: "text", Attributes: map[string]string{"comment": definition}}}}}},
		}
		output, err := Render(diff, "before.sql", "after.sql")
		require.ErrorContains(t, err, "use text output")
		require.Empty(t, output)
	}
}
