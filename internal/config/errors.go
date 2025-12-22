package config

import "errors"

var (
	// ErrMissingDiscordToken is returned when DISCORD_TOKEN is not set.
	ErrMissingDiscordToken = errors.New("DISCORD_TOKEN environment variable is required")
)
