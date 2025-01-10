package service

import (
	"context"

	"github.com/DalyChouikh/delify/internal/bot/commands"
	"github.com/DalyChouikh/delify/internal/bot/handlers"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
)

type BotService struct {
	client  bot.Client
	handler *handlers.CommandHandler
}

func NewBotService(token string) (*BotService, error) {
	handler := handlers.NewCommandHandler()

	client, err := disgo.New(token,
		bot.WithGatewayConfigOpts(gateway.WithIntents(
			gateway.IntentGuilds,
			gateway.IntentGuildMessages,
			gateway.IntentDirectMessages,
			gateway.IntentGuildVoiceStates,
			gateway.IntentMessageContent,
		)),
		bot.WithEventListeners(&events.ListenerAdapter{
			OnApplicationCommandInteraction: handler.HandleApplicationCommand,
		}),
	)

	if err != nil {
		return nil, err
	}

	return &BotService{
		client:  client,
		handler: handler,
	}, nil
}

func (s *BotService) Start(ctx context.Context) error {
	if _, err := s.client.Rest().SetGlobalCommands(s.client.ApplicationID(), commands.Commands); err != nil {
		return err
	}

	return s.client.OpenGateway(ctx)
}

func (s *BotService) Close() {
	s.client.Close(context.Background())
}
