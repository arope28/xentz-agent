package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"xentz-agent/internal/config"
)

type configMultiFlag []string

func (m *configMultiFlag) String() string { return fmt.Sprint([]string(*m)) }
func (m *configMultiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func RunConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	configPath := fs.String("config", "", "Config path override")
	var addIncludes configMultiFlag
	var removeIncludes configMultiFlag
	var addExcludes configMultiFlag
	var removeExcludes configMultiFlag
	listIncludes := fs.Bool("list-includes", false, "List current include paths")
	listExcludes := fs.Bool("list-excludes", false, "List current exclude paths")
	listAll := fs.Bool("list-all", false, "List both include and exclude paths")
	fs.Var(&addIncludes, "add-include", "Add include path (repeatable)")
	fs.Var(&removeIncludes, "remove-include", "Remove include path (repeatable)")
	fs.Var(&addExcludes, "add-exclude", "Add exclude pattern (repeatable)")
	fs.Var(&removeExcludes, "remove-exclude", "Remove exclude pattern (repeatable)")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfgFile, err := config.ResolvePath(*configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	localCfg, apiKey, err := LoadEnrollment(cfgFile, "")
	if err != nil {
		return fmt.Errorf("load enrollment: %w", err)
	}
	localCfg.DeviceAPIKey = apiKey
	if err := ModeMismatchError("config", localCfg); err != nil {
		return err
	}
	if localCfg.ServerURL == "" && localCfg.Restic.Repository == "" {
		return fmt.Errorf("missing config and no enrollment identity available (run `xentz-agent install` or `xentz-agent recover`)")
	}

	if *listAll || *listIncludes || *listExcludes {
		return listConfig(localCfg, *listAll, *listIncludes, *listExcludes)
	}

	if len(addIncludes) == 0 && len(removeIncludes) == 0 && len(addExcludes) == 0 && len(removeExcludes) == 0 {
		return fmt.Errorf("no operations specified. Use --add-include, --remove-include, --add-exclude, --remove-exclude, or --list-all")
	}

	isEnrolled := localCfg.DeviceAPIKey != "" && localCfg.ServerURL != ""

	currentCfg := localCfg
	if isEnrolled {
		fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
		if fetchErr != nil {
			return fmt.Errorf("failed to fetch config from server: %w", fetchErr)
		}
		currentCfg = fetchedCfg
	}

	newIncludes := applyIncludeChanges(currentCfg.Include, addIncludes, removeIncludes)
	newExcludes := applyExcludeChanges(currentCfg.Exclude, addExcludes, removeExcludes)

	if len(newIncludes) == 0 {
		return fmt.Errorf("error: at least one include path is required")
	}

	if isEnrolled {
		log.Println("Updating configuration on server...")
		updatedCfg, err := config.UpdateConfigOnServer(localCfg.ServerURL, localCfg.DeviceAPIKey, newIncludes, newExcludes)
		if err != nil {
			return fmt.Errorf("failed to update config on server: %w", err)
		}
		log.Println("✓ Configuration updated on server")
		log.Printf("  Include paths: %d", len(updatedCfg.Include))
		log.Printf("  Exclude patterns: %d", len(updatedCfg.Exclude))
		return nil
	}

	currentCfg.Include = newIncludes
	currentCfg.Exclude = newExcludes
	if err := config.Write(cfgFile, currentCfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	log.Println("✓ Configuration updated locally")
	log.Printf("  Include paths: %d", len(newIncludes))
	log.Printf("  Exclude patterns: %d", len(newExcludes))
	return nil
}

func listConfig(localCfg config.Config, listAll, listIncludes, listExcludes bool) error {
	cfg := localCfg
	if localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
		fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
		if fetchErr != nil {
			log.Printf("warning: failed to fetch config from server: %v", fetchErr)
			log.Println("Showing local config instead...")
		} else {
			cfg = fetchedCfg
		}
	}

	if listAll || listIncludes {
		fmt.Println("Include paths:")
		if len(cfg.Include) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, p := range cfg.Include {
				fmt.Printf("  %s\n", p)
			}
		}
	}

	if listAll || listExcludes {
		if listAll {
			fmt.Println("")
		}
		fmt.Println("Exclude patterns:")
		if len(cfg.Exclude) == 0 {
			fmt.Println("  (none)")
		} else {
			for _, p := range cfg.Exclude {
				fmt.Printf("  %s\n", p)
			}
		}
	}
	return nil
}

func applyIncludeChanges(current []string, add, remove []string) []string {
	next := append([]string(nil), current...)

	for _, path := range add {
		normalized := normalizeConfigPath(path)
		if contains(next, normalized) {
			log.Printf("warning: include path already exists: %s", normalized)
			continue
		}
		if !pathExists(normalized) {
			log.Printf("warning: include path does not exist: %s (will be added anyway)", normalized)
		}
		next = append(next, normalized)
	}

	for _, path := range remove {
		normalized := normalizeConfigPath(path)
		if contains(next, normalized) {
			next = removeFromSlice(next, normalized)
			log.Printf("removed include path: %s", normalized)
		} else {
			log.Printf("warning: include path not found: %s", normalized)
		}
	}

	return removeDuplicates(next)
}

func applyExcludeChanges(current []string, add, remove []string) []string {
	next := append([]string(nil), current...)

	for _, pattern := range add {
		if contains(next, pattern) {
			log.Printf("warning: exclude pattern already exists: %s", pattern)
			continue
		}
		next = append(next, pattern)
	}

	for _, pattern := range remove {
		if contains(next, pattern) {
			next = removeFromSlice(next, pattern)
			log.Printf("removed exclude pattern: %s", pattern)
		} else {
			log.Printf("warning: exclude pattern not found: %s", pattern)
		}
	}

	return removeDuplicates(next)
}

func normalizeConfigPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[2:])
		}
	} else if p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			p = home
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func removeDuplicates(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}
	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func removeFromSlice(slice []string, item string) []string {
	result := []string{}
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
