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
	} else {
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
	if osName == "windows" {
		fmt.Println("  1. Install restic: winget install restic.restic")
	} else if osName == "darwin" {
		fmt.Println("  1. Install restic: brew install restic")
	} else {
		fmt.Println("  1. Install restic: sudo apt install restic (or your package manager)")
	}
	fmt.Printf("  2. Run: %s install --repo <your-repo> --password <pwd> --include <paths>\n", binaryName)
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

