package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
)

func getSettings() Settings {
	return Settings{
		ClubName:           getSetting("club_name", "Game Club"),
		Currency:           getSetting("currency", "KGS"),
		DefaultPricePerMin: getSettingFloat("default_price_per_minute", 1.0),
		MaxMinutes:         getSettingInt64("max_minutes", 0),
		BonusRate:          getSettingFloat("bonus_rate", 0.05),
		RequireShift:       getSettingBool("require_shift", true),
		TelegramEnabled:    getSettingBool("telegram_enabled", false),
		TelegramBotToken:   getSetting("telegram_bot_token", ""),
		TelegramChatID:     getSetting("telegram_chat_id", ""),
		NotifyLowMinutes:   getSettingInt64("notify_low_minutes", 5),
		DurationPresets:    getDurationPresets(),
	}
}

var defaultDurationPresets = []DurationPreset{
	{Label: "1 час", Minutes: 60},
	{Label: "1.5 часа", Minutes: 90},
	{Label: "2 часа", Minutes: 120},
	{Label: "3 часа", Minutes: 180},
}

func getDurationPresets() []DurationPreset {
	v := getSetting("duration_presets", "")
	if v == "" {
		return defaultDurationPresets
	}
	var list []DurationPreset
	if err := json.Unmarshal([]byte(v), &list); err != nil || len(list) == 0 {
		return defaultDurationPresets
	}
	return list
}

func getSetting(key, def string) string {
	var v string
	err := db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v)
	if err != nil || v == "" {
		return def
	}
	return v
}

func getSettingFloat(key string, def float64) float64 {
	v := getSetting(key, "")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func getSettingInt64(key string, def int64) int64 {
	v := getSetting(key, "")
	if v == "" {
		return def
	}
	i, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return i
}

func getSettingBool(key string, def bool) bool {
	v := getSetting(key, "")
	if v == "" {
		return def
	}
	return v == "1" || v == "true"
}

// setSetting — upsert, драйвер-агностичный.
func setSetting(key, value string) {
	res, err := db.Exec("UPDATE settings SET value = ? WHERE key = ?", value, key)
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			return
		}
	}
	db.Exec("INSERT INTO settings (key, value) VALUES (?,?)", key, value)
}

func seedSettings() {
	if getSetting("default_price_per_minute", "") == "" {
		if p, err := strconv.ParseFloat(os.Getenv("DEFAULT_PRICE"), 64); err == nil {
			setSetting("default_price_per_minute", strconv.FormatFloat(p, 'f', -1, 64))
		}
	}
	if getSetting("max_minutes", "") == "" {
		if m, err := strconv.Atoi(os.Getenv("MAX_MINUTES")); err == nil {
			setSetting("max_minutes", strconv.Itoa(m))
		}
	}
	if getSetting("club_name", "") == "" {
		setSetting("club_name", os.Getenv("CLUB_NAME"))
	}
	if getSetting("currency", "") == "" {
		setSetting("currency", os.Getenv("CURRENCY"))
	}
	if getSetting("require_shift", "") == "" {
		setSetting("require_shift", envBool("REQUIRE_SHIFT", true))
	}
	if getSetting("telegram_bot_token", "") == "" {
		setSetting("telegram_bot_token", os.Getenv("TELEGRAM_BOT_TOKEN"))
	}
	if getSetting("telegram_chat_id", "") == "" {
		setSetting("telegram_chat_id", os.Getenv("TELEGRAM_CHAT_ID"))
	}
	if getSetting("notify_low_minutes", "") == "" {
		if m, err := strconv.Atoi(os.Getenv("NOTIFY_LOW_MINUTES")); err == nil {
			setSetting("notify_low_minutes", strconv.Itoa(m))
		}
	}
	if getSetting("duration_presets", "") == "" {
		if b, err := json.Marshal(defaultDurationPresets); err == nil {
			setSetting("duration_presets", string(b))
		}
	}
}

func envBool(key string, def bool) string {
	v := os.Getenv(key)
	if v == "" {
		if def {
			return "1"
		}
		return "0"
	}
	if v == "1" || v == "true" || v == "yes" {
		return "1"
	}
	return "0"
}

func settingsGetHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, getSettings())
}

func settingsPutHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClubName           *string  `json:"club_name"`
		Currency           *string  `json:"currency"`
		DefaultPricePerMin *float64 `json:"default_price_per_minute"`
		MaxMinutes         *int64   `json:"max_minutes"`
		BonusRate          *float64 `json:"bonus_rate"`
		RequireShift       *bool    `json:"require_shift"`
		TelegramEnabled    *bool    `json:"telegram_enabled"`
		TelegramBotToken   *string  `json:"telegram_bot_token"`
		TelegramChatID     *string  `json:"telegram_chat_id"`
		NotifyLowMinutes   *int64   `json:"notify_low_minutes"`
		DurationPresets    *[]DurationPreset `json:"duration_presets"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.ClubName != nil {
		setSetting("club_name", *req.ClubName)
	}
	if req.Currency != nil {
		setSetting("currency", *req.Currency)
	}
	if req.DefaultPricePerMin != nil {
		setSetting("default_price_per_minute", strconv.FormatFloat(*req.DefaultPricePerMin, 'f', -1, 64))
	}
	if req.MaxMinutes != nil {
		setSetting("max_minutes", strconv.FormatInt(*req.MaxMinutes, 10))
	}
	if req.BonusRate != nil {
		setSetting("bonus_rate", strconv.FormatFloat(*req.BonusRate, 'f', -1, 64))
	}
	if req.RequireShift != nil {
		setSetting("require_shift", boolStr(*req.RequireShift))
	}
	if req.TelegramEnabled != nil {
		setSetting("telegram_enabled", boolStr(*req.TelegramEnabled))
	}
	if req.TelegramBotToken != nil {
		setSetting("telegram_bot_token", *req.TelegramBotToken)
	}
	if req.TelegramChatID != nil {
		setSetting("telegram_chat_id", *req.TelegramChatID)
	}
	if req.NotifyLowMinutes != nil {
		setSetting("notify_low_minutes", strconv.FormatInt(*req.NotifyLowMinutes, 10))
	}
	if req.DurationPresets != nil {
		if b, err := json.Marshal(*req.DurationPresets); err == nil {
			setSetting("duration_presets", string(b))
		}
	}
	logAction(r, "settings_update", "Настройки обновлены")
	writeJSON(w, 200, getSettings())
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}
