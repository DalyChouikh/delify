package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env.local")
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	client, err := disgo.New(os.Getenv("DISCORD_TOKEN"),
		bot.WithGatewayConfigOpts(gateway.WithIntents(
			gateway.IntentGuilds,
			gateway.IntentGuildMessages,
			gateway.IntentDirectMessages,
			gateway.IntentGuildVoiceStates,
		)),
		bot.WithEventListenerFunc(onMessageCreate),
	)
	if err != nil {
		panic(err)
	}
	// connect to the gateway
	if err = client.OpenGateway(context.TODO()); err != nil {
		panic(err)
	}
	log.Println("Bot is running")

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	<-s
}

func onMessageCreate(e *events.MessageCreate) {
	if e.Message.Author.Bot {
		return
	}
	var message string = fmt.Sprintf("Received message: %s", e.Message.Content)
	log.Println("Content: " + e.Message.Content)
	_, _ = e.Client().Rest().CreateMessage(e.ChannelID, discord.NewMessageCreateBuilder().SetContent(message).Build())
}
