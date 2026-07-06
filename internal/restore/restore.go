package restore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const Usage = "usage: xentz-agent restore guided | snapshots | find <path> | ls <snapshot_id> [path] | stats [snapshot_id] | check | <snapshot_id> --target <dir> [--path <path>] | dump <snapshot_id> <path> [--output <file>]"

type Snapshot struct {
	ID      string `json:"id"`
	ShortID string `json:"short_id"`
	Time    string `json:"time"`
}

func Run(args []string, env []string) error {
	if _, err := exec.LookPath("restic"); err != nil {
		return fmt.Errorf("restic not found in PATH (install restic first)")
	}
	if len(args) == 0 {
		return fmt.Errorf(Usage)
	}

	switch args[0] {
	case "guided":
		if err := runGuided(env); err != nil {
			return fmt.Errorf("guided restore failed: %w", err)
		}
		return nil
	case "snapshots":
		return runRestic(env, "snapshots")
	case "find":
		if len(args) < 2 {
			return fmt.Errorf("usage: xentz-agent restore find <path>")
		}
		return runRestic(env, append([]string{"find", args[1]}, args[2:]...)...)
	case "ls":
		if len(args) < 2 {
			return fmt.Errorf("usage: xentz-agent restore ls <snapshot_id> [path]")
		}
		resticArgs := []string{"ls", args[1]}
		if len(args) > 2 {
			resticArgs = append(resticArgs, args[2:]...)
		}
		return runRestic(env, resticArgs...)
	case "stats":
		resticArgs := []string{"stats"}
		if len(args) > 1 {
			resticArgs = append(resticArgs, args[1:]...)
		}
		return runRestic(env, resticArgs...)
	case "check":
		return runRestic(env, "check")
	case "dump":
		return runDump(args[1:], env)
	default:
		return runRestore(args, env)
	}
}

func runRestic(env []string, args ...string) error {
	cmd := exec.CommandContext(context.Background(), "restic", args...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restic %s: %w", args[0], err)
	}
	return nil
}

// parseDumpArgs accepts --output before or after the positional arguments;
// the stdlib flag package stops at the first positional, which silently
// dropped a trailing --output and sent the file to stdout instead.
func parseDumpArgs(args []string) (snapshotID, pathInSnapshot, outPath string, err error) {
	const usage = "usage: xentz-agent restore dump <snapshot_id> <path> [--output <file>]"
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--output" || arg == "-output":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--output requires a file path\n%s", usage)
			}
			i++
			outPath = args[i]
		case strings.HasPrefix(arg, "--output="):
			outPath = strings.TrimPrefix(arg, "--output=")
		case strings.HasPrefix(arg, "-output="):
			outPath = strings.TrimPrefix(arg, "-output=")
		case strings.HasPrefix(arg, "-") && arg != "-":
			return "", "", "", fmt.Errorf("unknown flag %q\n%s", arg, usage)
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return "", "", "", fmt.Errorf("%s", usage)
	}
	return positional[0], positional[1], outPath, nil
}

func runDump(args []string, env []string) error {
	snapshotID, pathInSnapshot, outPath, err := parseDumpArgs(args)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(context.Background(), "restic", "dump", snapshotID, pathInSnapshot)
	cmd.Env = env
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		cmd.Stdout = f
	} else {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restic dump: %w", err)
	}
	return nil
}

func runRestore(args []string, env []string) error {
	snapshotID := args[0]
	rfs := flag.NewFlagSet("restore", flag.ExitOnError)
	target := rfs.String("target", "", "Directory to restore into")
	pathInSnapshot := rfs.String("path", "", "Restore only this path (optional)")
	if err := rfs.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *target == "" {
		return fmt.Errorf("usage: xentz-agent restore <snapshot_id> --target <dir> [--path <path>]")
	}
	resticArgs := []string{"restore", snapshotID, "--target", *target}
	if *pathInSnapshot != "" {
		resticArgs = append(resticArgs, "--include", *pathInSnapshot)
	}
	cmd := exec.CommandContext(context.Background(), "restic", resticArgs...)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restic restore: %w", err)
	}
	return nil
}

func promptLine(r *bufio.Reader, label string) (string, error) {
	fmt.Print(label)
	s, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(s), nil
}

func promptYesNo(r *bufio.Reader, label string, def bool) (bool, error) {
	suffix := " [y/N]: "
	if def {
		suffix = " [Y/n]: "
	}
	raw, err := promptLine(r, label+suffix)
	if err != nil {
		return false, err
	}
	if raw == "" {
		return def, nil
	}
	v := strings.ToLower(strings.TrimSpace(raw))
	return v == "y" || v == "yes", nil
}

func tailText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

func dirHasEntries(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func getSnapshots(env []string) ([]Snapshot, error) {
	cmd := exec.CommandContext(context.Background(), "restic", "snapshots", "--json")
	cmd.Env = env
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list snapshots: %w (%s)", err, tailText(stderr.String(), 1024))
	}
	var snaps []Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snaps); err != nil {
		return nil, fmt.Errorf("parse snapshots: %w", err)
	}
	return snaps, nil
}

