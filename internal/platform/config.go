package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Group    string    `json:"group"`
	Students [2]string `json:"students"`
	Work     string    `json:"work"`
	Mode     string    `json:"mode"` // screen | window | area
	SignURL  string    `json:"sign_url,omitempty"`
}

func LoadConfig() Config {
	var c Config
	data, err := os.ReadFile(filepath.Join(ConfigDir(), "config.json"))
	if err == nil {
		_ = json.Unmarshal(data, &c)
	}
	if c.Mode == "" {
		c.Mode = "screen"
	}
	return c
}

func SaveConfig(c Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	tmp := filepath.Join(dir, "config.json.tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "config.json"))
}
