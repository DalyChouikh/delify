// Package components provides Discord UI component builders.
package components

import "github.com/bwmarrin/discordgo"

// Button custom ID prefixes.
const (
	// Prefix for all Delify button IDs.
	ButtonPrefix = "delify:"

	// Playback control button IDs.
	ButtonPause  = ButtonPrefix + "pause"
	ButtonResume = ButtonPrefix + "resume"
	ButtonSkip   = ButtonPrefix + "skip"
	ButtonStop   = ButtonPrefix + "stop"
	ButtonClear  = ButtonPrefix + "clear"

	// Seek button IDs.
	ButtonSeekBack30  = ButtonPrefix + "seek:-30"
	ButtonSeekBack10  = ButtonPrefix + "seek:-10"
	ButtonSeekBack5   = ButtonPrefix + "seek:-5"
	ButtonSeekForward5  = ButtonPrefix + "seek:+5"
	ButtonSeekForward10 = ButtonPrefix + "seek:+10"
	ButtonSeekForward30 = ButtonPrefix + "seek:+30"

	// Queue navigation button IDs.
	ButtonQueuePrev = ButtonPrefix + "queue:prev"
	ButtonQueueNext = ButtonPrefix + "queue:next"
	ButtonQueueShow = ButtonPrefix + "queue:show"
)

// Button creates a new button component.
func Button(customID, label string, style discordgo.ButtonStyle, emoji *discordgo.ComponentEmoji, disabled bool) discordgo.Button {
	return discordgo.Button{
		CustomID: customID,
		Label:    label,
		Style:    style,
		Emoji:    emoji,
		Disabled: disabled,
	}
}

// Emoji creates a component emoji from a Unicode string.
func Emoji(e string) *discordgo.ComponentEmoji {
	return &discordgo.ComponentEmoji{
		Name: e,
	}
}

// PlaybackControlsRow creates the main playback control buttons.
// The pauseState determines whether to show Pause or Resume button.
func PlaybackControlsRow(isPaused bool) discordgo.ActionsRow {
	var pauseResumeButton discordgo.Button
	if isPaused {
		pauseResumeButton = Button(ButtonResume, "", discordgo.PrimaryButton, Emoji("▶️"), false)
	} else {
		pauseResumeButton = Button(ButtonPause, "", discordgo.PrimaryButton, Emoji("⏸️"), false)
	}

	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			Button(ButtonSeekBack30, "-30s", discordgo.SecondaryButton, nil, false),
			Button(ButtonSeekBack10, "-10s", discordgo.SecondaryButton, nil, false),
			pauseResumeButton,
			Button(ButtonSeekForward10, "+10s", discordgo.SecondaryButton, nil, false),
			Button(ButtonSeekForward30, "+30s", discordgo.SecondaryButton, nil, false),
		},
	}
}

// SeekFineRow creates fine-grained seek buttons.
func SeekFineRow() discordgo.ActionsRow {
	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			Button(ButtonSeekBack5, "-5s", discordgo.SecondaryButton, nil, false),
			Button(ButtonSeekForward5, "+5s", discordgo.SecondaryButton, nil, false),
			Button(ButtonSkip, "Skip", discordgo.PrimaryButton, Emoji("⏭️"), false),
			Button(ButtonClear, "Clear", discordgo.DangerButton, Emoji("🗑️"), false),
			Button(ButtonStop, "Stop", discordgo.DangerButton, Emoji("🛑"), false),
		},
	}
}

// NowPlayingComponents returns the full button layout for now playing embed.
func NowPlayingComponents(isPaused bool) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		PlaybackControlsRow(isPaused),
		SeekFineRow(),
	}
}

// QueueNavigationRow creates pagination buttons for queue display.
func QueueNavigationRow(page, totalPages int) discordgo.ActionsRow {
	return discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			Button(ButtonQueuePrev, "Previous", discordgo.SecondaryButton, Emoji("◀️"), page <= 1),
			Button(ButtonQueueNext, "Next", discordgo.SecondaryButton, Emoji("▶️"), page >= totalPages),
		},
	}
}

// QueueComponents returns button layout for queue embed.
func QueueComponents(page, totalPages int) []discordgo.MessageComponent {
	if totalPages <= 1 {
		return nil
	}
	return []discordgo.MessageComponent{
		QueueNavigationRow(page, totalPages),
	}
}

// DisabledPlaybackControls returns disabled playback controls.
func DisabledPlaybackControls() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				Button(ButtonSeekBack30, "-30s", discordgo.SecondaryButton, nil, true),
				Button(ButtonSeekBack10, "-10s", discordgo.SecondaryButton, nil, true),
				Button(ButtonPause, "", discordgo.PrimaryButton, Emoji("⏸️"), true),
				Button(ButtonSeekForward10, "+10s", discordgo.SecondaryButton, nil, true),
				Button(ButtonSeekForward30, "+30s", discordgo.SecondaryButton, nil, true),
			},
		},
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				Button(ButtonSeekBack5, "-5s", discordgo.SecondaryButton, nil, true),
				Button(ButtonSeekForward5, "+5s", discordgo.SecondaryButton, nil, true),
				Button(ButtonSkip, "Skip", discordgo.PrimaryButton, Emoji("⏭️"), true),
				Button(ButtonClear, "Clear", discordgo.DangerButton, Emoji("🗑️"), true),
				Button(ButtonStop, "Stop", discordgo.DangerButton, Emoji("🛑"), true),
			},
		},
	}
}
