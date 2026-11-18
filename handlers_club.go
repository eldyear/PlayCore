package main

import "net/http"

// clubsListHandler — список клубов с их последней подпиской (root).
func clubsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT c.id, c.name, c.created_at, COALESCE(s.plan,''), COALESCE(s.status,''), COALESCE(s.expires_at,0)
		FROM clubs c
		LEFT JOIN subscriptions s ON s.id = (SELECT id FROM subscriptions WHERE club_id = c.id ORDER BY id DESC LIMIT 1)
		ORDER BY c.id`)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()

	type clubItem struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		CreatedAt int64  `json:"created_at"`
		Plan      string `json:"plan"`
		Status    string `json:"status"`
		ExpiresAt int64  `json:"expires_at"`
	}
	list := []clubItem{}
	for rows.Next() {
		var c clubItem
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt, &c.Plan, &c.Status, &c.ExpiresAt); err != nil {
			continue
		}
		list = append(list, c)
	}
	writeJSON(w, 200, map[string]interface{}{"clubs": list})
}

// clubCreateHandler — создание клуба (root).
func clubCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := jsonDecode(r, &req); err != nil || req.Name == "" {
		writeErr(w, 400, "Укажите название клуба")
		return
	}
	id, err := insertID("INSERT INTO clubs (name, created_at) VALUES (?,?)", req.Name, nowUnix())
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	db.Exec("INSERT INTO subscriptions (club_id, plan, status, expires_at, created_at) VALUES (?,?,?,?,?)",
		id, "free", "trial", 0, nowUnix())
	logAction(r, "club_create", "Создан клуб «"+req.Name+"»")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

// subscriptionUpdateHandler — установка подписки клуба (root).
func subscriptionUpdateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ClubID    int64  `json:"club_id"`
		Plan      string `json:"plan"`
		Status    string `json:"status"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := jsonDecode(r, &req); err != nil || req.ClubID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Plan == "" {
		req.Plan = "free"
	}
	if req.Status == "" {
		req.Status = "active"
	}
	id, err := insertID("INSERT INTO subscriptions (club_id, plan, status, expires_at, created_at) VALUES (?,?,?,?,?)",
		req.ClubID, req.Plan, req.Status, req.ExpiresAt, nowUnix())
	if err != nil {
		writeErr(w, 500, "Ошибка сохранения")
		return
	}
	logAction(r, "subscription_update", "Обновлена подписка клуба #"+itoa(req.ClubID)+" («"+req.Plan+"», "+req.Status+")")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}
