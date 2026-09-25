package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

//go:embed models/mirasim_models.json
var embeddedModelsJSON []byte

type catalogFilePayload struct {
	Models []catalogModelItem `json:"models"`
}

type catalogModelItem struct {
	Slug             string `json:"slug"`
	ID               string `json:"id"`
	DisplayName      string `json:"display_name"`
	Label            string `json:"label"`
	OwnedBy          string `json:"owned_by"`
	Description      string `json:"description"`
	ContextWindow    int    `json:"context_window"`
	MaxContextWindow int    `json:"max_context_window"`
	SupportsThinking bool   `json:"supports_thinking"`
	Type             string `json:"type"`
}

type mirasimCatalogStore struct {
	mu          sync.RWMutex
	models      []pluginapi.ModelInfo
	bySlug      map[string]pluginapi.ModelInfo
	revision    uint64
	lastUpdated time.Time
}

var catalogStore = &mirasimCatalogStore{
	bySlug: make(map[string]pluginapi.ModelInfo),
}

var updaterOnce sync.Once

func init() {
	if err := loadCatalogFromBytes(embeddedModelsJSON, "embedded"); err != nil {
		fmt.Printf("mirasim: warning: failed to parse embedded mirasim_models.json: %v\n", err)
	}
}

// StartModelCatalogUpdater starts a background updater similar to Codex's periodic model refresh.
func StartModelCatalogUpdater(ctx context.Context) {
	updaterOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					cfg := loadedConfig()
					tok := resolveActiveToken(cfg)
					relayURL := resolveRelayBaseURL(cfg)
					_, _ = RefreshModelsFromUpstream(ctx, tok, relayURL)
				}
			}
		}()
	})
}

// GetModelsCatalog returns a snapshot of all currently available models.
func GetModelsCatalog() []pluginapi.ModelInfo {
	catalogStore.mu.RLock()
	defer catalogStore.mu.RUnlock()
	copied := make([]pluginapi.ModelInfo, len(catalogStore.models))
	copy(copied, catalogStore.models)
	return copied
}

// GetCatalogRevision returns the current revision number.
func GetCatalogRevision() uint64 {
	catalogStore.mu.RLock()
	defer catalogStore.mu.RUnlock()
	return catalogStore.revision
}

// ValidateCatalogJSON validates and parses raw model catalog JSON into ModelInfo items.
func ValidateCatalogJSON(data []byte) ([]pluginapi.ModelInfo, error) {
	var payload catalogFilePayload
	if err := json.Unmarshal(data, &payload); err == nil && len(payload.Models) > 0 {
		return parseCatalogItems(payload.Models)
	}

	// Try OpenAI format { "data": [ ... ] }
	var openAIPayload struct {
		Data []catalogModelItem `json:"data"`
	}
	if err := json.Unmarshal(data, &openAIPayload); err == nil && len(openAIPayload.Data) > 0 {
		return parseCatalogItems(openAIPayload.Data)
	}

	// Try pure array [ ... ]
	var arrayPayload []catalogModelItem
	if err := json.Unmarshal(data, &arrayPayload); err == nil && len(arrayPayload) > 0 {
		return parseCatalogItems(arrayPayload)
	}

	return nil, fmt.Errorf("catalog JSON has no recognized models array")
}

func parseCatalogItems(items []catalogModelItem) ([]pluginapi.ModelInfo, error) {
	created := time.Now().Unix()
	seen := make(map[string]struct{}, len(items))
	result := make([]pluginapi.ModelInfo, 0, len(items))

	for _, item := range items {
		slug := strings.TrimSpace(item.Slug)
		if slug == "" {
			slug = strings.TrimSpace(item.ID)
		}
		if slug == "" {
			continue
		}

		cleanSlug := strings.TrimPrefix(slug, "mirasim/")
		if _, exists := seen[cleanSlug]; exists {
			continue
		}
		seen[cleanSlug] = struct{}{}

		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" {
			displayName = strings.TrimSpace(item.Label)
		}
		if displayName == "" {
			displayName = cleanSlug
		}

		ownedBy := strings.TrimSpace(item.OwnedBy)
		if ownedBy == "" {
			ownedBy = detectOwner(cleanSlug)
		}

		modelType := strings.TrimSpace(item.Type)
		if modelType == "" {
			modelType = "chat"
		}

		info := pluginapi.ModelInfo{
			ID:          "mirasim/" + cleanSlug,
			Object:      "model",
			Created:     created,
			OwnedBy:     ownedBy,
			Type:        modelType,
			DisplayName: displayName,
		}
		result = append(result, info)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no valid models found in catalog")
	}
	return result, nil
}

func loadCatalogFromBytes(data []byte, source string) error {
	models, err := ValidateCatalogJSON(data)
	if err != nil {
		return fmt.Errorf("%s: %w", source, err)
	}

	catalogStore.mu.Lock()
	defer catalogStore.mu.Unlock()

	catalogStore.models = models
	catalogStore.bySlug = make(map[string]pluginapi.ModelInfo, len(models))
	for _, m := range models {
		clean := strings.TrimPrefix(m.ID, "mirasim/")
		catalogStore.bySlug[clean] = m
	}
	catalogStore.revision++
	catalogStore.lastUpdated = time.Now()
	return nil
}

// RefreshModelsFromUpstream dynamically queries upstream relay (/v1/models) and updates the in-memory catalog.
func RefreshModelsFromUpstream(ctx context.Context, token, relayURL string) (bool, error) {
	if relayURL == "" {
		relayURL = "https://relay.mirasim.ai"
	}
	url := strings.TrimRight(relayURL, "/") + "/v1/models"

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("create models request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("x-api-key", token)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("upstream models fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("upstream models returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return false, fmt.Errorf("read upstream models: %w", err)
	}

	if err := loadCatalogFromBytes(body, "upstream-relay"); err != nil {
		return false, err
	}

	return true, nil
}

func detectOwner(slug string) string {
	lower := strings.ToLower(slug)
	switch {
	case strings.HasPrefix(lower, "claude-"):
		return "anthropic"
	case strings.HasPrefix(lower, "gpt-"), strings.HasPrefix(lower, "o1"), strings.HasPrefix(lower, "o3"):
		return "openai"
	case strings.HasPrefix(lower, "gemini-"):
		return "google"
	case strings.HasPrefix(lower, "deepseek-"):
		return "deepseek"
	case strings.HasPrefix(lower, "kimi-"):
		return "moonshot"
	case strings.HasPrefix(lower, "qwen-"):
		return "alibaba"
	case strings.HasPrefix(lower, "grok-"):
		return "xai"
	case strings.HasPrefix(lower, "glm-"):
		return "zhipu"
	default:
		return "mirasim"
	}
}
