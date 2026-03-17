package domain

import "context"

// PluginManifest describes what a plugin can do.
type PluginManifest struct {
	Name        string
	Description string
	Commands    []PluginCommand
	URLPatterns []string // raw regex strings
}

// PluginCommand is a slash command provided by a plugin.
type PluginCommand struct {
	Command     string // e.g. "/tiktok"
	Description string
}

// PluginTrigger describes what triggered a plugin execution.
type PluginTrigger struct {
	Type           string // "command" or "url"
	Command        string // set when Type == "command"
	Args           string // command arguments
	URL            string // set when Type == "url"
	MatchedPattern string // the regex that matched
	RawText        string // original message text
}

// PluginUser is the minimal user info passed to plugins.
type PluginUser struct {
	ID       int64
	Username string
}

// PluginRequest is sent to a plugin's /execute endpoint.
type PluginRequest struct {
	Trigger   PluginTrigger
	User      PluginUser
	MessageID int
}

// PluginAction is a single action a plugin wants the bot to perform.
type PluginAction struct {
	Type      string         // "text", "file", "edit", "delete"
	Text      string         // for "text" and "edit"
	ParseMode string         // "HTML" or "Markdown"
	Buttons   []PluginButton // inline keyboard
	URL       string         // for "file": source URL
	Filename  string
	Caption   string
	MimeType  string
	MessageID int // for "edit" and "delete"
}

// PluginButton is an inline keyboard button.
type PluginButton struct {
	Text     string
	URL      string // open URL
	Callback string // callback data
}

// PluginResponse is returned by /execute and /callback.
type PluginResponse struct {
	Actions []PluginAction
	Toast   string // callback toast (only for /callback)
	Error   string // user-facing error message
}

// PluginCallbackRequest is sent to a plugin's /callback endpoint.
type PluginCallbackRequest struct {
	CallbackID string
	User       PluginUser
	MessageID  int
}

// PluginClient communicates with a single plugin's HTTP API.
type PluginClient interface {
	Health(ctx context.Context) error
	Manifest(ctx context.Context) (*PluginManifest, error)
	Execute(ctx context.Context, req *PluginRequest) (*PluginResponse, error)
	Callback(ctx context.Context, req *PluginCallbackRequest) (*PluginResponse, error)
}

// PluginRegistry holds all loaded plugins and routes triggers to them.
type PluginRegistry interface {
	// FindByCommand returns the plugin client and name for a given slash command, or nil.
	FindByCommand(command string) (PluginClient, string)
	// FindByURL returns the plugin client, name, and matched pattern for a URL, or nil.
	FindByURL(url string) (PluginClient, string, string)
	// FindByName returns the plugin client by plugin name, or nil.
	FindByName(name string) PluginClient
	// AllManifests returns manifests of all healthy plugins.
	AllManifests() []PluginManifest
}
