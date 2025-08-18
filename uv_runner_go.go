package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const uvVersion = "0.8.11" // Update as needed

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
	
	// Run the command
	fmt.Println("Running Python scripts...")
	cmd := exec.Command(uvPath, "run", 
		"https://raw.githubusercontent.com/tnldart/openapi-servers/refs/heads/main/servers/memory/oneshot.py",
		"https://raw.githubusercontent.com/tnldart/openapi-servers/refs/heads/main/servers/memory/main.py")
	
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
	url := fmt.Sprintf("https://github.com/astral-sh/uv/releases/download/%s/uv-%s.tar.gz", uvVersion, target)
	
	fmt.Printf("Downloading uv from: %s\n", url)
	
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download uv: status %d", resp.StatusCode)
	}
	
	// Extract tar.gz
	gzr, err := gzip.NewReader(resp.Body)
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
