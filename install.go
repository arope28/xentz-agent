// install.go - Universal Go-based installer (works on all platforms)
// Build: go build -o install-xentz-agent install.go
// Usage: ./install-xentz-agent
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	baseURL = "https://github.com/arope28/xentz-agent/releases/latest/download"
)

func main() {
	fmt.Println("xentz-agent Installer")
	fmt.Println("======================")
	fmt.Println("")

	// Detect platform
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Handle special cases
	if osName == "darwin" && arch == "arm64" {
		// Check for universal binary first
		fmt.Println("Detected: macOS (Apple Silicon)")
		fmt.Println("Checking for universal binary...")
	} else if osName == "darwin" && arch == "amd64" {
		fmt.Println("Detected: macOS (Intel)")
		fmt.Println("Checking for universal binary...")
	} else {
		fmt.Printf("Detected: %s (%s)\n", osName, arch)
	}

	// Check for restic
	if !checkRestic() {
		fmt.Println("")
		fmt.Println("⚠ restic is not installed")
		fmt.Println("")
		fmt.Print("Would you like to install restic now? (y/N): ")
		var response string
		fmt.Scanln(&response)
		if response == "y" || response == "Y" {
			if !installRestic(osName) {
				fmt.Println("")
				fmt.Println("Please install restic manually before using xentz-agent")
			}
		} else {
			fmt.Println("")
			fmt.Println("Please install restic manually before using xentz-agent:")
			if osName == "windows" {
				fmt.Println("  winget install restic.restic")
			} else if osName == "darwin" {
				fmt.Println("  brew install restic")
			} else {
				fmt.Println("  sudo apt install restic (or your package manager)")
			}
		}
		fmt.Println("")
	} else {
		fmt.Println("✓ restic is already installed")
		cmd := exec.Command("restic", "version")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()
		fmt.Println("")
	}

	// Determine binary name
	var binaryFile string
	var binaryName string

	if osName == "windows" {
		binaryName = "xentz-agent.exe"
		// For Windows, prefer universal or arch-specific
		binaryFile = fmt.Sprintf("xentz-agent-windows-%s.exe", arch)
	} else {
		binaryName = "xentz-agent"
		// For macOS, try universal first
		if osName == "darwin" {
			universalFile := "xentz-agent-darwin-universal"
			if checkURLExists(fmt.Sprintf("%s/%s", baseURL, universalFile)) {
				binaryFile = universalFile
				fmt.Println("Using universal binary for macOS")
			} else {
				binaryFile = fmt.Sprintf("xentz-agent-darwin-%s", arch)
			}
		} else {
			// Linux
			if arch == "arm" {
				// Check GOARM for armv7
				binaryFile = "xentz-agent-linux-armv7"
			} else {
				binaryFile = fmt.Sprintf("xentz-agent-linux-%s", arch)
			}
		}
	}

	downloadURL := fmt.Sprintf("%s/%s", baseURL, binaryFile)
	fmt.Printf("Downloading from: %s\n", downloadURL)
	fmt.Println("")

	// Determine install directory
	var installDir string
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if osName == "windows" {
		installDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "xentz-agent")
	} else if osName == "darwin" {
		// macOS: use ~/bin (more common on macOS)
		installDir = filepath.Join(home, "bin")
	} else {
		// Linux: use ~/.local/bin (XDG standard)
		installDir = filepath.Join(home, ".local", "bin")
	}

	// Create install directory
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		fmt.Printf("Error creating install directory: %v\n", err)
		os.Exit(1)
	}

	binaryPath := filepath.Join(installDir, binaryName)

	// Download binary
	fmt.Println("Downloading xentz-agent...")
	if err := downloadFile(downloadURL, binaryPath); err != nil {
		fmt.Printf("Error downloading binary: %v\n", err)
		fmt.Printf("Please check that the release exists at: %s\n", downloadURL)
		os.Exit(1)
	}

	// Make executable (Unix-like systems)
	if osName != "windows" {
		if err := os.Chmod(binaryPath, 0o755); err != nil {
			fmt.Printf("Error making binary executable: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println("✓ Installation complete!")
	fmt.Println("")
	fmt.Printf("Binary installed to: %s\n", binaryPath)
	fmt.Println("")

	// Check if in PATH
	pathEnv := os.Getenv("PATH")
	if !strings.Contains(pathEnv, installDir) {
		fmt.Printf("Note: %s is not in your PATH\n", installDir)
		if osName == "windows" {
			fmt.Println("Add it to your PATH:")
			fmt.Printf("  [Environment]::SetEnvironmentVariable('Path', \"$env:Path;%s\", 'User')\n", installDir)
		} else if osName == "darwin" {
			fmt.Println("Add this to your ~/.zshrc or ~/.bash_profile:")
			fmt.Printf("  export PATH=\"%s:$PATH\"\n", installDir)
		} else {
			fmt.Println("Add this to your ~/.bashrc, ~/.zshrc, or ~/.profile:")
			fmt.Printf("  export PATH=\"%s:$PATH\"\n", installDir)
		}
		fmt.Println("")
		fmt.Println("Or run the agent directly:")
		fmt.Printf("  %s --help\n", binaryPath)
	} else {
		fmt.Println("You can now run:")
		fmt.Printf("  %s --help\n", binaryName)
	}

	fmt.Println("")
	fmt.Println("Next steps:")
	if !checkRestic() {
		fmt.Println("  1. Install restic if not already installed")
	}
	fmt.Printf("  2. Run: %s install --repo <your-repo> --password <pwd> --include <paths>\n", binaryName)
}

func checkRestic() bool {
	_, err := exec.LookPath("restic")
	return err == nil
}

func installRestic(osName string) bool {
	fmt.Println("")
	fmt.Println("Attempting to install restic...")

	var cmd *exec.Cmd

	switch osName {
	case "darwin":
		// Check for Homebrew
		if _, err := exec.LookPath("brew"); err == nil {
			fmt.Println("Installing restic via Homebrew...")
			cmd = exec.Command("brew", "install", "restic")
		} else {
			fmt.Println("Homebrew not found. Please install restic manually:")
			fmt.Println("  brew install restic")
			return false
		}
	case "windows":
		// Try winget
		if _, err := exec.LookPath("winget"); err == nil {
			fmt.Println("Installing restic via winget...")
			cmd = exec.Command("winget", "install", "--id", "restic.restic", "--accept-package-agreements", "--accept-source-agreements")
		} else if _, err := exec.LookPath("choco"); err == nil {
			fmt.Println("Installing restic via Chocolatey...")
			cmd = exec.Command("choco", "install", "restic", "-y")
		} else {
			fmt.Println("No supported package manager found. Please install restic manually:")
			fmt.Println("  winget install restic.restic")
			return false
		}
	default:
		// Linux - try different package managers
		if _, err := exec.LookPath("apt-get"); err == nil {
			fmt.Println("Installing restic via apt...")
			cmd = exec.Command("sh", "-c", "sudo apt-get update && sudo apt-get install -y restic")
		} else if _, err := exec.LookPath("yum"); err == nil {
			fmt.Println("Installing restic via yum...")
			cmd = exec.Command("sudo", "yum", "install", "-y", "restic")
		} else if _, err := exec.LookPath("dnf"); err == nil {
			fmt.Println("Installing restic via dnf...")
			cmd = exec.Command("sudo", "dnf", "install", "-y", "restic")
		} else if _, err := exec.LookPath("pacman"); err == nil {
			fmt.Println("Installing restic via pacman...")
			cmd = exec.Command("sudo", "pacman", "-S", "--noconfirm", "restic")
		} else {
			fmt.Println("No supported package manager found. Please install restic manually:")
			fmt.Println("  Visit: https://restic.net")
			return false
		}
	}

	if cmd != nil {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("✗ Failed to install restic: %v\n", err)
			return false
		}
		fmt.Println("✓ restic installed successfully")
		return true
	}

	return false
}

func downloadFile(url, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func checkURLExists(url string) bool {
	resp, err := http.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

