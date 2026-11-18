package main

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// dataDir возвращает каталог данных (рядом с БД).
func dataDir() string {
	path := os.Getenv("DB_PATH")
	if path == "" {
		path = "./data/readyboard.db"
	}
	return filepath.Dir(resolvePath(path))
}

func backgroundPath() string {
	return filepath.Join(dataDir(), "background")
}

// backgroundUploadHandler — загрузка фонового изображения (admin).
func backgroundUploadHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ImageBase64 string `json:"image_base64"`
		ContentType string `json:"content_type"`
	}
	if err := jsonDecode(r, &req); err != nil || req.ImageBase64 == "" {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.ImageBase64)
	if err != nil {
		writeErr(w, 400, "Некорректные данные изображения")
		return
	}
	if len(data) > 10*1024*1024 {
		writeErr(w, 400, "Изображение слишком большое (макс. 10 МБ)")
		return
	}
	if err := os.WriteFile(backgroundPath(), data, 0644); err != nil {
		writeErr(w, 500, "Ошибка сохранения")
		return
	}
	contentType := req.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}
	setSetting("background_content_type", contentType)
	setSetting("background_version", strconv.FormatInt(nowUnix(), 10))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// backgroundGetHandler — отдача фона (публично, для агентов и админки).
func backgroundGetHandler(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(backgroundPath())
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", getSetting("background_content_type", "image/jpeg"))
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}
