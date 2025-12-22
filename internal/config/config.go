// Package config handles application configuration from environment variables.
package config

import (
	"os"
	"strconv"
)

// Config holds all application configuration.
type Config struct {
	Discord  DiscordConfig
	Lavalink LavalinkConfig
	LogLevel string
}

// DiscordConfig holds Discord-specific configuration.
type DiscordConfig struct {
	Token   string
	GuildID string
}

// LavalinkConfig holds Lavalink server configuration.
type LavalinkConfig struct {
	Host     string
	Port     int
	Password string
	Secure   bool
}

// Load reads configuration from environment variables.
func Load() *Config {
	return &Config{
		Discord: DiscordConfig{
			Token:   getEnv("DISCORD_TOKEN", ""),
			GuildID: getEnv("DISCORD_GUILD_ID", ""),
		},
		Lavalink: LavalinkConfig{
			Host:     getEnv("LAVALINK_HOST", "localhost"),
			Port:     getEnvAsInt("LAVALINK_PORT", 2333),
			Password: getEnv("LAVALINK_PASSWORD", "youshallnotpass"),
			Secure:   getEnvAsBool("LAVALINK_SECURE", false),
		},
		LogLevel: getEnv("LOG_LEVEL", "info"),
	}
}

// Validate checks if required configuration is present.
func (c *Config) Validate() error {
	if c.Discord.Token == "" {
		return ErrMissingDiscordToken
	}
	return nil
}

// getEnv retrieves an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt retrieves an environment variable as an integer.
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvAsBool retrieves an environment variable as a boolean.
func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
