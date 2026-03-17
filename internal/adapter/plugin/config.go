package plugin

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// PluginConfig represents a single plugin entry in plugins.yml.
type PluginConfig struct {
	Name    string        `yaml:"name"`
	URL     string        `yaml:"url"`
	Enabled *bool         `yaml:"enabled"` // pointer to distinguish unset from false
	Timeout time.Duration `yaml:"timeout"`
}

// PluginsFile is the top-level structure of plugins.yml.
type PluginsFile struct {
	Plugins []PluginConfig `yaml:"plugins"`
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func loadConfig(path string) ([]PluginConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading plugins config: %w", err)
	}

	var f PluginsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing plugins config: %w", err)
	}

	seen := make(map[string]bool)
	var result []PluginConfig

	for _, p := range f.Plugins {
		if !nameRe.MatchString(p.Name) || len(p.Name) > 32 {
			return nil, fmt.Errorf("invalid plugin name: %q", p.Name)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("duplicate plugin name: %q", p.Name)
		}
		seen[p.Name] = true

		if p.URL == "" {
			return nil, fmt.Errorf("plugin %q: url is required", p.Name)
		}

		// Default enabled to true.
		if p.Enabled != nil && !*p.Enabled {
			continue
		}

		// Default timeout to 30s.
		if p.Timeout == 0 {
			p.Timeout = 30 * time.Second
		}

		result = append(result, p)
	}

	return result, nil
}
