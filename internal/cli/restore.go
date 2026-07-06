package cli

import (
	"flag"
	"fmt"
	"os"

	"xentz-agent/internal/backup"
	"xentz-agent/internal/config"
	"xentz-agent/internal/restore"
)

func RunRestore(args []string) error {
	rfs := flag.NewFlagSet("restore", flag.ExitOnError)
	restoreConfigPath := rfs.String("config", "", "Config path override")
	if err := rfs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	cfgFile, err := config.ResolvePath(*restoreConfigPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	restoreCfg, resticPW, err := loadResticConfigAndPassword(cfgFile)
	if err != nil {
		return err
	}
	env := append(os.Environ(), backup.ResticEnv(restoreCfg, resticPW)...)
	return restore.Run(rfs.Args(), env)
}
