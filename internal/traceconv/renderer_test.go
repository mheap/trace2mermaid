package traceconv_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mheap/trace2mermaid/internal/traceconv"
)

var update = flag.Bool("update", false, "update .golden files")

// TestGolden runs all .jsonl files in testdata/ through Parse + Render
// and compares the output to the corresponding .golden file.
// Run with -update to regenerate the golden files after intentional changes.
func TestGolden(t *testing.T) {
	examples, err := filepath.Glob("testdata/*.jsonl")
	if err != nil {
		t.Fatalf("failed to glob testdata: %v", err)
	}
	if len(examples) == 0 {
		t.Fatal("no .jsonl files found in testdata/")
	}

	for _, jsonlPath := range examples {
		name := strings.TrimSuffix(filepath.Base(jsonlPath), ".jsonl")
		goldenPath := strings.TrimSuffix(jsonlPath, ".jsonl") + ".golden"

		t.Run(name, func(t *testing.T) {
			// Read input
			inputFile, err := os.Open(jsonlPath) //nolint:gosec // test data path from glob
			if err != nil {
				t.Fatalf("failed to open %s: %v", jsonlPath, err)
			}
			defer func() { _ = inputFile.Close() }()

			// Parse
			tr, parseErr := traceconv.Parse(inputFile)
			if parseErr != nil {
				t.Fatalf("failed to parse %s: %v", jsonlPath, parseErr)
			}

			// Render
			var buf bytes.Buffer
			if renderErr := traceconv.Render(&buf, tr); renderErr != nil {
				t.Fatalf("failed to render: %v", renderErr)
			}
			actual := buf.String()

			// Update golden files if -update flag is set
			if *update {
				if writeErr := os.WriteFile(goldenPath, []byte(actual), 0o644); writeErr != nil { //nolint:gosec // golden test files need to be readable
					t.Fatalf("failed to update golden file %s: %v", goldenPath, writeErr)
				}
				return
			}

			// Read expected
			expectedBytes, readErr := os.ReadFile(goldenPath) //nolint:gosec // test data path from glob
			if readErr != nil {
				t.Fatalf("failed to read golden file %s (run with -update to generate): %v", goldenPath, readErr)
			}

			// Compare
			if actual != string(expectedBytes) {
				t.Errorf("output mismatch for %s (run with -update to accept)\n--- expected ---\n%s\n--- actual ---\n%s", name, expectedBytes, actual)
			}
		})
	}
}
