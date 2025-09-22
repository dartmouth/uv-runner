// Alternative: Create output area with canvas.Text for full color control
// Uncomment this section and comment out the RichText section above if you want more control

/*
	outputContainer := container.NewVScroll(container.NewVBox())
	outputContainer.SetMinSize(fyne.NewSize(400, 200))
	a.outputContainer = outputContainer
*/package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"image/color"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Smart theme that ensures proper contrast for both light and dark modes
type smartContrastTheme struct{}

// Ensure we implement the full theme interface
var _ fyne.Theme = (*smartContrastTheme)(nil)

func (t *smartContrastTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameForeground:
		if variant == theme.VariantDark {
			return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // White text for dark theme
		}
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255} // Black text for light theme
	case theme.ColorNameBackground:
		if variant == theme.VariantDark {
			return color.NRGBA{R: 32, G: 32, B: 32, A: 255} // Dark background
		}
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // Light background
	case theme.ColorNameInputBackground:
		if variant == theme.VariantDark {
			return color.NRGBA{R: 48, G: 48, B: 48, A: 255} // Darker input background
		}
		return color.NRGBA{R: 250, G: 250, B: 250, A: 255} // Light input background
	case theme.ColorNameButton:
		if variant == theme.VariantDark {
			return color.NRGBA{R: 64, G: 64, B: 64, A: 255} // Dark button
		}
		return color.NRGBA{R: 240, G: 240, B: 240, A: 255} // Light button
	case theme.ColorNameDisabled:
		// Make disabled text much more readable - this affects our read-only output
		if variant == theme.VariantDark {
			return color.NRGBA{R: 240, G: 240, B: 240, A: 255} // Almost white for dark theme
		}
		return color.NRGBA{R: 20, G: 20, B: 20, A: 255} // Almost black for light theme
	case theme.ColorNameDisabledButton:
		if variant == theme.VariantDark {
			return color.NRGBA{R: 40, G: 40, B: 40, A: 255} // Darker grey for disabled
		}
		return color.NRGBA{R: 200, G: 200, B: 200, A: 255} // Light grey for disabled
	case theme.ColorNamePrimary:
		if variant == theme.VariantDark {
			return color.NRGBA{R: 100, G: 180, B: 255, A: 255} // Lighter blue for dark theme
		}
		return color.NRGBA{R: 0, G: 120, B: 215, A: 255} // Standard blue for light theme
	default:
		// Delegate to default theme for all other colors
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (t *smartContrastTheme) Font(style fyne.TextStyle) fyne.Resource {
	// Delegate to default theme for fonts
	return theme.DefaultTheme().Font(style)
}

func (t *smartContrastTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	// Delegate to default theme for icons
	return theme.DefaultTheme().Icon(name)
}

func (t *smartContrastTheme) Size(name fyne.ThemeSizeName) float32 {
	// Delegate to default theme for sizes
	return theme.DefaultTheme().Size(name)
}

const uvVersion = "0.8.19"

type App struct {
	fyneApp      fyne.App
	window       fyne.Window
	scriptList   *widget.List
	outputText   *widget.RichText
	runButton    *widget.Button
	addButton    *widget.Button
	removeButton *widget.Button
	scripts      []string
	uvPath       string
	tempDir      string
	selectedIdx  int         // Track selected item manually
	outputBuffer string      // Keep track of output text
	outputMutex  sync.Mutex  // Protect output buffer
	runningCmds  []*exec.Cmd // Track running processes
	cmdsMutex    sync.Mutex  // Protect running commands slice
}

func main() {
	a := app.NewWithID("com.example.uvrunner")

	// Use smart contrast theme that adapts to system theme
	a.Settings().SetTheme(&smartContrastTheme{})

	w := a.NewWindow("UV Python Script Runner")
	w.Resize(fyne.NewSize(800, 600))

	app := &App{
		fyneApp:      a,
		window:       w,
		selectedIdx:  -1, // No selection initially
		outputBuffer: "",
		runningCmds:  make([]*exec.Cmd, 0),
		scripts: []string{
			"https://raw.githubusercontent.com/tnldart/openapi-servers/refs/heads/main/servers/memory/oneshot.py",
			"https://raw.githubusercontent.com/tnldart/openapi-servers/refs/heads/main/servers/memory/main.py",
		},
	}

	app.setupUI()
	app.initializeUV()

	// Set up cleanup on window close
	w.SetCloseIntercept(func() {
		app.cleanup()
		w.Close()
	})

	w.ShowAndRun()
}

