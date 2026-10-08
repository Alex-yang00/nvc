// Package auth reads, validates and stores the Novita API key.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const EnvKey = "NOVITA_API_KEY"

var ErrNoKey = errors.New("no Novita API key: run `nvc login` or set NOVITA_API_KEY")

type config struct {
	APIKey string `json:"api_key"`
}

// ConfigDir is ~/.config/nvc on every Unix (not ~/Library on macOS) so docs stay one-liners.
func ConfigDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "nvc")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nvc")
}

func configPath() string { return filepath.Join(ConfigDir(), "config.json") }

// Key returns the API key: env var first, then config file.
func Key() (string, error) {
	if k := strings.TrimSpace(os.Getenv(EnvKey)); k != "" {
		return k, nil
	}
	b, err := os.ReadFile(configPath())
	if err != nil {
		return "", ErrNoKey
	}
	var c config
	if json.Unmarshal(b, &c) != nil || c.APIKey == "" {
		return "", ErrNoKey
	}
	return c.APIKey, nil
}

func Save(key string) error {
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(config{APIKey: key}, "", "  ")
	return os.WriteFile(configPath(), b, 0o600)
}

// Validate makes a 1-token chat call. /v1/models is public and accepts any key, so it can't be used.
func Validate(ctx context.Context, baseURL, key, model string) error {
	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"max_tokens":1}`, model)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", baseURL+"/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return errors.New("key rejected by Novita (401)")
	case resp.StatusCode >= 400:
		return fmt.Errorf("validation request failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Mask keeps the first 6 and last 4 characters.
func Mask(key string) string {
	if len(key) <= 12 {
		return "****"
	}
	return key[:6] + "…" + key[len(key)-4:]
}
