package handlers

import (
    "github.com/disgoorg/disgo/discord"
    "github.com/disgoorg/disgo/events"
)

type CommandHandler struct{}

func NewCommandHandler() *CommandHandler {
    return &CommandHandler{}
}

func (h *CommandHandler) HandleApplicationCommand(event *events.ApplicationCommandInteractionCreate) {
    data := event.Data
    switch data.CommandName() {
    case "ping":
        _ = event.CreateMessage(discord.NewMessageCreateBuilder().
            SetContent("Pong!").
            Build())
    }
}