func (a *App) setupUI() {
	// Create script list
	a.scriptList = widget.NewList(
		func() int { return len(a.scripts) },
		func() fyne.CanvasObject {
			return widget.NewLabel("Template")
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if id < len(a.scripts) {
				// Show just the filename or last part of URL
				script := a.scripts[id]
				if strings.Contains(script, "/") {
					parts := strings.Split(script, "/")
					script = parts[len(parts)-1]
				}
				label.SetText(script)
			}
		},
	)

	// Handle selection
	a.scriptList.OnSelected = func(id widget.ListItemID) {
		a.selectedIdx = id
	}
	a.scriptList.OnUnselected = func(id widget.ListItemID) {
		a.selectedIdx = -1
	}

	// Create buttons
	a.addButton = widget.NewButton("Add Script", a.addScript)
	a.removeButton = widget.NewButton("Remove Selected", a.removeScript)
	a.runButton = widget.NewButton("Run Scripts", a.runScripts)
	a.runButton.Importance = widget.HighImportance

	// Theme toggle buttons
	lightThemeBtn := widget.NewButton("Light Theme", func() {
		a.fyneApp.Settings().SetTheme(theme.LightTheme())
	})
	darkThemeBtn := widget.NewButton("Dark Theme", func() {
		a.fyneApp.Settings().SetTheme(theme.DarkTheme())
	})
	autoThemeBtn := widget.NewButton("Auto Theme", func() {
		a.fyneApp.Settings().SetTheme(&smartContrastTheme{})
	})

	// Create output area with RichText for better theme support
	a.outputText = widget.NewRichText()
	a.outputText.Wrapping = fyne.TextWrapWord

	outputScroll := container.NewScroll(a.outputText)
	outputScroll.SetMinSize(fyne.NewSize(400, 200))

	// Layout
	scriptControls := container.NewHBox(a.addButton, a.removeButton)
	themeControls := container.NewHBox(lightThemeBtn, darkThemeBtn, autoThemeBtn)

	scriptSection := container.NewBorder(
		widget.NewLabel("Python Scripts:"),
		container.NewVBox(scriptControls, themeControls),
		nil, nil,
		a.scriptList,
	)

	outputSection := container.NewBorder(
		widget.NewLabel("Output:"),
		nil, nil, nil,
		outputScroll,
	)

	mainContent := container.NewVSplit(
		scriptSection,
		outputSection,
	)
	mainContent.SetOffset(0.4) // 40% for scripts, 60% for output

	content := container.NewBorder(
		nil,
		a.runButton,
		nil, nil,
		mainContent,
	)

	a.window.SetContent(content)
}

func (a *App) addScript() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder("Enter script URL or local path...")
	entry.Resize(fyne.NewSize(400, entry.MinSize().Height))

	dialog.ShowForm("Add Script", "Add", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Script URL/Path", entry),
	}, func(ok bool) {
		if ok && entry.Text != "" {
			a.scripts = append(a.scripts, entry.Text)
			a.scriptList.Refresh()
		}
	}, a.window)
}

func (a *App) removeScript() {
	if a.selectedIdx < 0 || a.selectedIdx >= len(a.scripts) {
		dialog.ShowInformation("No Selection", "Please select a script to remove.", a.window)
		return
	}

	// Remove selected item
	a.scripts = append(a.scripts[:a.selectedIdx], a.scripts[a.selectedIdx+1:]...)
	a.selectedIdx = -1
	a.scriptList.Refresh()
}

func (a *App) cleanup() {
	a.appendOutput("Cleaning up processes...\n")

	a.cmdsMutex.Lock()
	defer a.cmdsMutex.Unlock()

	for _, cmd := range a.runningCmds {
		if cmd.Process != nil {
			// On macOS/Unix, kill the process group to ensure child processes are terminated
			if runtime.GOOS != "windows" {
				// Kill the process group (negative PID kills the process group)
				if err := exec.Command("kill", "-TERM", fmt.Sprintf("-%d", cmd.Process.Pid)).Run(); err == nil {
					// Wait a bit for graceful termination
					time.Sleep(100 * time.Millisecond)
				}
				// Force kill if still running
				exec.Command("kill", "-KILL", fmt.Sprintf("-%d", cmd.Process.Pid)).Run()
			} else {
				// On Windows, use taskkill to terminate the process tree
				exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
			}

			// Also try direct process kill as fallback
			cmd.Process.Kill()
		}
	}

	// Clean up temp directory if it exists
	if a.tempDir != "" {
		os.RemoveAll(a.tempDir)
	}

	a.runningCmds = make([]*exec.Cmd, 0)
	a.appendOutput("Cleanup completed.\n")
}

func (a *App) addRunningCmd(cmd *exec.Cmd) {
	a.cmdsMutex.Lock()
	defer a.cmdsMutex.Unlock()
	a.runningCmds = append(a.runningCmds, cmd)
}

func (a *App) removeRunningCmd(cmd *exec.Cmd) {
	a.cmdsMutex.Lock()
	defer a.cmdsMutex.Unlock()
	for i, c := range a.runningCmds {
		if c == cmd {
			a.runningCmds = append(a.runningCmds[:i], a.runningCmds[i+1:]...)
			break
		}
	}
}

func (a *App) setupProcessGroup(cmd *exec.Cmd) {
	// On Unix systems, create a new process group so we can kill
	// the entire group (including child processes) when needed
	if runtime.GOOS != "windows" {
		a.setupUnixProcessGroup(cmd)
	}
}

