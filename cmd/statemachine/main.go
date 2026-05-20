// Command statemachine generates Go source from a .sm transition table.
//
// Usage:
//
//	statemachine [-out FILE] INPUT.sm
//
// With no -out, the output path is INPUT_gen.go in the same directory as INPUT.
// Designed for use with go:generate:
//
//	//go:generate statemachine traffic.sm
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/matthewjberger/statemachine"
)

func main() {
	out := flag.String("out", "", "output .go file (default: <input>_gen.go in input directory)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: statemachine [-out FILE] INPUT.sm\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	input := flag.Arg(0)

	if err := run(input, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(input, out string) error {
	source, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("reading %s: %w", input, err)
	}

	ast, err := statemachine.Parse(input, source)
	if err != nil {
		return err
	}
	if err := statemachine.Validate(input, ast); err != nil {
		return err
	}

	generated, err := statemachine.Generate(ast)
	if err != nil {
		return err
	}

	if out == "" {
		base := filepath.Base(input)
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		out = filepath.Join(filepath.Dir(input), stem+"_gen.go")
	}

	if err := os.WriteFile(out, generated, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}
	return nil
}
