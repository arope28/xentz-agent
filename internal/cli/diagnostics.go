package cli

import (
	"flag"
	"fmt"
	"log"

	"xentz-agent/internal/diagnostics"
)

func RunDiagnostics(args []string) error {
	fs := flag.NewFlagSet("diagnostics", flag.ExitOnError)
	outPath := fs.String("out", "", "Output diagnostics bundle path")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *outPath == "" {
		return fmt.Errorf("--out is required")
	}
	if err := diagnostics.CreateBundle(*outPath); err != nil {
		return fmt.Errorf("diagnostics failed: %w", err)
	}
	log.Printf("diagnostics bundle created: %s", *outPath)
	return nil
}
