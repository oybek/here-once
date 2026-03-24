package handlers

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	gotgbot "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	tgHandlers "github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/oybek/ho/internal/db"
	"github.com/oybek/ho/internal/model"
	"github.com/oybek/ho/internal/text"
)

type State struct {
	mu sync.Mutex
	m  map[int64]*userDraft
}

func NewState() *State {
	return &State{
		m: make(map[int64]*userDraft),
	}
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
					Text:            text.KeyboardShareLocation,
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

func pickBestPhotoID(photos []gotgbot.PhotoSize) string {
	var (
		bestID   string
		bestSize int
		bestArea int
	)

	for _, p := range photos {
		if p.FileId == "" {
			continue
		}
		if p.FileSize > 0 {
			if int(p.FileSize) > bestSize {
				bestSize = int(p.FileSize)
				bestID = p.FileId
			}
			continue
		}
		area := int(p.Width) * int(p.Height)
		if area > bestArea {
			bestArea = area
			bestID = p.FileId
		}
	}

	return bestID
}

func sendRemember(b *gotgbot.Bot, store *db.Store, msg *gotgbot.Message, userID int64) error {
	latest, err := store.LatestHereOnceByUser(context.Background(), userID)
	if err != nil {
		log.Printf("failed to fetch latest memory: %v", err)
		_, replyErr := replyWithKeyboard(b, msg, text.SaveFailedPrompt)
		if replyErr != nil {
			log.Printf("failed to send error reply: %v", replyErr)
		}
		return err
	}
	if latest == nil {
		_, err := replyWithKeyboard(b, msg, text.RememberNoneFound)
		return err
	}

	hereOnce, err := store.RandomHereOnceByUser(context.Background(), userID)
	if err != nil {
		log.Printf("failed to fetch memory: %v", err)
		_, replyErr := replyWithKeyboard(b, msg, text.SaveFailedPrompt)
		if replyErr != nil {
			log.Printf("failed to send error reply: %v", replyErr)
		}
		return err
	}
	if hereOnce == nil {
		_, err := replyWithKeyboard(b, msg, text.RememberNoneFound)
		return err
	}

	ago := humanizeDuration(time.Since(hereOnce.Created))
	agoLine := fmt.Sprintf("%s ago", ago)
	if ago == "just now" {
		agoLine = "Just now"
	}

	distance := distanceKm(latest.Lat, latest.Lon, hereOnce.Lat, hereOnce.Lon)
	body := fmt.Sprintf(
		"%s\nIn %.1f kilometers from here\nYou said: %s",
		agoLine,
		distance,
		hereOnce.Note,
	)

	if len(hereOnce.PhotoIDs) == 0 {
		_, err = replyWithKeyboard(b, msg, body)
		return err
	}

	limit := len(hereOnce.PhotoIDs)
	if limit > 10 {
		limit = 10
	}
	media := make([]gotgbot.InputMedia, 0, limit)
	for i, id := range hereOnce.PhotoIDs[:limit] {
		item := &gotgbot.InputMediaPhoto{Media: gotgbot.InputFileByID(id)}
		if i == 0 {
			item.Caption = body
		}
		media = append(media, item)
	}
	_, err = b.SendMediaGroup(msg.Chat.Id, media, nil)
	return err
}

func distanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0
	lat1 = degToRad(lat1)
	lon1 = degToRad(lon1)
	lat2 = degToRad(lat2)
	lon2 = degToRad(lon2)

	dlat := lat2 - lat1
	dlon := lon2 - lon1

	a := math.Sin(dlat/2)*math.Sin(dlat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
}

func degToRad(deg float64) float64 {
	return deg * math.Pi / 180
}

func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		return fmt.Sprintf("%d minute%s", mins, plural(mins))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%d hour%s", hours, plural(hours))
	}
	if d < 7*24*time.Hour {
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d day%s", days, plural(days))
	}
	if d < 30*24*time.Hour {
		weeks := int(d.Hours() / (24 * 7))
		return fmt.Sprintf("%d week%s", weeks, plural(weeks))
	}
	if d < 365*24*time.Hour {
		months := int(d.Hours() / (24 * 30))
		return fmt.Sprintf("%d month%s", months, plural(months))
	}
	years := int(d.Hours() / (24 * 365))
	return fmt.Sprintf("%d year%s", years, plural(years))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
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
				UserID: user.Id,
				Lat:    msg.Location.Latitude,
				Lon:    msg.Location.Longitude,
			},
			Step:   stepNeedPhotos,
			ChatID: msg.Chat.Id,
		}
		addMsgID(draft, msg)

		state.mu.Lock()
		state.m[user.Id] = draft
		state.mu.Unlock()

		botMsg, err := replyWithKeyboard(b, msg, text.LocationReceivedPrompt)
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
			_, err := replyWithKeyboard(b, msg, text.ShareLocationPrompt)
			return err
		}
		addMsgID(draft, msg)

		bestID := pickBestPhotoID(msg.Photo)
		if bestID != "" {
			draft.HereOnce.PhotoIDs = []string{bestID}
		} else {
			draft.HereOnce.PhotoIDs = nil
		}
		draft.Step = stepNeedNote
		state.mu.Unlock()

		botMsg, err := replyWithKeyboard(b, msg, text.PhotoReceivedPrompt)
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
			_, err := replyWithKeyboard(b, msg, text.ShareLocationPrompt)
			return err
		}
		if msg.Text == "/remember" {
			return sendRemember(b, store, msg, user.Id)
		}

		state.mu.Lock()
		draft := state.m[user.Id]
		if draft == nil {
			state.mu.Unlock()
			_, err := replyWithKeyboard(b, msg, text.ShareLocationPrompt)
			return err
		}

		switch draft.Step {
		case stepNeedPhotos:
			state.mu.Unlock()
			_, err := replyWithKeyboard(b, msg, text.NeedPhotosPrompt)
			return err
		case stepNeedNote:
			if len(draft.HereOnce.PhotoIDs) == 0 {
				state.mu.Unlock()
				_, err := replyWithKeyboard(b, msg, text.NeedPhotosPrompt)
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
				_, replyErr := replyWithKeyboard(b, msg, text.SaveFailedPrompt)
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
			botMsg, err := sendWithKeyboard(b, chatID, text.SaveSuccessPrompt)
			if err == nil {
				_ = botMsg
			}
			return err
		default:
			state.mu.Unlock()
			_, err := replyWithKeyboard(b, msg, text.ShareLocationPrompt)
			return err
		}
		return nil
	}
}
