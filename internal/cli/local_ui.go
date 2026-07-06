package cli

import (
	"flag"
	"fmt"
	"log"

	"xentz-agent/internal/config"
	"xentz-agent/internal/localui"
)

func RunLocalUI(args []string) error {
	fs := flag.NewFlagSet("local-ui", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:9800", "Bind address")
	configPath := fs.String("config", "", "Config path override")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	cfgFile, err := config.ResolvePath(*configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	log.Printf("local UI listening on %s (header X-Local-Token required)", *addr)
	if err := localui.Start(*addr, cfgFile); err != nil {
		return fmt.Errorf("local UI failed: %w", err)
	}
	return nil
}
