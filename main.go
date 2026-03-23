package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	gotgbot "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	tgHandlers "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	messageFilters "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/oybek/ho/internal/db"
	appHandlers "github.com/oybek/ho/internal/handlers"
)

type Config struct {
	TelegramBotToken string
	DatabaseURL      string
}

func loadConfig() *Config {
	return &Config{
		TelegramBotToken: os.Getenv("TG_BOT_TOKEN"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
	}
}

func main() {
	cfg := loadConfig()
	if cfg.TelegramBotToken == "" {
		log.Fatal("TG_BOT_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	store, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer store.Close()

	state := appHandlers.NewState()

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

	dispatcher.AddHandler(tgHandlers.NewMessage(messageFilters.Location, appHandlers.HandleLocation(state)))
	dispatcher.AddHandler(tgHandlers.NewMessage(messageFilters.Photo, appHandlers.HandlePhoto(state)))
	dispatcher.AddHandler(tgHandlers.NewMessage(messageFilters.Text, appHandlers.HandleText(state, store)))

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
