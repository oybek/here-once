package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	gotgbot "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	messageFilters "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

type Config struct {
	TelegramBotToken string
}

func loadConfig() *Config {
	return &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
	}
}

func main() {
	cfg := loadConfig()
	if cfg.TelegramBotToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	bot, err := gotgbot.NewBot(cfg.TelegramBotToken, nil)
	if err != nil {
		log.Fatalf("failed to create bot: %v", err)
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Printf("handler error: %v", err)
			return ext.DispatcherActionNoop
		},
	})

	dispatcher.AddHandler(handlers.NewMessage(messageFilters.Text, func(b *gotgbot.Bot, ctx *ext.Context) error {
		if ctx.EffectiveMessage == nil || ctx.EffectiveMessage.Text == "" {
			return nil
		}
		_, err := ctx.EffectiveMessage.Reply(b, ctx.EffectiveMessage.Text, nil)
		return err
	}))

	updater := ext.NewUpdater(dispatcher, nil)

	if err := updater.StartPolling(bot, &ext.PollingOpts{DropPendingUpdates: true}); err != nil {
		log.Fatalf("failed to start polling: %v", err)
	}
	log.Printf("bot started as @%s", bot.User.Username)

	go func() {
		<-ctx.Done()
		updater.Stop()
	}()

	updater.Idle()
}
