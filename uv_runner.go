package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const uvVersion = "0.8.19" // Update as needed

func main() {
	// Determine platform and architecture
	platform := getPlatform()
	arch := getArch()
	target := fmt.Sprintf("%s-%s", arch, platform)

	fmt.Printf("Detected platform: %s\n", target)

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "uv-runner-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDir)

	// Download and extract uv
	uvPath, err := downloadUV(tempDir, target)
	if err != nil {
		panic(err)
	}

	// Determine which scripts to run:
	// - If the user supplies one or more paths/URLs as args, pass them through.
	// - Otherwise, use the built-in defaults.
	var scripts []string
	if len(os.Args) > 1 {
		// Use provided args as script paths/URLs
		scripts = os.Args[1:]
	} else {
		// Fall back to defaults
		scripts = []string{
			"https://raw.githubusercontent.com/tnldart/openapi-servers/refs/heads/main/servers/memory/oneshot.py",
			"https://raw.githubusercontent.com/tnldart/openapi-servers/refs/heads/main/servers/memory/main.py",
		}
	}

	// Build command: uv run <scripts...>
	fmt.Println("Running Python scripts...")
	args := append([]string{"run"}, scripts...)
	cmd := exec.Command(uvPath, args...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		fmt.Printf("Error running uv: %v\n", err)
		os.Exit(1)
	}
}

func getPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "apple-darwin"
	case "linux":
		return "unknown-linux-gnu"
	case "windows":
		return "pc-windows-msvc"
	default:
		panic(fmt.Sprintf("Unsupported platform: %s", runtime.GOOS))
	}
}

func getArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		panic(fmt.Sprintf("Unsupported architecture: %s", runtime.GOARCH))
	}
}

func downloadUV(tempDir, target string) (string, error) {
	// Determine file extension based on platform
	var fileExt string
	var tmpFilePattern string
	if runtime.GOOS == "windows" {
		fileExt = ".zip"
		tmpFilePattern = "uv-*.zip"
	} else {
		fileExt = ".tar.gz"
		tmpFilePattern = "uv-*.tar.gz"
	}

	url := fmt.Sprintf("https://github.com/astral-sh/uv/releases/download/%s/uv-%s%s", uvVersion, target, fileExt)
	checksumURL := url + ".sha256"

	fmt.Printf("Downloading uv from: %s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download uv: status %d", resp.StatusCode)
	}

	// Create a temporary file to store the downloaded archive
	tmpFile, err := os.CreateTemp(tempDir, tmpFilePattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download to temp file and calculate checksum
	hasher := sha256.New()
	multiWriter := io.MultiWriter(tmpFile, hasher)

	_, err = io.Copy(multiWriter, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save download: %w", err)
	}

	// Calculate the actual checksum
	actualChecksum := fmt.Sprintf("%x", hasher.Sum(nil))

	// Download and verify checksum
	fmt.Printf("Downloading checksum from: %s\n", checksumURL)
	checksumResp, err := http.Get(checksumURL)
	if err != nil {
		return "", fmt.Errorf("failed to download checksum: %w", err)
	}
	defer checksumResp.Body.Close()

	if checksumResp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download checksum: status %d", checksumResp.StatusCode)
	}

	checksumBytes, err := io.ReadAll(checksumResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read checksum: %w", err)
	}

	// Parse expected checksum (typically in format: "checksum filename")
	expectedChecksum := strings.Fields(string(checksumBytes))[0]

	// Verify checksum
	if actualChecksum != expectedChecksum {
		return "", fmt.Errorf("checksum verification failed: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	fmt.Println("Checksum verification successful")

	// Reset file position for reading
	_, err = tmpFile.Seek(0, 0)
	if err != nil {
		return "", fmt.Errorf("failed to reset file position: %w", err)
	}

	// Extract archive based on platform
	if runtime.GOOS == "windows" {
		return extractZip(tmpFile, tempDir)
	} else {
		return extractTarGz(tmpFile, tempDir)
	}
}

func extractTarGz(file *os.File, tempDir string) (string, error) {
	// Extract tar.gz from the verified file
	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	var uvPath string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		fmt.Printf("Found file in archive: %s\n", header.Name)

		// Look for uv executable (could be nested in a directory)
		if filepath.Base(header.Name) == "uv" || filepath.Base(header.Name) == "uv.exe" {
			uvPath = filepath.Join(tempDir, filepath.Base(header.Name))

			file, err := os.Create(uvPath)
			if err != nil {
				return "", err
			}

			_, err = io.Copy(file, tr)
			file.Close()
			if err != nil {
				return "", err
			}

			// Make executable
			err = os.Chmod(uvPath, 0755)
			if err != nil {
				return "", err
			}

			break
		}
	}

	if uvPath == "" {
		return "", fmt.Errorf("uv binary not found in archive")
	}

	return uvPath, nil
}

func extractZip(file *os.File, tempDir string) (string, error) {
	// Get file info for zip reader
	fileInfo, err := file.Stat()
	if err != nil {
		return "", err
	}

	// Create zip reader
	zr, err := zip.NewReader(file, fileInfo.Size())
	if err != nil {
		return "", err
	}

	var uvPath string
	for _, f := range zr.File {
		fmt.Printf("Found file in archive: %s\n", f.Name)

		// Look for uv executable (could be nested in a directory)
		if filepath.Base(f.Name) == "uv.exe" || filepath.Base(f.Name) == "uv" {
			uvPath = filepath.Join(tempDir, filepath.Base(f.Name))

			rc, err := f.Open()
			if err != nil {
				return "", err
			}

			outFile, err := os.Create(uvPath)
			if err != nil {
				rc.Close()
				return "", err
			}

			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				return "", err
			}

			// Make executable
			err = os.Chmod(uvPath, 0755)
			if err != nil {
				return "", err
			}

			break
		}
	}

	if uvPath == "" {
		return "", fmt.Errorf("uv binary not found in archive")
	}

	return uvPath, nil
}
