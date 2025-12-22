package embed

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/DalyChouikh/delify/internal/utils"
	"github.com/disgoorg/disgolink/v3/lavalink"
)

// TemplateConfig holds common configuration for embed templates.
type TemplateConfig struct {
	BotAvatarURL       string
	DeveloperAvatarURL string
}

// Templates provides pre-built embed templates.
type Templates struct {
	config TemplateConfig
}

// NewTemplates creates a new Templates instance.
func NewTemplates(config TemplateConfig) *Templates {
	return &Templates{config: config}
}

// TrackInfo contains information about a track for display.
type TrackInfo struct {
	Title          string
	Author         string
	URL            string
	Duration       lavalink.Duration
	ArtworkURL     string
	RequestedByID  string
	RequestedBy    string
	Position       int
	QueueLength    int
	NextTrackTitle string
	NextTrackAuthor string
}

// NowPlaying creates a now playing embed.
func (t *Templates) NowPlaying(track TrackInfo) *discordgo.MessageEmbed {
	durationStr := utils.FormatDuration(track.Duration)

	embed := New().
		Color(ColorNowPlaying).
		TitleWithURL(track.Title, track.URL).
		Thumbnail(track.ArtworkURL).
		Field("Duration", durationStr, true).
		Field("Requested by", MentionUser(track.RequestedByID), true)

	// Add position if in a queue
	if track.QueueLength > 0 {
		embed.Field("Queue", fmt.Sprintf("%d songs", track.QueueLength), true)
	}

	// Add next track info if available
	if track.NextTrackTitle != "" {
		nextUp := FormatTrackField(track.NextTrackTitle, track.NextTrackAuthor, 50)
		embed.Field("⏭️ Up Next", nextUp, false)
	}

	// Set author (bot) and footer (developer)
	result := embed.Build()
	result.Author = BotAuthor("Now Playing 🎵", t.config.BotAvatarURL)
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)
	result.Timestamp = time.Now().Format(time.RFC3339)

	return result
}

// AddedToQueue creates an embed for when a track is added to queue.
func (t *Templates) AddedToQueue(track TrackInfo) *discordgo.MessageEmbed {
	durationStr := utils.FormatDuration(track.Duration)

	embed := New().
		Color(ColorInfo).
		TitleWithURL(track.Title, track.URL).
		Thumbnail(track.ArtworkURL).
		Field("Duration", durationStr, true).
		Field("Position", fmt.Sprintf("#%d", track.Position), true).
		Field("Requested by", MentionUser(track.RequestedByID), true)

	result := embed.Build()
	result.Author = BotAuthor("Added to Queue 📝", t.config.BotAvatarURL)
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)

	return result
}

// PlaylistAdded creates an embed for when a playlist is added.
func (t *Templates) PlaylistAdded(name string, trackCount int, requestedByID string) *discordgo.MessageEmbed {
	embed := New().
		Color(ColorSuccess).
		Title(name).
		Field("Tracks Added", fmt.Sprintf("%d", trackCount), true).
		Field("Requested by", MentionUser(requestedByID), true)

	result := embed.Build()
	result.Author = BotAuthor("Playlist Added 📋", t.config.BotAvatarURL)
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)

	return result
}

// QueueItem represents a single item in the queue display.
type QueueItem struct {
	Position    int
	Title       string
	Author      string
	Duration    lavalink.Duration
	RequestedBy string
}

