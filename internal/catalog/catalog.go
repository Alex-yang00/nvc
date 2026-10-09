// Package catalog fetches and caches Novita's /v1/models list.
//
// The launch path never blocks on the network for long: a cache of any age is used
// immediately (refreshed in the background when older than TTL), and with no cache
// we wait at most FetchTimeout before falling back to built-in lineup defaults.
package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

const (
	TTL          = 24 * time.Hour
	FetchTimeout = 2 * time.Second
)

type price struct {
	PerM string `json:"price_per_m_decimal"`
}

type Model struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"display_name"`
	ContextSize int      `json:"context_size"`
	MaxOutput   int      `json:"max_output_tokens"`
	Features    []string `json:"features"`
	Endpoints   []string `json:"endpoints"`
	ModelType   string   `json:"model_type"`
	Status      int      `json:"status"`
	Pricing     struct {
		Prompt     price `json:"prompt"`
		Completion price `json:"completion"`
		CacheRead  price `json:"input_cache_read"`
	} `json:"pricing"`
}

func (m Model) Supports(endpoint string) bool { return slices.Contains(m.Endpoints, endpoint) }

// InputPrice / OutputPrice are USD per 1M tokens.
func (m Model) InputPrice() float64 { f, _ := strconv.ParseFloat(m.Pricing.Prompt.PerM, 64); return f }
func (m Model) OutputPrice() float64 {
	f, _ := strconv.ParseFloat(m.Pricing.Completion.PerM, 64)
	return f
}

type Catalog struct {
	FetchedAt time.Time `json:"fetched_at"`
	Models    []Model   `json:"data"`
}

func (c *Catalog) Get(id string) (Model, bool) {
	if c == nil {
		return Model{}, false
	}
	for _, m := range c.Models {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}

// CachePath is the model list cache inside dir; CachePath(dir)+".tmp" may exist briefly while writing.
func CachePath(dir string) string { return filepath.Join(dir, "models.json") }

func Fetch(ctx context.Context, baseURL string) (*Catalog, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/openai/v1/models", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var c Catalog
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return nil, err
	}
	c.FetchedAt = time.Now()
	return &c, nil
}

func save(dir string, c *Catalog) {
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	b, _ := json.Marshal(c)
	tmp := CachePath(dir) + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, CachePath(dir))
	}
}

func loadCache(dir string) *Catalog {
	b, err := os.ReadFile(CachePath(dir))
	if err != nil {
		return nil
	}
	var c Catalog
	if json.Unmarshal(b, &c) != nil || len(c.Models) == 0 {
		return nil
	}
	return &c
}

// Load is for the launch path. It may return nil (offline, no cache); callers fall back to defaults.
// A stale cache is refreshed only when the caller is going to stay alive (refreshStale),
// because after syscall.Exec any background goroutine dies with us.
func Load(baseURL, cacheDir string, refreshStale bool) *Catalog {
	if c := loadCache(cacheDir); c != nil {
		if refreshStale && time.Since(c.FetchedAt) > TTL {
			ctx, cancel := context.WithTimeout(context.Background(), FetchTimeout)
			defer cancel()
			if fresh, err := Fetch(ctx, baseURL); err == nil {
				save(cacheDir, fresh)
				return fresh
			}
		}
		return c
	}
	ctx, cancel := context.WithTimeout(context.Background(), FetchTimeout)
	defer cancel()
	c, err := Fetch(ctx, baseURL)
	if err != nil {
		return nil
	}
	save(cacheDir, c)
	return c
}

// Refresh always hits the network (for `nvc models`).
func Refresh(ctx context.Context, baseURL, cacheDir string) (*Catalog, error) {
	c, err := Fetch(ctx, baseURL)
	if err != nil {
		if cached := loadCache(cacheDir); cached != nil {
			return cached, nil
		}
		return nil, err
	}
	save(cacheDir, c)
	return c, nil
}
