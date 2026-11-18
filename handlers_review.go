package main

import "net/http"

// reviewsListHandler — список отзывов клиентов (auth).
func reviewsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, client_name, rating, comment, created_at FROM reviews ORDER BY id DESC LIMIT 200")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []Review{}
	for rows.Next() {
		var rv Review
		rows.Scan(&rv.ID, &rv.ClientName, &rv.Rating, &rv.Comment, &rv.CreatedAt)
		list = append(list, rv)
	}
	writeJSON(w, 200, map[string]interface{}{"reviews": list})
}

// reviewCreateHandler — добавление отзыва (auth).
func reviewCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClientName string `json:"client_name"`
		Rating     int64  `json:"rating"`
		Comment    string `json:"comment"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.ClientName == "" {
		req.ClientName = "Гость"
	}
	if req.Rating < 1 || req.Rating > 5 {
		req.Rating = 5
	}
	id, err := insertID("INSERT INTO reviews (client_name, rating, comment, created_at) VALUES (?,?,?,?)",
		req.ClientName, req.Rating, req.Comment, nowUnix())
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	logAction(r, "review_create", "Отзыв от «"+req.ClientName+"» (оценка "+itoa(req.Rating)+"/5)")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

// reviewDeleteHandler — удаление отзыва (admin+).
func reviewDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	db.Exec("DELETE FROM reviews WHERE id = ?", id)
	logAction(r, "review_delete", "Отзыв #"+itoa(id)+" удалён")
	writeJSON(w, 200, map[string]bool{"ok": true})
}
