package tgbot

import (
	"context"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/logger"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const (
	// broadcastMaxEscapedLen bounds one broadcast message after HTML escaping.
	// SendMsgToTgbot delivers with ParseMode=HTML and Telegram rejects texts
	// over 4096 characters, so input whose escaped form exceeds this is
	// refused at the preview step instead of failing per-recipient.
	broadcastMaxEscapedLen = 3800
	// broadcastDelay spaces deliveries to stay well inside Telegram's flood
	// limits even for hundreds of recipients.
	broadcastDelay = 150 * time.Millisecond
)

// collectBroadcastRecipients returns the distinct Telegram chat IDs of every
// panel client that linked a Telegram account (TgID), across all inbounds
// (enabled or not — a disabled inbound's customers are still customers).
// Admins are excluded: they receive the broadcast summary instead.
func (t *Tgbot) collectBroadcastRecipients() []int64 {
	inbounds, err := t.inboundService.GetAllInbounds()
	if err != nil {
		logger.Warning("broadcast: unable to load inbounds", err)
		return nil
	}
	seen := make(map[int64]struct{})
	var recipients []int64
	for _, inbound := range inbounds {
		clients, err := t.inboundService.GetClients(inbound)
		if err != nil {
			continue
		}
		for _, client := range clients {
			if client.TgID == EmptyTelegramUserID {
				continue
			}
			if checkAdmin(client.TgID) {
				continue
			}
			if _, dup := seen[client.TgID]; dup {
				continue
			}
			seen[client.TgID] = struct{}{}
			recipients = append(recipients, client.TgID)
		}
	}
	return recipients
}

// broadcastPreviewText renders the confirmation message shown to the admin.
// The draft is HTML-escaped so the preview displays exactly the literal text
// recipients will receive.
func (t *Tgbot) broadcastPreviewText(draft string, count int) string {
	var b strings.Builder
	b.WriteString(t.I18nBot("tgbot.messages.broadcastPreview", "Count=="+strconv.Itoa(count)))
	b.WriteString("\r\n\r\n")
	b.WriteString(html.EscapeString(draft))
	return b.String()
}

// broadcastCancelKeyboard returns the single Cancel button shown with the
// broadcast prompt.
func (t *Tgbot) broadcastCancelKeyboard() telego.ReplyMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(t.I18nBot("tgbot.buttons.cancel")).WithCallbackData("broadcast_cancel"),
		),
	)
}

// broadcastConfirmKeyboard returns the Confirm/Cancel inline keyboard for the
// broadcast preview message.
func (t *Tgbot) broadcastConfirmKeyboard() telego.ReplyMarkup {
	return tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(t.I18nBot("tgbot.buttons.confirmBroadcast")).WithCallbackData("broadcast_send"),
			tu.InlineKeyboardButton(t.I18nBot("tgbot.buttons.cancel")).WithCallbackData("broadcast_cancel"),
		),
	)
}

// broadcastSendOne delivers one broadcast message with the same retry-on-
// connection-error policy as SendMsgToTgbot, returning the final error so the
// summary can report failures (e.g. users who blocked the bot).
func (t *Tgbot) broadcastSendOne(chatId int64, escapedText string) error {
	params := telego.SendMessageParams{
		ChatID:    tu.ID(chatId),
		Text:      escapedText,
		ParseMode: "HTML",
	}
	var lastErr error
	for attempt := range 3 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := bot.SendMessage(ctx, &params)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		errStr := err.Error()
		isConnectionError := strings.Contains(errStr, "connection") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "closed")
		if isConnectionError && attempt < 2 {
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
			continue
		}
		break
	}
	return lastErr
}

// runBroadcast delivers the raw draft to every recipient and reports the
// summary back to the admin chat that started it. Delivery happens in small
// paced steps; each recipient is counted individually.
func (t *Tgbot) runBroadcast(adminChatId int64, draft string, recipients []int64) {
	defer finishBroadcast()

	escaped := html.EscapeString(draft)
	sent := 0
	failed := 0
	for i, chatId := range recipients {
		if err := t.broadcastSendOne(chatId, escaped); err != nil {
			failed++
			logger.Warningf("broadcast: delivery to %d failed: %v", chatId, err)
		} else {
			sent++
		}
		if i < len(recipients)-1 {
			time.Sleep(broadcastDelay)
		}
	}
	t.SendMsgToTgbot(adminChatId, t.I18nBot("tgbot.messages.broadcastDone",
		"Sent=="+strconv.Itoa(sent), "Failed=="+strconv.Itoa(failed)))
}
