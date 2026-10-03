package cli

import (
	"flag"
	"fmt"
	"log"
	"net/url"

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

	// Print the ready-to-open URL first. Browsers cannot send the X-Local-Token
	// header, so the token has to travel in the query string; without printing
	// it here the user has no way to discover how to reach the UI.
	token, err := localui.ResolveToken()
	if err != nil {
		return fmt.Errorf("resolve local UI token: %w", err)
	}

	fmt.Printf("xentz-agent local UI starting.\n")
	fmt.Printf("Open this URL in a browser:\n\n    http://%s/?token=%s\n\n", *addr, url.QueryEscape(token))
	fmt.Printf("The page is only served on this machine. Press Ctrl+C to stop.\n")
	log.Printf("local UI listening on %s", *addr)

	if err := localui.Start(*addr, cfgFile); err != nil {
		return fmt.Errorf("local UI failed: %w", err)
	}
	return nil
}
