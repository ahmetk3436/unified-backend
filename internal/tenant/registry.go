package tenant

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type AppConfig struct {
	AppID              string            `json:"app_id"`
	AppName            string            `json:"app_name"`
	BundleID           string            `json:"bundle_id"`
	AIProvider         string            `json:"ai_provider"`
	AIConfig           map[string]string `json:"ai_config"`
	Features           map[string]bool   `json:"features"`
	RevenueCatAuth     string            `json:"revenuecat_webhook_auth"`
	AppleClientIDs     []string          `json:"apple_client_ids"`
}

type AppsFile struct {
	Apps []AppConfig `json:"apps"`
}

type Registry struct {
	mu   sync.RWMutex
	apps map[string]*AppConfig
}

func NewRegistry() *Registry {
	return &Registry{
		apps: make(map[string]*AppConfig),
	}
}

func LoadFromFile(path string) (*Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read apps config: %w", err)
	}

	var file AppsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse apps config: %w", err)
	}

	registry := NewRegistry()
	for i := range file.Apps {
		registry.Register(&file.Apps[i])
	}
	return registry, nil
}

func (r *Registry) Register(cfg *AppConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.apps[cfg.AppID] = cfg
}

func (r *Registry) Get(appID string) *AppConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.apps[appID]
}

func (r *Registry) Exists(appID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.apps[appID]
	return ok
}

func (r *Registry) HasFeature(appID, feature string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.apps[appID]
	if !ok {
		return false
	}
	return cfg.Features[feature]
}

func (r *Registry) All() []*AppConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*AppConfig, 0, len(r.apps))
	for _, cfg := range r.apps {
		result = append(result, cfg)
	}
	return result
}

func (r *Registry) GetWebhookAuth(appID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.apps[appID]
	if !ok {
		return ""
	}
	return cfg.RevenueCatAuth
}

func (r *Registry) GetBundleID(appID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, ok := r.apps[appID]
	if !ok {
		return ""
	}
	return cfg.BundleID
}
