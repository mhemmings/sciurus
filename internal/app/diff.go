package app

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mhemmings/sciurus/internal/diffimage"
	"github.com/mhemmings/sciurus/internal/model"
)

// runDiff compares the database definitions, independently of their input format
// or diagram layout. A true result requests the optional differences exit code;
// an error always takes precedence, including an unsuccessful report write.
func runDiff(args []string, stdout, stderr io.Writer) (bool, error) {
	flags := flag.NewFlagSet("diff", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "report format: text or png")
	output := flags.String("output", "-", "report output file, or - for stdout")
	exitCode := flags.Bool("exit-code", false, "exit 1 when database definitions differ (errors exit 2)")
	if err := flags.Parse(args); err != nil {
		return false, err
	}
	if flags.NArg() != 2 {
		return false, errors.New("usage: sciurus diff [--format text|png] [--output FILE] [--exit-code] BEFORE AFTER")
	}
	if *format != "text" && *format != "png" {
		return false, fmt.Errorf("unsupported diff format %q; use text or png", *format)
	}
	inputs := flags.Args()
	var documents [2]*model.Document
	for i, path := range inputs {
		data, _, err := readFile(path)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", path, err)
		}
		if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
			documents[i], err = model.Decode(data)
		} else {
			documents[i], err = model.ImportSchemaSQL(string(data), "schema")
		}
		if err != nil {
			return false, fmt.Errorf("read %s: %w", path, err)
		}
	}
	outputPath := *output
	if outputPath != "-" {
		var err error
		outputPath, err = protectedOutputPath(outputPath, inputs...)
		if err != nil {
			return false, err
		}
	}
	diff, err := model.Compare(documents[0], documents[1])
	if err != nil {
		return false, err
	}
	var data []byte
	if *format == "png" {
		data, err = diffimage.Render(diff, filepath.Base(inputs[0]), filepath.Base(inputs[1]))
		if err != nil {
			return false, err
		}
	} else {
		var report strings.Builder
		if !diff.HasChanges() {
			report.WriteString("No database changes.\n")
		} else {
			fmt.Fprintf(&report, "--- %s\n+++ %s\n", inputs[0], inputs[1])
			for _, table := range diff.Tables {
				fmt.Fprintf(&report, "\nTABLE %s (%s)\n", table.Name, table.Kind)
				for _, change := range table.Changes {
					fmt.Fprintf(&report, "  %s", strings.ToUpper(change.Kind))
					if change.Name != "" {
						fmt.Fprintf(&report, " %s", change.Name)
					}
					report.WriteByte('\n')
					for i, definition := range []string{change.Before, change.After} {
						if definition == "" {
							continue
						}
						prefix := "    - "
						if i == 1 {
							prefix = "    + "
						}
						for _, line := range strings.Split(definition, "\n") {
							report.WriteString(prefix + line + "\n")
						}
					}
				}
			}
		}
		data = []byte(report.String())
	}
	if outputPath == "-" {
		_, err = stdout.Write(data)
	} else {
		_, err = writeFile(outputPath, data, nil)
	}
	return *exitCode && diff.HasChanges(), err
}
