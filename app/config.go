package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// config holds defaults loaded from <configDir>/config.json. Command-line flags
// override these values.
type config struct {
	ProxyAddr   string   `json:"proxyAddr"`
	APIAddr     string   `json:"apiAddr"`
	Upstream    string   `json:"upstream"`
	OpenBrowser bool     `json:"openBrowser"`
	Scope       []string `json:"scope"`
}

// loadConfig reads config.json if present, falling back to built-in defaults.
func loadConfig() config {
	cfg := config{
		ProxyAddr:   "127.0.0.1:8080",
		APIAddr:     "127.0.0.1:8081",
		OpenBrowser: true,
	}
	path := filepath.Join(configDir(), "config.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	// Decode onto the defaults so missing keys keep their default.
	_ = json.Unmarshal(b, &cfg)
	if cfg.ProxyAddr == "" {
		cfg.ProxyAddr = "127.0.0.1:8080"
	}
	if cfg.APIAddr == "" {
		cfg.APIAddr = "127.0.0.1:8081"
	}
	return cfg
}
