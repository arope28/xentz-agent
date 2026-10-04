package cli

import (
	"fmt"
	"log"

	"xentz-agent/internal/diagnostics"
)

func RunDiagnostics(args []string) error {
	fs := newFlagSet("diagnostics")
	outPath := fs.String("out", "", "Output diagnostics bundle path")
	if help, err := parseFlags(fs, args); err != nil {
		return err
	} else if help {
		return nil
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
