// Package utils provides utility functions for formatting and helpers.
package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgolink/v3/lavalink"
)

// FormatDuration formats a lavalink.Duration to HH:MM:SS or MM:SS format.
func FormatDuration(d lavalink.Duration) string {
	duration := time.Duration(d) * time.Millisecond
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// FormatDurationMs formats milliseconds to HH:MM:SS or MM:SS format.
func FormatDurationMs(ms int64) string {
	return FormatDuration(lavalink.Duration(ms))
}

// ParseDuration parses a duration string (e.g., "1:30", "90", "1:30:00") to milliseconds.
func ParseDuration(s string) (int64, error) {
	s = strings.TrimSpace(s)

	// Try parsing as plain seconds
	if !strings.Contains(s, ":") {
		seconds, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration format")
		}
		return seconds * 1000, nil
	}

	parts := strings.Split(s, ":")
	var hours, minutes, seconds int64
	var err error

	switch len(parts) {
	case 2: // MM:SS
		minutes, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes")
		}
		seconds, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid seconds")
		}
	case 3: // HH:MM:SS
		hours, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hours")
		}
		minutes, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid minutes")
		}
		seconds, err = strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid seconds")
		}
	default:
		return 0, fmt.Errorf("invalid duration format")
	}

	totalMs := (hours*3600 + minutes*60 + seconds) * 1000
	return totalMs, nil
}

// Truncate truncates a string to the specified length, adding "..." if truncated.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// GetUserAvatarURL returns the avatar URL for a Discord user.
// If userID is empty, returns an empty string.
func GetUserAvatarURL(userID, avatarHash string) string {
	if userID == "" || avatarHash == "" {
		return ""
	}
	// Discord CDN URL for avatars
	return fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png?size=64", userID, avatarHash)
}

// GetDefaultAvatarURL returns the default Discord avatar URL for a user.
func GetDefaultAvatarURL(userID string) string {
	if userID == "" {
		return ""
	}
	// Parse the user ID to get discriminator for default avatar
	// Discord uses user_id % 5 for default avatars (or % 6 for new system)
	id, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return ""
	}
	index := (id >> 22) % 6
	return fmt.Sprintf("https://cdn.discordapp.com/embed/avatars/%d.png", index)
}

// CalculateQueueDuration calculates total duration of remaining tracks.
func CalculateQueueDuration(durations []lavalink.Duration) lavalink.Duration {
	var total lavalink.Duration
	for _, d := range durations {
		total += d
	}
	return total
}
