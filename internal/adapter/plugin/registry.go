package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"gitlab.com/tiny-services/multiverse-bot/internal/domain"
)

// builtinCommands cannot be overridden by plugins.
var builtinCommands = map[string]bool{
	"/start":      true,
	"/settings":   true,
	"/config":     true,
	"/cookies":    true,
	"/details":    true,
	"/save":       true,
	"/auth":       true,
	"/disconnect": true,
	"/watch":      true,
}

// pluginEntry holds a loaded plugin's client, manifest, and compiled URL patterns.
type pluginEntry struct {
	client   domain.PluginClient
	manifest domain.PluginManifest
	patterns []*regexp.Regexp // compiled URL patterns
}

// registry implements domain.PluginRegistry.
type registry struct {
	plugins    []pluginEntry
	commandMap map[string]int // command → index in plugins
	nameMap    map[string]int // name → index in plugins
}

const startupTimeout = 10 * time.Second

// LoadRegistry reads plugins.yml, health-checks each plugin, fetches manifests,
// validates them, and returns a ready-to-use PluginRegistry.
func LoadRegistry(configPath string, log *slog.Logger) (domain.PluginRegistry, error) {
	configs, err := loadConfig(configPath)
	if err != nil {
		return nil, err
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no enabled plugins in %s", configPath)
	}

	r := &registry{
		commandMap: make(map[string]int),
		nameMap:    make(map[string]int),
	}

	for _, cfg := range configs {
		client := newHTTPClient(cfg.Name, cfg.URL, cfg.Timeout)

		ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
		if err := client.Health(ctx); err != nil {
			cancel()
			log.Warn("plugin unhealthy, skipping", "plugin", cfg.Name, "error", err)
			continue
		}

		manifest, err := client.Manifest(ctx)
		cancel()
		if err != nil {
			log.Warn("plugin manifest failed, skipping", "plugin", cfg.Name, "error", err)
			continue
		}

		if manifest.Name != cfg.Name {
			log.Warn("plugin manifest name mismatch, skipping",
				"plugin", cfg.Name, "manifest_name", manifest.Name)
			continue
		}

		// Validate commands.
		valid := true
		for _, cmd := range manifest.Commands {
			if !strings.HasPrefix(cmd.Command, "/") {
				log.Warn("plugin command must start with /", "plugin", cfg.Name, "command", cmd.Command)
				valid = false
				break
			}
			if builtinCommands[cmd.Command] {
				log.Warn("plugin command collides with built-in", "plugin", cfg.Name, "command", cmd.Command)
				valid = false
				break
			}
			if _, exists := r.commandMap[cmd.Command]; exists {
				log.Warn("plugin command collides with another plugin", "plugin", cfg.Name, "command", cmd.Command)
				valid = false
				break
			}
		}
		if !valid {
			continue
		}

		// Compile URL patterns.
		var patterns []*regexp.Regexp
		for _, pat := range manifest.URLPatterns {
			compiled, err := regexp.Compile(pat)
			if err != nil {
				log.Warn("plugin URL pattern invalid, skipping plugin",
					"plugin", cfg.Name, "pattern", pat, "error", err)
				valid = false
				break
			}
			patterns = append(patterns, compiled)
		}
		if !valid {
			continue
		}

		idx := len(r.plugins)
		r.plugins = append(r.plugins, pluginEntry{
			client:   client,
			manifest: *manifest,
			patterns: patterns,
		})

		r.nameMap[cfg.Name] = idx
		for _, cmd := range manifest.Commands {
			r.commandMap[cmd.Command] = idx
		}

		log.Info("plugin loaded",
			"plugin", cfg.Name,
			"commands", len(manifest.Commands),
			"url_patterns", len(manifest.URLPatterns),
		)
	}

	return r, nil
}

func (r *registry) FindByCommand(command string) (domain.PluginClient, string) {
	if idx, ok := r.commandMap[command]; ok {
		return r.plugins[idx].client, r.plugins[idx].manifest.Name
	}
	return nil, ""
}

func (r *registry) FindByName(name string) domain.PluginClient {
	if idx, ok := r.nameMap[name]; ok {
		return r.plugins[idx].client
	}
	return nil
}

func (r *registry) FindByURL(url string) (domain.PluginClient, string, string) {
	for _, entry := range r.plugins {
		for i, pat := range entry.patterns {
			if pat.MatchString(url) {
				return entry.client, entry.manifest.Name, entry.manifest.URLPatterns[i]
			}
		}
	}
	return nil, "", ""
}

func (r *registry) AllManifests() []domain.PluginManifest {
	manifests := make([]domain.PluginManifest, len(r.plugins))
	for i, entry := range r.plugins {
		manifests[i] = entry.manifest
	}
	return manifests
}
