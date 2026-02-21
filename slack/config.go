package slack

// Config holds Slack notification configuration. Agents embed this in their
// own config structs and populate it from YAML, env vars, or flags.
type Config struct {
	Enabled     bool     `yaml:"enabled" json:"enabled"`
	WebhookURL  string   `yaml:"webhook_url" json:"webhook_url"`
	BotToken    string   `yaml:"bot_token" json:"bot_token"`
	Channel     string   `yaml:"channel" json:"channel"`
	NotifyUsers []string `yaml:"notify_users" json:"notify_users"`
	Events      EventConfig `yaml:"events" json:"events"`
}

// EventConfig controls which event types trigger automatic Slack notifications
// when using NewEventHandler.
type EventConfig struct {
	OnError   bool `yaml:"on_error" json:"on_error"`
	OnResult  bool `yaml:"on_result" json:"on_result"`
	OnStart   bool `yaml:"on_start" json:"on_start"`
	OnToolUse bool `yaml:"on_tool_use" json:"on_tool_use"`
}

// IsConfigured returns true if the config has enough information to send messages.
func (c Config) IsConfigured() bool {
	return c.Enabled && (c.WebhookURL != "" || (c.BotToken != "" && c.Channel != ""))
}
