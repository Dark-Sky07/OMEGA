package tgbot

import (
	"errors"
	"strings"
	"time"

	"github.com/Dark-Sky07/phoenix/internal/logger"
	"github.com/Dark-Sky07/phoenix/internal/web/service"

	"github.com/mymmrac/telego"
)

// inviteStartPrefix marks the deep-link payload of invite links:
// https://t.me/<bot>?start=invite_<code>
const inviteStartPrefix = "invite_"

// buildInviteLink renders the shareable Telegram deep link for a code.
func (t *Tgbot) buildInviteLink(code string) string {
	return "https://t.me/" + bot.Username() + "?start=" + inviteStartPrefix + code
}

// sendClientInvite generates a fresh invite for one client and sends the
// shareable link to the admin chat that asked for it.
func (t *Tgbot) sendClientInvite(chatId int64, email string) {
	inviteSvc := service.ClientInviteService{}
	code, expiresAt, err := inviteSvc.CreateInvite(email)
	if err != nil {
		logger.Warning("invite: create failed:", err)
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.answers.errorOperation"))
		return
	}
	expires := time.UnixMilli(expiresAt).Format("2006-01-02 15:04")
	t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.inviteCreated",
		"Email=="+email, "Link=="+t.buildInviteLink(code), "Expires=="+expires))
}

// redeemInviteStart handles /start invite_<code>: it claims the code for the
// current Telegram user and binds their account (TgID) to the client the
// invite belongs to, so the user immediately gets self-service access.
func (t *Tgbot) redeemInviteStart(chatId int64, from *telego.User, code string) {
	inviteSvc := service.ClientInviteService{}
	email, err := inviteSvc.RedeemInvite(code, from.ID)
	if err != nil {
		logger.Info("invite: redeem failed:", err)
		switch {
		case errors.Is(err, service.ErrInviteUsed):
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.inviteUsed"))
		case errors.Is(err, service.ErrInviteExpired):
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.inviteExpired"))
		default:
			t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.inviteInvalid"))
		}
		return
	}

	traffic, err := t.inboundService.GetClientTrafficByEmail(email)
	if err != nil || traffic == nil {
		logger.Warning("invite: bound email has no traffic row:", email, err)
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.answers.errorOperation"))
		return
	}
	needRestart, err := t.clientService.SetClientTelegramUserID(&t.inboundService, traffic.Id, from.ID)
	if needRestart {
		t.xrayService.SetToNeedRestart()
	}
	if err != nil {
		logger.Warning("invite: bind failed:", err)
		t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.answers.errorOperation"))
		return
	}

	t.SendMsgToTgbot(chatId, t.I18nBot("tgbot.messages.inviteRedeemed",
		"Firstname=="+strings.TrimSpace(from.FirstName), "Email=="+email))
	t.SendAnswer(chatId, t.I18nBot("tgbot.commands.pleaseChoose"), false)
}
