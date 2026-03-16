// Package main provides the trace2mermaid CLI tool.
package main

import (
	"fmt"
	"os"

	"github.com/mheap/trace2mermaid/internal/traceconv"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Output format constants.
const (
	formatMermaid = "mermaid"
	formatSVG     = "svg"
)

func main() {
	format := formatMermaid
	var file string

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--version", "-v":
			fmt.Printf("trace2mermaid %s (commit: %s, built: %s)\n", version, commit, date)
			return
		case "--help", "-h":
			printUsage()
			return
		case "--format", "-f":
			i++
			if i >= len(args) {
				fatal("--format requires a value (mermaid, svg)")
			}
			format = args[i]
		default:
			if file != "" {
				fmt.Fprintf(os.Stderr, "Error: unexpected argument %q\n\n", args[i]) //nolint:gosec // CLI stderr output, not web response
				printUsage()
				os.Exit(1)
			}
			file = args[i]
		}
	}

	if format != formatMermaid && format != formatSVG {
		fatal("invalid format %q (must be mermaid or svg)", format)
	}

	f, err := openInput(file)
	if err != nil {
		fatal("%s", err)
	}
	if f != os.Stdin {
		defer func() { _ = f.Close() }()
	}

	t, err := traceconv.Parse(f)
	if err != nil {
		fatal("%s", err)
	}

	switch format {
	case formatMermaid:
		err = traceconv.Render(os.Stdout, t)
	case formatSVG:
		err = traceconv.RenderSVG(os.Stdout, t)
	}
	if err != nil {
		fatal("%s", err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...) //nolint:gosec // CLI stderr output, not web response
	os.Exit(1)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: trace2mermaid [options] [file]

Convert OpenTelemetry trace JSONL to a Mermaid Gantt diagram.

Arguments:
  file        Path to OTLP JSON Lines trace file (reads stdin if omitted)

Options:
  -f, --format <format>  Output format: mermaid (default), svg
  -h, --help             Show this help message
  -v, --version          Show version information
`)
}

func openInput(path string) (*os.File, error) {
	if path != "" {
		f, err := os.Open(path) //nolint:gosec // input file path from CLI args is intentional
		if err != nil {
			return nil, fmt.Errorf("could not open file %q: %w", path, err)
		}
		return f, nil
	}

	// Check if stdin has data (is a pipe, not a terminal)
	info, err := os.Stdin.Stat()
	if err != nil {
		return nil, fmt.Errorf("could not stat stdin: %w", err)
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		printUsage()
		os.Exit(1)
	}

	return os.Stdin, nil
}
