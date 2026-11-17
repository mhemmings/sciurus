package app

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhemmings/sciurus/internal/model"
)

// TestCLIRoundTrip exercises headless import, validation, reproducible generation,
// and atomic output through the public command runner rather than its helpers.
func TestCLIRoundTrip(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	sqlPath := filepath.Join(directory, "input.sql")
	modelPath := filepath.Join(directory, "diagram.er.json")
	outputPath := filepath.Join(directory, "schema.sql")
	err := os.WriteFile(sqlPath, []byte("CREATE TABLE widgets (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, name VARCHAR(64) NOT NULL DEFAULT 'new', PRIMARY KEY (id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;"), 0600)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"import", "--output", modelPath, "--name", "shop", sqlPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("import: %d %s", code, &stderr)
	}
	code = Run(context.Background(), []string{"validate", modelPath}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "Valid: shop (1 tables") {
		t.Fatalf("validate: %d %s %s", code, &stdout, &stderr)
	}
	// An accidentally repeated output path must not replace the ER source with
	// generated SQL; aliases should receive the same protection as exact paths.
	before, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	code = Run(context.Background(), []string{"generate", "--output", modelPath, modelPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("generation accepted its own input as the output path")
	}
	after, err := os.ReadFile(modelPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("output path collision changed the source: %v", err)
	}
	// With no timestamp, stdout and an explicit filesystem destination produce
	// the same SQL. The model tests exercise repeated generation across schemas.
	stdout.Reset()
	code = Run(context.Background(), []string{"generate", "--no-timestamp", modelPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("generate: %d %s", code, &stderr)
	}
	baseline := bytes.Clone(stdout.Bytes())
	if bytes.Contains(baseline, []byte("Generated at")) || !bytes.Contains(baseline, []byte("CREATE TABLE `widgets`")) {
		t.Fatalf("unexpected SQL output: %s", baseline)
	}
	code = Run(context.Background(), []string{"generate", "--output", outputPath, "--no-timestamp", modelPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("generate to file: %d %s", code, &stderr)
	}
	output, err := os.ReadFile(outputPath)
	if err != nil || !bytes.Equal(output, baseline) {
		t.Fatalf("file and stdout generation differ: %v", err)
	}
	// A broken input must leave an already generated schema untouched.
	err = os.WriteFile(modelPath, []byte(`{"version":999}`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	code = Run(context.Background(), []string{"generate", "--output", outputPath, modelPath}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("invalid model generated successfully")
	}
	output, err = os.ReadFile(outputPath)
	if err != nil || !bytes.Equal(output, baseline) {
		t.Fatalf("failed generation clobbered the existing schema: %v", err)
	}
}

func TestCLIWorkbenchImport(t *testing.T) {
	t.Parallel()
	xml, err := os.ReadFile("../testing/testdata/workbench-schema.xml")
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("document.mwb.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = entry.Write(xml)
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(t.TempDir(), "diagram.MWB")
	err = os.WriteFile(inputPath, archive.Bytes(), 0600)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"import", "--name", "workshop", inputPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("import: %d %s", code, &stderr)
	}
	document, err := model.Decode(stdout.Bytes())
	if err != nil || document.Name != "workshop" {
		t.Fatalf("imported ER JSON: %v %s", err, &stdout)
	}
	stdout.Reset()
	code = Run(context.Background(), []string{"import", "--output", inputPath, inputPath}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "output") {
		t.Fatalf("input/output collision: %d %s", code, &stderr)
	}
	after, err := os.ReadFile(inputPath)
	if err != nil || !bytes.Equal(after, archive.Bytes()) {
		t.Fatalf("import changed the Workbench input: %v", err)
	}
}

func TestCLIImportFileTypes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"import", "schema.txt"}, want: ".sql or .mwb"},
		{args: []string{"import", "--workbench", "layout.mwb", "diagram.mwb"}, want: "already includes its layout"},
	} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), test.args, &stdout, &stderr)
		if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), test.want) {
			t.Fatalf("arguments %v: code %d, stdout %q, stderr %q", test.args, code, &stdout, &stderr)
		}
	}
}

func TestCLIPositionalInputArity(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"generate"},
		{"validate"},
		{"import"},
		{"generate", "first.er.json", "second.er.json"},
		{"validate", "first.er.json", "second.er.json"},
		{"import", "first.sql", "second.sql"},
		{"serve", "first.er.json", "second.er.json"},
		{"first.er.json", "second.er.json"},
	} {
		var stdout, stderr bytes.Buffer
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		code := Run(ctx, args, &stdout, &stderr)
		if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage:") {
			t.Fatalf("arguments %v: code %d, stdout %q, stderr %q", args, code, &stdout, &stderr)
		}
	}
}

func TestCLIEditorLoadsPositionalModel(t *testing.T) {
	t.Parallel()
	modelPath := filepath.Join(t.TempDir(), "invalid.er.json")
	err := os.WriteFile(modelPath, []byte(`{"version":999}`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--addr", "127.0.0.1:0", modelPath},
		{"serve", "--addr", "127.0.0.1:0", modelPath},
	} {
		var stdout, stderr bytes.Buffer
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		code := Run(ctx, args, &stdout, &stderr)
		if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "read model:") {
			t.Fatalf("arguments %v: code %d, stdout %q, stderr %q", args, code, &stdout, &stderr)
		}
	}
}

// TestCLIRejectsNetworkExposure ensures no flag can accidentally expose the
// filesystem-writing editor to a LAN, wildcard socket, or arbitrary DNS name.
func TestCLIRejectsNetworkExposure(t *testing.T) {
	t.Parallel()
	for _, address := range []string{":7480", "0.0.0.0:7480", "[::]:7480", "192.168.1.2:7480", "example.com:7480"} {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"serve", "--addr", address}, &stdout, &stderr)
		if code == 0 || !strings.Contains(stderr.String(), "loopback") {
			t.Fatalf("unsafe address %q was not rejected: %d %s", address, code, &stderr)
		}
	}
}

func TestCLIVersion(t *testing.T) {
	previousVersion := version
	t.Cleanup(func() { version = previousVersion })
	for _, test := range []struct {
		version string
		want    string
	}{
		{version: "", want: "Sciurus dev\n"},
		{version: "v1.2.3", want: "Sciurus v1.2.3\n"},
	} {
		version = test.version
		for _, args := range [][]string{{"version"}, {"--version"}} {
			var stdout, stderr bytes.Buffer
			code := Run(context.Background(), args, &stdout, &stderr)
			if code != 0 || stdout.String() != test.want || stderr.Len() != 0 {
				t.Fatalf("version %q with %v: code %d, stdout %q, stderr %q", test.version, args, code, &stdout, &stderr)
			}
		}
	}
}
