package main

import "net/http"

// announcementsListHandler — список объявлений (auth).
func announcementsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, title, text, active, created_at FROM announcements ORDER BY id DESC")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []Announcement{}
	for rows.Next() {
		var a Announcement
		var active int
		rows.Scan(&a.ID, &a.Title, &a.Text, &active, &a.CreatedAt)
		a.Active = active == 1
		list = append(list, a)
	}
	writeJSON(w, 200, map[string]interface{}{"announcements": list})
}

// announcementCreateHandler — создание объявления (admin+).
func announcementCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title  string `json:"title"`
		Text   string `json:"text"`
		Active *bool  `json:"active"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Title == "" {
		req.Title = "Объявление"
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	id, err := insertID("INSERT INTO announcements (title, text, active, created_at) VALUES (?,?,?,?)",
		req.Title, req.Text, boolToInt(active), nowUnix())
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	logAction(r, "announcement_create", "Объявление «"+req.Title+"» создано")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

// announcementUpdateHandler — обновление объявления (admin+).
func announcementUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Title  *string `json:"title"`
		Text   *string `json:"text"`
		Active *bool   `json:"active"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Title != nil {
		db.Exec("UPDATE announcements SET title = ? WHERE id = ?", *req.Title, id)
	}
	if req.Text != nil {
		db.Exec("UPDATE announcements SET text = ? WHERE id = ?", *req.Text, id)
	}
	if req.Active != nil {
		db.Exec("UPDATE announcements SET active = ? WHERE id = ?", boolToInt(*req.Active), id)
	}
	logAction(r, "announcement_update", "Объявление #"+itoa(id)+" обновлено")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// announcementDeleteHandler — удаление объявления (admin+).
func announcementDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	db.Exec("DELETE FROM announcements WHERE id = ?", id)
	logAction(r, "announcement_delete", "Объявление #"+itoa(id)+" удалено")
	writeJSON(w, 200, map[string]bool{"ok": true})
}
