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
	ChatID   int64
	MsgIDs   []int64
}

func locationKeyboard() *gotgbot.ReplyKeyboardMarkup {
	return &gotgbot.ReplyKeyboardMarkup{
		Keyboard: [][]gotgbot.KeyboardButton{
			{
				{
					Text:            "Capture the moment",
					RequestLocation: true,
				},
			},
		},
		ResizeKeyboard: true,
	}
}

func replyWithKeyboard(b *gotgbot.Bot, msg *gotgbot.Message, text string) (*gotgbot.Message, error) {
	resp, err := msg.Reply(b, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: locationKeyboard(),
	})
	return resp, err
}

func sendWithKeyboard(b *gotgbot.Bot, chatID int64, text string) (*gotgbot.Message, error) {
	return b.SendMessage(chatID, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: locationKeyboard(),
	})
}

func addMsgID(draft *userDraft, msg *gotgbot.Message) {
	if draft == nil || msg == nil {
		return
	}
	draft.MsgIDs = append(draft.MsgIDs, int64(msg.MessageId))
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
			Step:   stepNeedPhotos,
			ChatID: msg.Chat.Id,
		}
		addMsgID(draft, msg)

		state.mu.Lock()
		state.m[user.Id] = draft
		state.mu.Unlock()

		botMsg, err := replyWithKeyboard(b, msg, "Got your location. Please send one or more photos of this place.")
		addMsgID(draft, botMsg)
		return err
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
			_, err := replyWithKeyboard(b, msg, "Please share your location first.")
			return err
		}
		addMsgID(draft, msg)

		photoIDs := make([]string, 0, len(msg.Photo))
		for _, p := range msg.Photo {
			photoIDs = append(photoIDs, p.FileId)
		}
		draft.HereOnce.PhotoIDs = photoIDs
		draft.Step = stepNeedNote
		state.mu.Unlock()

		botMsg, err := replyWithKeyboard(b, msg, "Thanks! Now tell me how you feel here, right now.")
		addMsgID(draft, botMsg)
		return err
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
			_, err := replyWithKeyboard(b, msg, "Share your location to start.")
			return err
		}

		state.mu.Lock()
		draft := state.m[user.Id]
		if draft == nil {
			state.mu.Unlock()
			_, err := replyWithKeyboard(b, msg, "Share your location to start.")
			return err
		}

		switch draft.Step {
		case stepNeedPhotos:
			state.mu.Unlock()
			_, err := replyWithKeyboard(b, msg, "Please send one or more photos of this place.")
			return err
		case stepNeedNote:
			if len(draft.HereOnce.PhotoIDs) == 0 {
				state.mu.Unlock()
				_, err := replyWithKeyboard(b, msg, "Please send one or more photos of this place.")
				return err
			}
			addMsgID(draft, msg)
			draft.HereOnce.Note = msg.Text
			draft.HereOnce.Created = time.Now().UTC()
			toInsert := draft.HereOnce
			chatID := draft.ChatID
			msgIDs := append([]int64(nil), draft.MsgIDs...)
			delete(state.m, user.Id)
			state.mu.Unlock()

			if _, err := store.InsertHereOnce(context.Background(), &toInsert); err != nil {
				log.Printf("failed to insert here_once: %v", err)
				_, replyErr := replyWithKeyboard(b, msg, "Sorry, I couldn't save that. Please try again.")
				if replyErr != nil {
					log.Printf("failed to send error reply: %v", replyErr)
				}
				return err
			}
			go func(chatID int64, ids []int64) {
				time.Sleep(2 * time.Second)
				for _, id := range ids {
					_, err := b.DeleteMessage(chatID, id, nil)
					if err != nil {
						log.Printf("failed to delete message %d: %v", id, err)
					}
				}
			}(chatID, msgIDs)
			botMsg, err := sendWithKeyboard(b, chatID, "Saved! Share another location any time.")
			if err == nil {
				_ = botMsg
			}
			return err
		default:
			state.mu.Unlock()
			_, err := replyWithKeyboard(b, msg, "Share your location to start.")
			return err
		}
		return nil
	}
}
