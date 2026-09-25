package mirasim

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ProcessManager manages the lifecycle of a child Mirasim server process.
type ProcessManager struct {
	cmd     *exec.Cmd
	port    int
	token   string
	wsURL   string
	mu      sync.Mutex
	stopped bool
}

// LocateServerScript looks for server.cjs in candidate directories.
// If not found, it attempts to extract it from any Mirasim*.exe installer in the folder.
func LocateServerScript(customPath string) (string, error) {
	if customPath != "" {
		if _, err := os.Stat(customPath); err == nil {
			abs, err := filepath.Abs(customPath)
			if err == nil {
				return abs, nil
			}
			return customPath, nil
		}
	}

	candidates := []string{
		"./server.cjs",
		"../server.cjs",
		"../../server.cjs",
		"./resources/server.cjs",
		"../resources/server.cjs",
		"../../resources/server.cjs",
	}

	home, err := os.UserHomeDir()
	if err == nil {
		if runtime.GOOS == "windows" {
			candidates = append(candidates,
				filepath.Join(home, "AppData", "Local", "Programs", "Mirasim", "resources", "server.cjs"),
			)
		} else if runtime.GOOS == "darwin" {
			candidates = append(candidates,
				"/Applications/Mirasim.app/Contents/Resources/server.cjs",
			)
		}
	}

	for _, cand := range candidates {
		if _, err := os.Stat(cand); err == nil {
			abs, err := filepath.Abs(cand)
			if err == nil {
				return abs, nil
			}
			return cand, nil
		}
	}

	// Try extracting from Mirasim*.exe in candidate folders if 7z is installed
	if extracted, err := TryExtractFromExe(); err == nil && extracted != "" {
		log.Printf("[AutoExtract] Successfully extracted server.cjs from installer to: %s", extracted)
		return extracted, nil
	}

	return "", fmt.Errorf("server.cjs not found; specify path with server_script in config or place server.cjs in workspace")
}

// TryExtractFromExe searches for a Mirasim*.exe in the workspace and extracts server.cjs.
func TryExtractFromExe() (string, error) {
	searchDirs := []string{".", "..", "../.."}
	var installer string

	for _, dir := range searchDirs {
		exes, _ := filepath.Glob(filepath.Join(dir, "Mirasim*.exe"))
		if len(exes) > 0 {
			installer = exes[0]
			break
		}
	}

	if installer == "" {
		return "", fmt.Errorf("no Mirasim*.exe installer found")
	}

	sevenZip := "7z"
	if _, err := exec.LookPath("7z"); err != nil {
		win7z := `C:\Program Files\7-Zip\7z.exe`
		if _, err := os.Stat(win7z); err == nil {
			sevenZip = win7z
		} else {
			return "", fmt.Errorf("7-Zip (7z) not found; cannot auto-extract from %s", installer)
		}
	}

	log.Printf("[AutoExtract] Found installer %s, extracting using %s...", installer, sevenZip)

	tmpDir, err := os.MkdirTemp("", "mirasim-extract-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	cmd1 := exec.Command(sevenZip, "e", installer, "$PLUGINSDIR/app-64.7z", "-o"+tmpDir, "-y")
	if out, err := cmd1.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to extract app-64.7z from %s: %s: %w", installer, string(out), err)
	}

	app7z := filepath.Join(tmpDir, "app-64.7z")
	if _, err := os.Stat(app7z); err != nil {
		return "", fmt.Errorf("app-64.7z not found after extraction")
	}

	outDir := filepath.Dir(installer)
	cmd2 := exec.Command(sevenZip, "e", app7z, "resources/server.cjs", "-o"+outDir, "-y")
	if out, err := cmd2.CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to extract resources/server.cjs from app-64.7z: %s: %w", string(out), err)
	}

	extractedPath := filepath.Join(outDir, "server.cjs")
	if _, err := os.Stat(extractedPath); err == nil {
		abs, _ := filepath.Abs(extractedPath)
		return abs, nil
	}

	return "", fmt.Errorf("server.cjs not found after extraction")
}

// StartMirasimServer spawns the Mirasim backend on the specified port.
func StartMirasimServer(serverScript string, port int, workdir string) (*ProcessManager, error) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return nil, fmt.Errorf("node.js not found in PATH: %w", err)
	}

	if workdir == "" {
		workdir, _ = os.Getwd()
	}

	args := []string{
		serverScript,
		"serve",
		"--port", fmt.Sprintf("%d", port),
		"--no-open",
		"--no-im",
		"--host", "127.0.0.1",
	}

	cmd := exec.Command(nodePath, args...)
	cmd.Dir = workdir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start mirasim server: %w", err)
	}

	pm := &ProcessManager{
		cmd:  cmd,
		port: port,
	}

	// Capture token from stdout
	tokenChan := make(chan string, 1)
	tokenRegex := regexp.MustCompile(`token=([a-zA-Z0-9_\-]+)`)

	go func() {
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			line = strings.TrimSpace(line)
			if line != "" {
				log.Printf("[Mirasim STDOUT] %s", line)
			}
			matches := tokenRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				select {
				case tokenChan <- matches[1]:
				default:
				}
			}
		}
	}()

	go func() {
		reader := bufio.NewReader(stderr)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					// Ignore
				}
				break
			}
			line = strings.TrimSpace(line)
			if line != "" {
				log.Printf("[Mirasim STDERR] %s", line)
			}
		}
	}()

	// Wait up to 15s for the server to be ready and emit token
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var token string
	select {
	case token = <-tokenChan:
	case <-ctx.Done():
		if tok, err := ReadTokenForPort(port); err == nil && tok != "" {
			token = tok
		} else {
			_ = pm.Stop()
			return nil, fmt.Errorf("timed out waiting for Mirasim server token on port %d", port)
		}
	}

	pm.token = token
	pm.wsURL = fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", port, token)

	log.Printf("[Mirasim] Server started on port %d with token %s", port, token)
	return pm, nil
}

// Stop terminates the spawned process.
func (pm *ProcessManager) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.stopped || pm.cmd == nil || pm.cmd.Process == nil {
		return nil
	}
	pm.stopped = true

	log.Printf("[Mirasim] Stopping background server process (pid %d)...", pm.cmd.Process.Pid)
	err := pm.cmd.Process.Kill()
	_ = pm.cmd.Wait()
	return err
}

// WsURL returns the WebSocket URL.
func (pm *ProcessManager) WsURL() string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.wsURL
}

// Port returns the listening port.
func (pm *ProcessManager) Port() int {
	return pm.port
}

// Token returns the authentication token.
func (pm *ProcessManager) Token() string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.token
}
