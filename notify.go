package main

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// notifyTelegram отправляет сообщение в Telegram (если включено и настроено).
func notifyTelegram(text string) {
	s := getSettings()
	if !s.TelegramEnabled || s.TelegramBotToken == "" || s.TelegramChatID == "" {
		return
	}
	// Отправляем в фоне, чтобы не блокировать основной поток.
	go func(token, chatID, msg string) {
		client := http.Client{Timeout: 5 * time.Second}
		form := url.Values{}
		form.Set("chat_id", chatID)
		form.Set("text", msg)
		client.PostForm("https://api.telegram.org/bot"+token+"/sendMessage", form)
	}(s.TelegramBotToken, s.TelegramChatID, text)
}

// tickNotifications — оповещения о низком остатке времени.
func tickNotifications() {
	s := getSettings()
	if !s.TelegramEnabled || s.TelegramBotToken == "" {
		return
	}
	threshold := s.NotifyLowMinutes * 60
	if threshold <= 0 {
		threshold = 300
	}
	now := nowUnix()

	rows, err := db.Query(`
		SELECT s.id, p.name, s.ends_at - ?
		FROM sessions s
		JOIN pcs p ON p.id = s.pc_id
		WHERE s.status = 'active' AND s.low_notified = 0 AND s.ends_at - ? <= ?`,
		now, now, threshold)
	if err != nil {
		return
	}

	type low struct {
		id       int64
		pcName   string
		remaining int64
	}
	var list []low
	for rows.Next() {
		var l low
		rows.Scan(&l.id, &l.pcName, &l.remaining)
		if l.remaining < 0 {
			l.remaining = 0
		}
		list = append(list, l)
	}
	rows.Close()

	for _, l := range list {
		notifyTelegram("⏰ " + l.pcName + ": осталось " + formatDuration(l.remaining) + " времени.")
		db.Exec("UPDATE sessions SET low_notified = 1 WHERE id = ?", l.id)
	}
}

// testTelegramHandler — отправка тестового сообщения (проверка настроек).
func telegramTestHandler(w http.ResponseWriter, r *http.Request) {
	notifyTelegram("✅ Game Club Control: тестовое уведомление подключено.")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// sanitizeTelegram экранирует нежелательные символы в разметке (не используется сейчас).
func sanitizeTelegram(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}
