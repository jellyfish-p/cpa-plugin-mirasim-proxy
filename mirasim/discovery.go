package mirasim

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// InstanceInfo represents an active Mirasim server.
type InstanceInfo struct {
	Port  int
	Token string
	WsURL string
}

// FindActiveInstances scans ~/.mirasim/run/ for local-*.token files
// and verifies if the corresponding port is reachable and serving Mirasim.
func FindActiveInstances() ([]InstanceInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	runDir := filepath.Join(home, ".mirasim", "run")
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return nil, err
	}

	var instances []InstanceInfo
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "local-") && strings.HasSuffix(name, ".token") {
			portStr := strings.TrimSuffix(strings.TrimPrefix(name, "local-"), ".token")
			port, err := strconv.Atoi(portStr)
			if err != nil {
				continue
			}

			tokenBytes, err := os.ReadFile(filepath.Join(runDir, name))
			if err != nil {
				continue
			}
			token := strings.TrimSpace(string(tokenBytes))
			if token == "" {
				continue
			}

			// Verify that the server is genuinely responding to Mirasim API
			if isMirasimResponding(port, 400*time.Millisecond) {
				wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws?token=%s", port, token)
				instances = append(instances, InstanceInfo{
					Port:  port,
					Token: token,
					WsURL: wsURL,
				})
			}
		}
	}

	return instances, nil
}

// ReadTokenForPort reads the token file for a specific port.
func ReadTokenForPort(port int) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	tokenPath := filepath.Join(home, ".mirasim", "run", fmt.Sprintf("local-%d.token", port))
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// isMirasimResponding checks if /api/state responds with a valid Mirasim payload.
func isMirasimResponding(port int, timeout time.Duration) bool {
	client := http.Client{
		Timeout: timeout,
	}

	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/state", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var state struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return false
	}

	return state.Name == "mirasim"
}
