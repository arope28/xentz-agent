package cli

import (
	"fmt"
	"os"

	"xentz-agent/internal/backup"
	"xentz-agent/internal/config"
	"xentz-agent/internal/restore"
)

func RunRestore(args []string) error {
	rfs := newFlagSet("restore")
	restoreConfigPath := rfs.String("config", "", "Config path override")
	if help, err := parseFlags(rfs, args); err != nil {
		return err
	} else if help {
		return nil
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
