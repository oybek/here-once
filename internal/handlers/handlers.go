package handlers

import (
	"context"
	"log"
	"sync"
	"time"

	gotgbot "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	tgHandlers "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/oybek/ho/internal/db"
	"github.com/oybek/ho/internal/model"
)

type State struct {
	mu sync.Mutex
	m  map[int64]*userDraft
}

func NewState() *State {
	return &State{m: make(map[int64]*userDraft)}
}

type step int

const (
	stepNeedLocation step = iota
	stepNeedPhotos
	stepNeedNote
)

type userDraft struct {
	HereOnce model.HereOnce
	Step     step
}

func locationKeyboard() *gotgbot.ReplyKeyboardMarkup {
	return &gotgbot.ReplyKeyboardMarkup{
		Keyboard: [][]gotgbot.KeyboardButton{
			{
				{
					Text:            "Share location",
					RequestLocation: true,
				},
			},
		},
		ResizeKeyboard: true,
	}
}

func replyWithKeyboard(b *gotgbot.Bot, msg *gotgbot.Message, text string) error {
	_, err := msg.Reply(b, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: locationKeyboard(),
	})
	return err
}

func HandleLocation(state *State) tgHandlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		msg := ctx.EffectiveMessage
		if msg == nil || msg.Location == nil {
			return nil
		}
		user := ctx.EffectiveUser
		if user == nil {
			return nil
		}

		draft := &userDraft{
			HereOnce: model.HereOnce{
				Lat: msg.Location.Latitude,
				Lon: msg.Location.Longitude,
			},
			Step: stepNeedPhotos,
		}

		state.mu.Lock()
		state.m[user.Id] = draft
		state.mu.Unlock()

		return replyWithKeyboard(b, msg, "Got your location. Please send one or more photos of this place.")
	}
}

func HandlePhoto(state *State) tgHandlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		msg := ctx.EffectiveMessage
		if msg == nil || len(msg.Photo) == 0 {
			return nil
		}
		user := ctx.EffectiveUser
		if user == nil {
			return nil
		}

		state.mu.Lock()
		draft := state.m[user.Id]
		if draft == nil || draft.Step != stepNeedPhotos {
			state.mu.Unlock()
			return replyWithKeyboard(b, msg, "Please share your location first.")
		}

		photoIDs := make([]string, 0, len(msg.Photo))
		for _, p := range msg.Photo {
			photoIDs = append(photoIDs, p.FileId)
		}
		draft.HereOnce.PhotoIDs = photoIDs
		draft.Step = stepNeedNote
		state.mu.Unlock()

		return replyWithKeyboard(b, msg, "Thanks! Now tell me how you feel here, right now.")
	}
}

func HandleText(state *State, store *db.Store) tgHandlers.Response {
	return func(b *gotgbot.Bot, ctx *ext.Context) error {
		msg := ctx.EffectiveMessage
		if msg == nil || msg.Text == "" {
			return nil
		}
		user := ctx.EffectiveUser
		if user == nil {
			return nil
		}

		if msg.Text == "/start" {
			return replyWithKeyboard(b, msg, "Share your location to start.")
		}

		var (
			toInsert model.HereOnce
			doInsert bool
		)

		state.mu.Lock()
		draft := state.m[user.Id]
		if draft == nil {
			state.mu.Unlock()
			return replyWithKeyboard(b, msg, "Share your location to start.")
		}

		switch draft.Step {
		case stepNeedPhotos:
			state.mu.Unlock()
			return replyWithKeyboard(b, msg, "Please send one or more photos of this place.")
		case stepNeedNote:
			if len(draft.HereOnce.PhotoIDs) == 0 {
				state.mu.Unlock()
				return replyWithKeyboard(b, msg, "Please send one or more photos of this place.")
			}
			draft.HereOnce.Note = msg.Text
			draft.HereOnce.Created = time.Now().UTC()
			toInsert = draft.HereOnce
			doInsert = true
			delete(state.m, user.Id)
			state.mu.Unlock()
		default:
			state.mu.Unlock()
			return replyWithKeyboard(b, msg, "Share your location to start.")
		}

		if doInsert {
			if _, err := store.InsertHereOnce(context.Background(), &toInsert); err != nil {
				log.Printf("failed to insert here_once: %v", err)
				return replyWithKeyboard(b, msg, "Sorry, I couldn't save that. Please try again.")
			}
			return replyWithKeyboard(b, msg, "Saved! Share another location any time.")
		}

		return nil
	}
}
