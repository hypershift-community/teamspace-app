package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the application configuration
type Config struct {
	Server struct {
		Port int `json:"port"`
	} `json:"server"`

	Session struct {
		HashKey  string `json:"hash_key"`
		BlockKey string `json:"block_key"`
	} `json:"session"`

	OAuth struct {
		GithubClientID     string `json:"github_client_id"`
		GithubClientSecret string `json:"github_client_secret"`
		RedirectURL        string `json:"redirect_url"`
	} `json:"oauth"`

	App struct {
		FrontendURL  string   `json:"frontend_url"`
		GithubOrg    string   `json:"github_org"`
		AllowedTeams []string `json:"allowed_teams"`
	} `json:"app"`
}

// LoadFromFile loads configuration from a JSON file
func LoadFromFile(filePath string) (*Config, error) {
	// Start with default configuration
	cfg := &Config{}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file does not exist: %s", filePath)
	}

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %v", err)
	}

	// Parse JSON
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("could not parse config file: %v", err)
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveToFile saves the configuration to a JSON file
func (c *Config) SaveToFile(filePath string) error {
	// Validate config before saving
	if err := c.Validate(); err != nil {
		return err
	}

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal config to JSON: %v", err)
	}

	// Write file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("could not write config file: %v", err)
	}

	return nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Ensure session keys are at least 32 bytes if provided
	if c.Session.HashKey != "" && len(c.Session.HashKey) < 32 {
		return fmt.Errorf("session hash key must be at least 32 bytes")
	}

	if c.Session.BlockKey != "" && len(c.Session.BlockKey) < 32 {
		return fmt.Errorf("session block key must be at least 32 bytes")
	}

	return nil
}
