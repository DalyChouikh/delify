package embed

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/DalyChouikh/delify/internal/utils"
)

// Builder helps construct Discord embeds with a fluent API.
type Builder struct {
	embed *discordgo.MessageEmbed
}

// New creates a new embed builder.
func New() *Builder {
	return &Builder{
		embed: &discordgo.MessageEmbed{},
	}
}

// Title sets the embed title.
func (b *Builder) Title(title string) *Builder {
	b.embed.Title = title
	return b
}

// TitleWithURL sets the embed title with a URL.
func (b *Builder) TitleWithURL(title, url string) *Builder {
	b.embed.Title = title
	b.embed.URL = url
	return b
}

// Description sets the embed description.
func (b *Builder) Description(desc string) *Builder {
	b.embed.Description = desc
	return b
}

// Color sets the embed color.
func (b *Builder) Color(color int) *Builder {
	b.embed.Color = color
	return b
}

// Thumbnail sets the embed thumbnail.
func (b *Builder) Thumbnail(url string) *Builder {
	if url != "" {
		b.embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: url,
		}
	}
	return b
}

// Image sets the embed image.
func (b *Builder) Image(url string) *Builder {
	if url != "" {
		b.embed.Image = &discordgo.MessageEmbedImage{
			URL: url,
		}
	}
	return b
}

// Author sets the embed author with icon.
func (b *Builder) Author(name, iconURL string) *Builder {
	b.embed.Author = &discordgo.MessageEmbedAuthor{
		Name:    name,
		IconURL: iconURL,
	}
	return b
}

// AuthorWithURL sets the embed author with icon and URL.
func (b *Builder) AuthorWithURL(name, iconURL, url string) *Builder {
	b.embed.Author = &discordgo.MessageEmbedAuthor{
		Name:    name,
		IconURL: iconURL,
		URL:     url,
	}
	return b
}

// Footer sets the embed footer.
func (b *Builder) Footer(text, iconURL string) *Builder {
	b.embed.Footer = &discordgo.MessageEmbedFooter{
		Text:    text,
		IconURL: iconURL,
	}
	return b
}

// Field adds a field to the embed.
func (b *Builder) Field(name, value string, inline bool) *Builder {
	b.embed.Fields = append(b.embed.Fields, &discordgo.MessageEmbedField{
		Name:   name,
		Value:  value,
		Inline: inline,
	})
	return b
}

// Timestamp sets the embed timestamp.
func (b *Builder) Timestamp(ts string) *Builder {
	b.embed.Timestamp = ts
	return b
}

// Build returns the constructed embed.
func (b *Builder) Build() *discordgo.MessageEmbed {
	return b.embed
}

// DeveloperFooter creates a standard footer with developer info.
func DeveloperFooter(developerAvatarURL string) *discordgo.MessageEmbedFooter {
	return &discordgo.MessageEmbedFooter{
		Text:    "Developed by Daly ❤️",
		IconURL: developerAvatarURL,
	}
}

// BotAuthor creates a standard author section with bot info.
func BotAuthor(title, botAvatarURL string) *discordgo.MessageEmbedAuthor {
	return &discordgo.MessageEmbedAuthor{
		Name:    title,
		IconURL: botAvatarURL,
	}
}

// GetArtworkURL extracts artwork URL from track, preferring Spotify over YouTube.
func GetArtworkURL(artworkURL, sourceType string) string {
	// If we have an artwork URL, use it
	if artworkURL != "" {
		return artworkURL
	}
	// Return empty if no artwork available
	return ""
}

// MentionUser creates a user mention string.
func MentionUser(userID string) string {
	return fmt.Sprintf("<@%s>", userID)
}

// FormatTrackField formats track info for embed fields.
func FormatTrackField(title, author string, maxLen int) string {
	display := fmt.Sprintf("%s - %s", author, title)
	return utils.Truncate(display, maxLen)
}
