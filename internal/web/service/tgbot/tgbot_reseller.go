package tgbot

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/util/common"
	"github.com/Dark-Sky07/phoenix/internal/web/service"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// resellerStartPrefix marks the deep-link payload of reseller link codes:
// https://t.me/<bot>?start=reseller_<code>
const resellerStartPrefix = "reseller_"

// BotUsername returns the Telegram username of the running bot (empty when
// the bot is stopped), so the panel can render reseller deep links.
func BotUsername() string {
	return botUsername()
}

// linkedReseller resolves the enabled reseller bound to a Telegram chat id,
// or nil when the chat is not a linked reseller.
func (t *Tgbot) linkedReseller(tgUserId int64) *model.Reseller {
	reseller, err := (&service.ResellerTelegramService{}).ResellerByTgID(tgUserId)
	if err != nil || reseller == nil {
		return nil
	}
	return reseller
}

// redeemResellerStart handles /start reseller_<code>: it binds the Telegram
// account to the reseller that issued the code and greets them with the
// reseller menu.
func (t *Tgbot) redeemResellerStart(chatId int64, from *telego.User, code string) {
	reseller, err := (&service.ResellerTelegramService{}).RedeemLinkCode(code, from.ID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrResellerTgAlreadyLinked):
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.resellerLinkAlreadyBound"))
		case errors.Is(err, service.ErrResellerTgLinkExpired):
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.resellerLinkExpired"))
		default:
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.resellerLinkInvalid"))
		}
		return
	}
	t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.resellerLinked",
		"Name=="+resellerDisplayName(reseller)))
	t.SendResellerAnswer(chatId, t.I18nBot("tgbot.commands.pleaseChoose"))
}

// resellerDisplayName picks the human-friendly label of a reseller.
func resellerDisplayName(reseller *model.Reseller) string {
	if reseller.Name != "" {
		return reseller.Name
	}
	return reseller.Username
}

// sendResellerUsage renders the linked reseller's live usage snapshot.
func (t *Tgbot) sendResellerUsage(chatId int64, reseller *model.Reseller) {
	stat, err := (&service.ResellerService{}).StatFor(reseller.Id)
	if err != nil || stat == nil {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.wentWrong"))
		return
	}
	trafficLimit := t.I18nBot("tgbot.unlimited")
	if reseller.TrafficLimit > 0 {
		trafficLimit = common.FormatTraffic(reseller.TrafficLimit)
	}
	clientLimit := t.I18nBot("tgbot.unlimited")
	if reseller.ClientLimit > 0 {
		clientLimit = strconv.Itoa(reseller.ClientLimit)
	}
	expiry := t.I18nBot("tgbot.unlimited")
	if reseller.ExpiryTime > 0 {
		expiry = time.UnixMilli(reseller.ExpiryTime).Format("2006-01-02")
	}
	pct := 0
	if reseller.TrafficLimit > 0 {
		pct = int(stat.AllocatedTraffic * 100 / reseller.TrafficLimit)
	}
	t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.resellerUsage",
		"Name=="+resellerDisplayName(reseller),
		"Allocated=="+common.FormatTraffic(stat.AllocatedTraffic),
		"TrafficLimit=="+trafficLimit,
		"Pct=="+strconv.Itoa(pct),
		"Used=="+common.FormatTraffic(stat.UsedTraffic),
		"Clients=="+strconv.Itoa(stat.ClientCount),
		"ClientLimit=="+clientLimit,
		"Online=="+strconv.Itoa(stat.OnlineCount),
		"Expiry=="+expiry))
}

// sendResellerNotifications lists the newest in-panel notifications of the
// linked reseller.
func (t *Tgbot) sendResellerNotifications(chatId int64, reseller *model.Reseller) {
	rows, _, err := (&service.ResellerNotifyService{}).List(reseller.Id, 5)
	if err != nil {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.wentWrong"))
		return
	}
	if len(rows) == 0 {
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.resellerNoNotifications"))
		return
	}
	for _, row := range rows {
		t.SendMsgToTgbot(chatId, t.FormatResellerNotification(reseller, row))
	}
}

// FormatResellerNotification renders one notification row in the bot's
// language. Exported so the notify job can push the same text it stores.
func (t *Tgbot) FormatResellerNotification(reseller *model.Reseller, row *model.ResellerNotification) string {
	if row == nil || reseller == nil {
		return ""
	}
	values := map[string]any{}
	if row.Params != "" {
		_ = json.Unmarshal([]byte(row.Params), &values)
	}
	param := func(key string) string {
		if v, ok := values[key]; ok {
			switch typed := v.(type) {
			case float64:
				return strconv.Itoa(int(typed))
			case string:
				return typed
			}
		}
		return ""
	}
	args := []string{"Name==" + resellerDisplayName(reseller)}
	key := ""
	switch row.Kind {
	case service.ResellerNotifQuota80:
		key = "tgbot.messages.resellerNotifQuota80"
		args = append(args, "Pct=="+param("pct"))
	case service.ResellerNotifQuota95:
		key = "tgbot.messages.resellerNotifQuota95"
		args = append(args, "Pct=="+param("pct"))
	case service.ResellerNotifQuotaOut:
		key = "tgbot.messages.resellerNotifQuotaOut"
	case service.ResellerNotifClient80:
		key = "tgbot.messages.resellerNotifClient80"
		args = append(args, "Pct=="+param("pct"))
	case service.ResellerNotifClientMax:
		key = "tgbot.messages.resellerNotifClientMax"
		args = append(args, "Count=="+param("count"), "Limit=="+param("limit"))
	case service.ResellerNotifExpirySoon:
		key = "tgbot.messages.resellerNotifExpirySoon"
		args = append(args, "Days=="+param("days"))
	case service.ResellerNotifExpired:
		key = "tgbot.messages.resellerNotifExpired"
	default:
		return ""
	}
	return t.I18nBot(key, args...)
}

// SendResellerAnswer sends a message with the reseller command keyboard.
func (t *Tgbot) SendResellerAnswer(chatId int64, msg string) {
	keyboard := tu.InlineKeyboard(
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(t.I18nBot("tgbot.buttons.resellerUsage")).WithCallbackData(t.encodeQuery("reseller_usage")),
			tu.InlineKeyboardButton(t.I18nBot("tgbot.buttons.resellerNotifications")).WithCallbackData(t.encodeQuery("reseller_notifications")),
		),
		tu.InlineKeyboardRow(
			tu.InlineKeyboardButton(t.I18nBot("tgbot.buttons.resellerCommands")).WithCallbackData(t.encodeQuery("reseller_commands")),
		),
	)
	t.SendMsgToTgbot(chatId, msg, keyboard)
}
