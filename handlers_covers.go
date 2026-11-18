package main

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// normalizeCoverName — единое имя обложки: нижний регистр, без пробелов по краям.
// Клиент ищет обложку по имени ярлыка, приведённому к такому же виду.
func normalizeCoverName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// coversUploadHandler — загрузка кастомной обложки (admin).
func coversUploadHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		ImageBase64 string `json:"image_base64"`
		ContentType string `json:"content_type"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	name := normalizeCoverName(req.Name)
	if name == "" {
		writeErr(w, 400, "Укажите имя обложки")
		return
	}
	data, err := base64.StdEncoding.DecodeString(req.ImageBase64)
	if err != nil || len(data) == 0 {
		writeErr(w, 400, "Некорректное изображение")
		return
	}
	if len(data) > 5*1024*1024 {
		writeErr(w, 400, "Изображение слишком большое (макс. 5 МБ)")
		return
	}
	contentType := req.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	if db.driver == "postgres" {
		db.Exec(`INSERT INTO covers (name, content_type, data, updated_at) VALUES (?,?,?,?)
			ON CONFLICT (name) DO UPDATE SET content_type = EXCLUDED.content_type, data = EXCLUDED.data, updated_at = EXCLUDED.updated_at`,
			name, contentType, data, nowUnix())
	} else {
		db.Exec(`INSERT INTO covers (name, content_type, data, updated_at) VALUES (?,?,?,?)
			ON CONFLICT(name) DO UPDATE SET content_type = excluded.content_type, data = excluded.data, updated_at = excluded.updated_at`,
			name, contentType, data, nowUnix())
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// coversListHandler — список имён обложек (auth).
func coversListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT name FROM covers ORDER BY name")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var n string
		rows.Scan(&n)
		names = append(names, n)
	}
	writeJSON(w, 200, map[string]interface{}{"covers": names})
}

// coversDeleteHandler — удаление обложки (admin).
func coversDeleteHandler(w http.ResponseWriter, r *http.Request) {
	name := normalizeCoverName(r.PathValue("name"))
	db.Exec("DELETE FROM covers WHERE name = ?", name)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// coversGetHandler — отдача обложки (публично, для агентов и админки).
func coversGetHandler(w http.ResponseWriter, r *http.Request) {
	name := normalizeCoverName(r.PathValue("name"))
	var data []byte
	var contentType string
	err := db.QueryRow("SELECT data, content_type FROM covers WHERE name = ?", name).Scan(&data, &contentType)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "max-age=3600")
	w.Write(data)
}
