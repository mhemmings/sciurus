package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/mhemmings/sciurus/internal/model"
)

// version may be set at build time using -ldflags to identify a distributed
// executable without requiring the frontend's package metadata at runtime.
var version string

// Run executes the command line with injectable output and cancellation. Zero
// indicates success; invalid input or runtime failure returns a nonzero status.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	command := "serve"
	if len(args) > 0 {
		switch args[0] {
		case "serve", "generate", "validate", "import", "diff", "help", "version":
			command = args[0]
			args = args[1:]
		}
	}
	if command == "help" || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		fmt.Fprint(stdout, `Sciurus — a local MySQL ER designer

Usage:
  sciurus [serve] [--addr 127.0.0.1:7480] [diagram.er.json]
  sciurus generate [--output schema.sql] [--no-timestamp] diagram.er.json
  sciurus validate diagram.er.json
  sciurus import [--output diagram.er.json] [--name schema] schema.sql|diagram.mwb
  sciurus diff [--format text|png] [--output FILE] [--exit-code] BEFORE AFTER
  sciurus version

The editor and examples are embedded; no separate frontend or internet is needed.
The server binds only to loopback. Use --addr 127.0.0.1:0 for an available port.
With a model filename, Save updates that file atomically and detects external changes.
A nonexistent model file is created on first Save. Without a filename, use the
browser's Open/Save controls. Generate, import and diff default to standard output;
--output - also selects standard output. SQL is never executed by this tool.
Place options before filenames; use -- before a filename starting with a dash.
Import selects SQL or Workbench from the .sql or .mwb filename and writes ER JSON.
For SQL input, --workbench diagram.mwb optionally supplies positions and groups.
Diff accepts ER JSON or supported MySQL CREATE TABLE SQL in either position.
Diff ignores diagram metadata. With --exit-code, changes return 1; errors return 2.
`)
		return 0
	}
	if command == "version" || (command == "serve" && len(args) == 1 && args[0] == "--version") {
		buildVersion := version
		if buildVersion == "" {
			buildVersion = "dev"
			if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
				buildVersion = info.Main.Version
			}
		}
		fmt.Fprintf(stdout, "Sciurus %s\n", buildVersion)
		return 0
	}
	var err error
	var exitForChanges bool
	switch command {
	case "serve":
		err = runServe(ctx, args, stdout, stderr)
	case "generate":
		err = runGenerate(args, stdout, stderr)
	case "validate":
		err = runValidate(args, stdout, stderr)
	case "import":
		err = runImport(args, stdout, stderr)
	case "diff":
		exitForChanges, err = runDiff(args, stdout, stderr)
	}
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "sciurus: %v\n", err)
		if command == "diff" {
			return 2
		}
		return 1
	}
	if exitForChanges {
		return 1
	}
	return 0
}

// runServe starts an HTTP server only on an explicit loopback address, printing
// the chosen URL after listening so ephemeral ports are reported accurately.
func runServe(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	address := flags.String("addr", "127.0.0.1:7480", "loopback host:port (port 0 selects an available port)")
	err := flags.Parse(args)
	if err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("usage: sciurus [serve] [--addr ADDRESS] [MODEL]")
	}
	modelPath := flags.Arg(0)
	host, port, err := net.SplitHostPort(*address)
	if err != nil {
		return fmt.Errorf("--addr must be a loopback host:port: %w", err)
	}
	if strings.EqualFold(host, "localhost") {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("--addr must use a loopback IP or localhost; network exposure is not supported")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	defer listener.Close()
	handler, err := newHandler(modelPath, listener.Addr().String())
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownContext)
		case <-stopped:
		}
	}()
	fmt.Fprintf(stdout, "Sciurus → http://%s\n", listener.Addr())
	if modelPath == "" {
		fmt.Fprintln(stdout, "Use Open in the editor to choose a model, or load a bundled example.")
	} else {
		fmt.Fprintf(stdout, "Model: %s\n", modelPath)
	}
	fmt.Fprintln(stdout, "Press Ctrl+C to stop.")
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// runGenerate validates an ER model and emits deterministic MySQL SQL without
// starting a server. The optional timestamp is the only time-dependent output.
func runGenerate(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "-", "SQL output file, or - for stdout")
	noTimestamp := flags.Bool("no-timestamp", false, "omit the generated-at comment")
	err := flags.Parse(args)
	if err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" {
		return errors.New("usage: sciurus generate [--output FILE] [--no-timestamp] MODEL")
	}
	modelPath := flags.Arg(0)
	data, _, err := readFile(modelPath)
	if err != nil {
		return err
	}
	document, err := model.Decode(data)
	if err != nil {
		return err
	}
	generatedAt := time.Now().UTC()
	if *noTimestamp {
		generatedAt = time.Time{}
	}
	sql, err := model.Generate(document, generatedAt)
	if err != nil {
		return err
	}
	if *output == "-" {
		_, err = stdout.Write(sql)
		return err
	}
	outputPath, err := protectedOutputPath(*output, modelPath)
	if err != nil {
		return err
	}
	_, err = writeFile(outputPath, sql, nil)
	return err
}

// runValidate is a small, script-friendly integrity check for AI-authored models.
func runValidate(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	err := flags.Parse(args)
	if err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" {
		return errors.New("usage: sciurus validate MODEL")
	}
	data, _, err := readFile(flags.Arg(0))
	if err != nil {
		return err
	}
	document, err := model.Decode(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Valid: %s (%d tables, %d groups)\n", document.Name, len(document.Tables), len(document.Groups))
	return nil
}

// runImport selects the schema reader by filename extension and writes ER JSON.
func runImport(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("import", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "-", "ER model output file, or - for stdout")
	name := flags.String("name", "imported_schema", "name for the imported ER model")
	workbenchPath := flags.String("workbench", "", "optional MySQL Workbench .mwb file supplying diagram positions and groups")
	err := flags.Parse(args)
	if err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" {
		return errors.New("usage: sciurus import [--output FILE] [--name NAME] [--workbench FILE] INPUT.sql|INPUT.mwb")
	}
	inputPath := flags.Arg(0)
	extension := strings.ToLower(filepath.Ext(inputPath))
	if extension != ".sql" && extension != ".mwb" {
		return errors.New("import requires a .sql or .mwb input file")
	}
	if extension == ".mwb" && *workbenchPath != "" {
		return errors.New("--workbench supplies layout for .sql input; a .mwb input already includes its layout")
	}
	data, _, err := readFile(inputPath)
	if err != nil {
		return err
	}
	var document *model.Document
	if extension == ".mwb" {
		document, err = model.ImportWorkbench(data, *name)
	} else {
		document, err = model.ImportSQL(string(data), *name)
	}
	if err != nil {
		return err
	}
	if *workbenchPath != "" {
		layout, _, readErr := readFile(*workbenchPath)
		if readErr != nil {
			return readErr
		}
		err = model.ApplyWorkbenchLayout(document, layout)
		if err != nil {
			return err
		}
	}
	data, err = model.Encode(document)
	if err != nil {
		return err
	}
	if *output == "-" {
		_, err = stdout.Write(data)
		return err
	}
	outputPath, err := protectedOutputPath(*output, inputPath, *workbenchPath)
	if err != nil {
		return err
	}
	_, err = writeFile(outputPath, data, nil)
	return err
}
