// Package errors provides user-friendly error messages.
package errors

import (
	"github.com/bwmarrin/discordgo"
	"github.com/DalyChouikh/delify/internal/embed"
)

// ErrorCode represents a specific error type.
type ErrorCode string

const (
	ErrNotInVoice       ErrorCode = "NOT_IN_VOICE"
	ErrDifferentChannel ErrorCode = "DIFFERENT_CHANNEL"
	ErrQueueEmpty       ErrorCode = "QUEUE_EMPTY"
	ErrNothingPlaying   ErrorCode = "NOTHING_PLAYING"
	ErrTrackFailed      ErrorCode = "TRACK_FAILED"
	ErrNoResults        ErrorCode = "NO_RESULTS"
	ErrSeekInvalid      ErrorCode = "SEEK_INVALID"
	ErrAlreadyPaused    ErrorCode = "ALREADY_PAUSED"
	ErrNotPaused        ErrorCode = "NOT_PAUSED"
	ErrNoPermission     ErrorCode = "NO_PERMISSION"
	ErrInvalidInput     ErrorCode = "INVALID_INPUT"
	ErrInternal         ErrorCode = "INTERNAL"
)

// errorMessages maps error codes to user-friendly messages.
var errorMessages = map[ErrorCode]struct {
	Title   string
	Message string
}{
	ErrNotInVoice: {
		Title:   "Not in Voice Channel",
		Message: "You need to be in a voice channel to use this command!",
	},
	ErrDifferentChannel: {
		Title:   "Different Voice Channel",
		Message: "You must be in the same voice channel as me to use this command!",
	},
	ErrQueueEmpty: {
		Title:   "Queue Empty",
		Message: "The queue is empty! Add some songs with `/play`.",
	},
	ErrNothingPlaying: {
		Title:   "Nothing Playing",
		Message: "Nothing is playing right now! Use `/play` to start listening.",
	},
	ErrTrackFailed: {
		Title:   "Playback Failed",
		Message: "Failed to play this track. It might be unavailable or restricted. Try another one!",
	},
	ErrNoResults: {
		Title:   "No Results",
		Message: "No results found for your search. Try a different query!",
	},
	ErrSeekInvalid: {
		Title:   "Invalid Seek Position",
		Message: "Can't seek to that position! Make sure it's within the track duration.",
	},
	ErrAlreadyPaused: {
		Title:   "Already Paused",
		Message: "Playback is already paused! Use the resume button or `/resume` to continue.",
	},
	ErrNotPaused: {
		Title:   "Not Paused",
		Message: "Playback is not paused! Use the pause button or `/pause` to pause.",
	},
	ErrNoPermission: {
		Title:   "No Permission",
		Message: "You don't have permission to use this command!",
	},
	ErrInvalidInput: {
		Title:   "Invalid Input",
		Message: "The input you provided is invalid. Please check and try again.",
	},
	ErrInternal: {
		Title:   "Something Went Wrong",
		Message: "An unexpected error occurred. Please try again later.",
	},
}

// UserError represents an error that can be displayed to users.
type UserError struct {
	Code    ErrorCode
	Title   string
	Message string
	Details string // Optional additional details
}

// New creates a new UserError from an error code.
func New(code ErrorCode) *UserError {
	info := errorMessages[code]
	if info.Title == "" {
		info = errorMessages[ErrInternal]
	}
	return &UserError{
		Code:    code,
		Title:   info.Title,
		Message: info.Message,
	}
}

// WithDetails adds details to the error.
func (e *UserError) WithDetails(details string) *UserError {
	e.Details = details
	return e
}

// Error implements the error interface.
func (e *UserError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// ToEmbed converts the error to a Discord embed.
func (e *UserError) ToEmbed() *discordgo.MessageEmbed {
	description := e.Message
	if e.Details != "" {
		description += "\n\n*" + e.Details + "*"
	}

	return embed.New().
		Color(embed.ColorError).
		Title("❌ " + e.Title).
		Description(description).
		Build()
}

// ToEmbeds wraps the embed in a slice for easy use.
func (e *UserError) ToEmbeds() []*discordgo.MessageEmbed {
	return []*discordgo.MessageEmbed{e.ToEmbed()}
}

// EphemeralResponse creates an ephemeral interaction response with the error.
func (e *UserError) EphemeralResponse() *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: e.ToEmbeds(),
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	}
}

// EphemeralWebhookEdit creates a webhook edit with the error for deferred responses.
func (e *UserError) EphemeralWebhookEdit() *discordgo.WebhookEdit {
	embeds := e.ToEmbeds()
	return &discordgo.WebhookEdit{
		Embeds: &embeds,
	}
}
