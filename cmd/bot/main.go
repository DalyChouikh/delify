package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DalyChouikh/delify/configs"
	"github.com/DalyChouikh/delify/internal/bot/service"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(".env.local"); err != nil {
		log.Fatal("Error loading .env file")
	}

	config := configs.NewConfig(os.Getenv("DISCORD_TOKEN"))

	botService, err := service.NewBotService(config.DiscordToken)
	if err != nil {
		log.Fatal(err)
	}

	if err := botService.Start(context.Background()); err != nil {
		log.Fatal(err)
	}

	log.Println("Bot is running")

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	<-s

	botService.Close()
}