func runGuided(env []string) error {
	r := bufio.NewReader(os.Stdin)
	fmt.Println("Guided restore")
	fmt.Println("1) one file   2) one folder   3) full snapshot")
	kind, err := promptLine(r, "Choose restore type [1-3]: ")
	if err != nil {
		return err
	}
	if kind != "1" && kind != "2" && kind != "3" {
		return fmt.Errorf("invalid restore type")
	}

	snapChoice, err := promptLine(r, "Use latest snapshot? [Y/n]: ")
	if err != nil {
		return err
	}
	snapshotID := "latest"
	if strings.EqualFold(snapChoice, "n") || strings.EqualFold(snapChoice, "no") {
		snaps, err := getSnapshots(env)
		if err != nil {
			return err
		}
		if len(snaps) == 0 {
			return fmt.Errorf("no snapshots found")
		}
		limit := len(snaps)
		if limit > 10 {
			limit = 10
		}
		fmt.Println("Recent snapshots:")
		for i := 0; i < limit; i++ {
			id := snaps[i].ID
			if snaps[i].ShortID != "" {
				id = snaps[i].ShortID
			}
			fmt.Printf("  %d) %s  %s\n", i+1, id, snaps[i].Time)
		}
		sel, err := promptLine(r, "Choose number (or paste snapshot ID): ")
		if err != nil {
			return err
		}
		if sel == "" {
			return fmt.Errorf("snapshot selection required")
		}
		if n, parseErr := strconv.Atoi(strings.TrimSpace(sel)); parseErr == nil && n >= 1 && n <= limit {
			snapshotID = snaps[n-1].ID
		} else {
			snapshotID = sel
		}
	}

	home, _ := os.UserHomeDir()
	defaultTarget := filepath.Join(home, "Desktop", "xentz-restore-"+time.Now().Format("20060102-150405"))
	target := defaultTarget
	pathInSnapshot := ""

	switch kind {
	case "1":
		pathInSnapshot, err = promptLine(r, "Enter full original file path: ")
		if err != nil {
			return err
		}
		if pathInSnapshot == "" {
			return fmt.Errorf("file path required")
		}
		customTarget, err := promptLine(r, "Output file path (blank for Desktop default): ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(customTarget) != "" {
			target = customTarget
		} else {
			target = filepath.Join(defaultTarget, filepath.Base(pathInSnapshot))
		}
	case "2":
		pathInSnapshot, err = promptLine(r, "Enter full original folder path: ")
		if err != nil {
			return err
		}
		if pathInSnapshot == "" {
			return fmt.Errorf("folder path required")
		}
		customTarget, err := promptLine(r, "Target directory (blank for Desktop default): ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(customTarget) != "" {
			target = customTarget
		}
	case "3":
		customTarget, err := promptLine(r, "Target directory (blank for Desktop default): ")
		if err != nil {
			return err
		}
		if strings.TrimSpace(customTarget) != "" {
			target = customTarget
		}
	}

	if target == "/" || target == "/System" || target == "/Library" || target == "/usr" {
		return fmt.Errorf("refusing dangerous target directory: %s", target)
	}

	fmt.Println("\nSummary:")
	fmt.Printf("  restore type: %s\n", map[string]string{"1": "one file", "2": "one folder", "3": "full snapshot"}[kind])
	fmt.Printf("  snapshot:     %s\n", snapshotID)
	if pathInSnapshot != "" {
		fmt.Printf("  source path:  %s\n", pathInSnapshot)
	}
	fmt.Printf("  target:       %s\n", target)
	ok, err := promptYesNo(r, "Run restore now?", true)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	switch kind {
	case "1":
		if _, err := os.Stat(target); err == nil {
			overwrite, promptErr := promptYesNo(r, "Output file already exists. Overwrite?", false)
			if promptErr != nil {
				return promptErr
			}
			if !overwrite {
				return fmt.Errorf("restore cancelled (target file exists)")
			}
		}
		_ = os.MkdirAll(filepath.Dir(target), 0o700)
		cmd := exec.CommandContext(context.Background(), "restic", "dump", snapshotID, pathInSnapshot)
		cmd.Env = env
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("open output file: %w", err)
		}
		defer f.Close()
		cmd.Stdout = f
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("restore failed: %w (%s)", err, tailText(stderr.String(), 2048))
		}
	case "2", "3":
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			hasEntries, dirErr := dirHasEntries(target)
			if dirErr == nil && hasEntries {
				overwrite, promptErr := promptYesNo(r, "Target directory is not empty. Continue anyway?", false)
				if promptErr != nil {
					return promptErr
				}
				if !overwrite {
					return fmt.Errorf("restore cancelled (target directory not empty)")
				}
			}
		}
		_ = os.MkdirAll(target, 0o700)
		args := []string{"restore", snapshotID, "--target", target}
		if kind == "2" {
			args = append(args, "--include", pathInSnapshot)
		}
		cmd := exec.CommandContext(context.Background(), "restic", args...)
		cmd.Env = env
		cmd.Stdout = os.Stdout
		var stderr bytes.Buffer
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("restore failed: %w (%s)", err, tailText(stderr.String(), 2048))
		}
	}

	fmt.Printf("\nRestore complete. Files written under: %s\n", target)
	if runtime.GOOS == "darwin" {
		fmt.Printf("Tip: open \"%s\"\n", target)
	}
	return nil
}
