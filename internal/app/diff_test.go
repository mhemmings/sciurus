package app

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhemmings/sciurus/internal/model"
	"github.com/stretchr/testify/require"
)

func TestDiffMixedInputsAndExitCodes(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	sqlPath := filepath.Join(directory, "before.sql")
	erPath := filepath.Join(directory, "diagram.er.json")
	changedPath := filepath.Join(directory, "after.sql")
	source := "CREATE TABLE widgets (id INT NOT NULL, name VARCHAR(64) NOT NULL DEFAULT 'new', PRIMARY KEY (id));"
	require.NoError(t, os.WriteFile(sqlPath, []byte(source), 0600))
	require.NoError(t, os.WriteFile(changedPath, []byte(strings.Replace(source, "'new'", "'ready'", 1)), 0600))
	document, err := model.ImportSQL(source, "A diagram title")
	require.NoError(t, err)
	document.Tables[0].ID = "editor-generated-id"
	document.Tables[0].Columns[0].ID = "editor-column-id"
	document.Tables[0].Indexes[0].ID = "editor-index-id"
	document.Tables[0].Indexes[0].Columns[0].Column = "editor-column-id"
	document.Tables[0].Position = model.Point{X: 876, Y: -125}
	document.Tables[0].Width = 500
	encoded, err := model.Encode(document)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(erPath, encoded, 0600))

	for _, inputs := range [][2]string{{erPath, sqlPath}, {sqlPath, erPath}, {erPath, erPath}, {sqlPath, sqlPath}} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"diff", "--exit-code", inputs[0], inputs[1]}, &stdout, &stderr)
		require.Equal(t, 0, code, stderr.String())
		require.Equal(t, "No database changes.\n", stdout.String())
		require.Empty(t, stderr.String())
	}
	for _, exitCode := range []bool{false, true} {
		var stdout, stderr bytes.Buffer
		args := []string{"diff"}
		expected := 0
		if exitCode {
			args = append(args, "--exit-code")
			expected = 1
		}
		args = append(args, erPath, changedPath)
		code := Run(context.Background(), args, &stdout, &stderr)
		require.Equal(t, expected, code, stderr.String())
		require.Contains(t, stdout.String(), "TABLE widgets (modified)")
		require.Contains(t, stdout.String(), "'new'")
		require.Contains(t, stdout.String(), "'ready'")
		require.Empty(t, stderr.String())
	}

	// CI receives the image even when --exit-code signals a schema mismatch.
	imagePath := filepath.Join(directory, "diff.png")
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"diff", "--exit-code", "--format", "png", "--output", imagePath, erPath, changedPath}, &stdout, &stderr)
	require.Equal(t, 1, code, stderr.String())
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
	data, err := os.ReadFile(imagePath)
	require.NoError(t, err)
	configuration, err := png.DecodeConfig(bytes.NewReader(data))
	require.NoError(t, err)
	require.Greater(t, configuration.Width, 0)
	require.Greater(t, configuration.Height, 0)

	// Failed inputs or output collisions must not replace either input or an
	// existing report, and must be distinguishable from legitimate differences.
	brokenPath := filepath.Join(directory, "broken.sql")
	require.NoError(t, os.WriteFile(brokenPath, []byte("DROP TABLE widgets;"), 0600))
	for _, args := range [][]string{
		{"diff", "--output", imagePath, erPath, brokenPath},
		{"diff", "--output", erPath, erPath, changedPath},
		{"diff", "--output", changedPath, erPath, changedPath},
		{"diff", "--format", "invalid", erPath, changedPath},
		{"diff", erPath},
	} {
		stdout.Reset()
		stderr.Reset()
		code = Run(context.Background(), args, &stdout, &stderr)
		require.Equal(t, 2, code)
		require.NotEmpty(t, stderr.String())
		require.Empty(t, stdout.String())
	}
	after, err := os.ReadFile(imagePath)
	require.NoError(t, err)
	require.Equal(t, data, after)
	after, err = os.ReadFile(erPath)
	require.NoError(t, err)
	require.Equal(t, encoded, after)
	after, err = os.ReadFile(changedPath)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(source, "'new'", "'ready'", 1), string(after))
}

func TestDiffEmptySchema(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	erPath := filepath.Join(directory, "empty.er.json")
	sqlPath := filepath.Join(directory, "empty.sql")
	data, err := model.Encode(model.New("Empty"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(erPath, data, 0600))
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"generate", "--output", sqlPath, erPath}, &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	code = Run(context.Background(), []string{"diff", "--exit-code", erPath, sqlPath}, &stdout, &stderr)
	require.Equal(t, 0, code, stderr.String())
	require.Equal(t, "No database changes.\n", stdout.String())
}
