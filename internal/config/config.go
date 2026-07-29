// Package config loads and validates the agent's runtime configuration.
package config

import (
	"encoding/json"
	"errors"
	"os"
)

// Config is the agent's runtime configuration, read from a JSON file.
type Config struct {
	BaseURL     string `json:"base_url"`     // e.g. https://invoicesup.kordino.com
	Token       string `json:"token"`        // connector personal access token
	Folder      string `json:"folder"`       // where {code}_facturas.txt files are written
	PollSeconds int    `json:"poll_seconds"` // interval between polls (>= 5)
}

// Load reads and validates the config file. poll_seconds defaults to 30.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	if c.PollSeconds == 0 {
		c.PollSeconds = 30
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) validate() error {
	switch {
	case c.BaseURL == "":
		return errors.New("base_url is required")
	case c.Token == "":
		return errors.New("token is required")
	case c.Folder == "":
		return errors.New("folder is required")
	case c.PollSeconds < 5:
		return errors.New("poll_seconds must be >= 5")
	}
	return nil
}