func (a *App) initializeUV() {
	a.appendOutput("Initializing UV Python package manager...\n")

	go func() {
		// Create temp directory
		tempDir, err := os.MkdirTemp("", "uv-runner-*")
		if err != nil {
			a.appendOutput(fmt.Sprintf("Error creating temp directory: %v\n", err))
			return
		}
		a.tempDir = tempDir

		// Download and extract uv
		uvPath, err := a.downloadUV(tempDir)
		if err != nil {
			a.appendOutput(fmt.Sprintf("Error downloading UV: %v\n", err))
			return
		}

		a.uvPath = uvPath
		a.appendOutput("UV initialized successfully!\n")

		// Enable run button using fyne.Do
		fyne.Do(func() {
			a.runButton.Enable()
		})
	}()
}

func (a *App) runScripts() {
	if a.uvPath == "" {
		dialog.ShowError(fmt.Errorf("UV not initialized yet"), a.window)
		return
	}

	if len(a.scripts) == 0 {
		dialog.ShowInformation("No Scripts", "Please add some scripts to run.", a.window)
		return
	}

	a.runButton.Disable()

	// Clean up any existing running processes before starting new ones
	a.cmdsMutex.Lock()
	if len(a.runningCmds) > 0 {
		a.appendOutput("Stopping existing processes...\n")
		for _, cmd := range a.runningCmds {
			if cmd.Process != nil {
				if runtime.GOOS != "windows" {
					exec.Command("kill", "-TERM", fmt.Sprintf("-%d", cmd.Process.Pid)).Run()
				} else {
					exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
				}
				cmd.Process.Kill()
			}
		}
		a.runningCmds = make([]*exec.Cmd, 0)
	}
	a.cmdsMutex.Unlock()

	a.outputBuffer = "" // Clear output
	fyne.Do(func() {
		a.outputText.ParseMarkdown("")
	})
	a.appendOutput("Starting script execution...\n")

	go func() {
		defer func() {
			fyne.Do(func() {
				a.runButton.Enable()
			})
		}()

		// Build command: uv run <scripts...>
		args := append([]string{"run"}, a.scripts...)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, a.uvPath, args...)

		// Set up process group for proper cleanup on Unix systems
		if runtime.GOOS != "windows" {
			a.setupProcessGroup(cmd)
		}

		// Create pipes for stdout and stderr
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			a.appendOutput(fmt.Sprintf("Error creating stdout pipe: %v\n", err))
			return
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			a.appendOutput(fmt.Sprintf("Error creating stderr pipe: %v\n", err))
			return
		}

		// Start the command
		if err := cmd.Start(); err != nil {
			a.appendOutput(fmt.Sprintf("Error starting command: %v\n", err))
			return
		}

		// Track the running command
		a.addRunningCmd(cmd)

		// Read output in goroutines
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			a.readOutput(stdout, "STDOUT")
		}()

		go func() {
			defer wg.Done()
			a.readOutput(stderr, "STDERR")
		}()

		// Wait for output readers to finish
		wg.Wait()

		// Wait for completion
		if err := cmd.Wait(); err != nil {
			a.appendOutput(fmt.Sprintf("Command finished with error: %v\n", err))
		} else {
			a.appendOutput("Scripts completed successfully!\n")
		}

		// Remove from tracking when completed
		a.removeRunningCmd(cmd)
	}()
}

func (a *App) readOutput(reader io.Reader, prefix string) {
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			output := string(buf[:n])
			a.appendOutput(output)
		}
		if err != nil {
			if err != io.EOF {
				a.appendOutput(fmt.Sprintf("%s read error: %v\n", prefix, err))
			}
			break
		}
	}
}

func (a *App) appendOutput(text string) {
	// Thread-safe buffer update
	a.outputMutex.Lock()
	a.outputBuffer += text
	newText := a.outputBuffer
	a.outputMutex.Unlock()

	// Use Fyne's proper threading API
	fyne.Do(func() {
		a.outputText.ParseMarkdown("```\n" + newText + "\n```")
	})
}

func (a *App) downloadUV(tempDir string) (string, error) {
	platform := getPlatform()
	arch := getArch()
	target := fmt.Sprintf("%s-%s", arch, platform)

	a.appendOutput(fmt.Sprintf("Detected platform: %s\n", target))

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

	a.appendOutput(fmt.Sprintf("Downloading UV from: %s\n", url))

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
	a.appendOutput("Verifying checksum...\n")
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

	a.appendOutput("Checksum verification successful\n")
	a.appendOutput("Extracting UV binary...\n")

	// Reset file position for reading
	_, err = tmpFile.Seek(0, 0)
	if err != nil {
		return "", fmt.Errorf("failed to reset file position: %w", err)
	}

	// Extract archive based on platform
	if runtime.GOOS == "windows" {
		return a.extractZip(tmpFile, tempDir)
	} else {
		return a.extractTarGz(tmpFile, tempDir)
	}
}

func (a *App) extractTarGz(file *os.File, tempDir string) (string, error) {
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

func (a *App) extractZip(file *os.File, tempDir string) (string, error) {
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