// QueueDisplay creates a queue list embed with pagination.
func (t *Templates) QueueDisplay(items []QueueItem, currentTrack *TrackInfo, page, totalPages, totalTracks int, totalDuration lavalink.Duration) *discordgo.MessageEmbed {
	embed := New().
		Color(ColorQueue)

	// Current track section
	if currentTrack != nil {
		nowPlaying := fmt.Sprintf("**[%s](%s)** - %s\n`%s`",
			utils.Truncate(currentTrack.Title, 40),
			currentTrack.URL,
			currentTrack.Author,
			utils.FormatDuration(currentTrack.Duration))
		embed.Field("🎵 Now Playing", nowPlaying, false)
	}

	// Queue items
	if len(items) > 0 {
		var queueText string
		for _, item := range items {
			line := fmt.Sprintf("`%d.` **%s** - %s [`%s`]\n",
				item.Position,
				utils.Truncate(item.Title, 35),
				utils.Truncate(item.Author, 20),
				utils.FormatDuration(item.Duration))
			queueText += line
		}
		embed.Field("📋 Up Next", queueText, false)
	} else if currentTrack == nil {
		embed.Description("The queue is empty! Use `/play` to add some songs.")
	}

	// Stats
	if totalTracks > 0 {
		stats := fmt.Sprintf("%d songs • %s total", totalTracks, utils.FormatDuration(totalDuration))
		embed.Field("📊 Queue Stats", stats, false)
	}

	result := embed.Build()
	result.Author = BotAuthor("Music Queue", t.config.BotAvatarURL)
	result.Footer = &discordgo.MessageEmbedFooter{
		Text:    fmt.Sprintf("Page %d/%d • Developed by Daly ❤️", page, totalPages),
		IconURL: t.config.DeveloperAvatarURL,
	}

	return result
}

// Skipped creates an embed for when a track is skipped.
func (t *Templates) Skipped(skippedTitle string, nextTrack *TrackInfo) *discordgo.MessageEmbed {
	embed := New().
		Color(ColorInfo).
		Description(fmt.Sprintf("⏭️ Skipped **%s**", utils.Truncate(skippedTitle, 50)))

	if nextTrack != nil {
		embed.Field("Now Playing", fmt.Sprintf("**%s** - %s", nextTrack.Title, nextTrack.Author), false)
	} else {
		embed.Field("Queue", "No more tracks in queue", false)
	}

	result := embed.Build()
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)

	return result
}

// Stopped creates an embed for when playback is stopped.
func (t *Templates) Stopped() *discordgo.MessageEmbed {
	embed := New().
		Color(ColorStopped).
		Description("⏹️ Playback stopped and queue cleared.")

	result := embed.Build()
	result.Author = BotAuthor("Stopped", t.config.BotAvatarURL)
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)

	return result
}

// Paused creates an embed for when playback is paused.
func (t *Templates) Paused(track TrackInfo, position lavalink.Duration) *discordgo.MessageEmbed {
	embed := New().
		Color(ColorWarning).
		Title(track.Title).
		Thumbnail(track.ArtworkURL).
		Field("Paused at", utils.FormatDuration(position), true).
		Field("Total", utils.FormatDuration(track.Duration), true)

	result := embed.Build()
	result.Author = BotAuthor("Paused ⏸️", t.config.BotAvatarURL)
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)

	return result
}

// Resumed creates an embed for when playback is resumed.
func (t *Templates) Resumed(track TrackInfo) *discordgo.MessageEmbed {
	embed := New().
		Color(ColorSuccess).
		Description(fmt.Sprintf("▶️ Resumed **%s**", utils.Truncate(track.Title, 50)))

	result := embed.Build()
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)

	return result
}

// Seeked creates an embed for when the track position is changed.
func (t *Templates) Seeked(track TrackInfo, newPosition lavalink.Duration) *discordgo.MessageEmbed {
	embed := New().
		Color(ColorInfo).
		Description(fmt.Sprintf("⏩ Seeked to **%s** / %s",
			utils.FormatDuration(newPosition),
			utils.FormatDuration(track.Duration)))

	result := embed.Build()
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)

	return result
}

// QueueCleared creates an embed for when the queue is cleared.
func (t *Templates) QueueCleared(count int) *discordgo.MessageEmbed {
	embed := New().
		Color(ColorInfo).
		Description(fmt.Sprintf("🗑️ Cleared **%d** tracks from the queue.", count))

	result := embed.Build()
	result.Footer = DeveloperFooter(t.config.DeveloperAvatarURL)

	return result
}
