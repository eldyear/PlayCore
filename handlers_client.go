package main

import "net/http"

func clientsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name, phone, balance, bonus_points, discount_percent, notes, password_hash, created_at FROM clients ORDER BY name")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []Client{}
	for rows.Next() {
		var c Client
		rows.Scan(&c.ID, &c.Name, &c.Phone, &c.Balance, &c.BonusPoints, &c.DiscountPercent, &c.Notes, &c.PasswordHash, &c.CreatedAt)
		c.HasPassword = c.PasswordHash != ""
		list = append(list, c)
	}
	writeJSON(w, 200, map[string]interface{}{"clients": list})
}

func clientCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string  `json:"name"`
		Phone           string  `json:"phone"`
		Balance         float64 `json:"balance"`
		BonusPoints     int64   `json:"bonus_points"`
		DiscountPercent float64 `json:"discount_percent"`
		Notes           string  `json:"notes"`
		Password        string  `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Name == "" {
		writeErr(w, 400, "Укажите имя клиента")
		return
	}
	pwdHash := ""
	if req.Password != "" {
		h, err := hashPassword(req.Password)
		if err != nil {
			writeErr(w, 500, "Ошибка хеширования")
			return
		}
		pwdHash = h
	}
	id, err := insertID(
		"INSERT INTO clients (name, phone, balance, bonus_points, discount_percent, notes, password_hash, created_at) VALUES (?,?,?,?,?,?,?,?)",
		req.Name, req.Phone, req.Balance, req.BonusPoints, req.DiscountPercent, req.Notes, pwdHash, nowUnix(),
	)
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	logAction(r, "client_create", "Клиент «"+req.Name+"» создан")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

func clientUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Name            string  `json:"name"`
		Phone           string  `json:"phone"`
		DiscountPercent float64 `json:"discount_percent"`
		Notes           string  `json:"notes"`
		Password        string  `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	db.Exec(
		"UPDATE clients SET name=?, phone=?, discount_percent=?, notes=? WHERE id=?",
		req.Name, req.Phone, req.DiscountPercent, req.Notes, id,
	)
	if req.Password != "" {
		h, _ := hashPassword(req.Password)
		db.Exec("UPDATE clients SET password_hash = ? WHERE id = ?", h, id)
	}
	logAction(r, "client_update", "Клиент «"+req.Name+"» обновлён")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func clientDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	db.Exec("DELETE FROM clients WHERE id = ?", id)
	logAction(r, "client_delete", "Клиент #"+itoa(id)+" удалён")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Пополнение баланса клиента (депозит)
func clientDepositHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Amount float64 `json:"amount"`
		Method string  `json:"method"`
		Note   string  `json:"note"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Amount <= 0 {
		writeErr(w, 400, "Укажите сумму")
		return
	}
	if req.Method == "" {
		req.Method = "cash"
	}
	if !ensureOpenShift(w, req.Method) {
		return
	}
	var name string
	if err := db.QueryRow("SELECT name FROM clients WHERE id = ?", id).Scan(&name); err != nil {
		writeErr(w, 404, "Клиент не найден")
		return
	}
	db.Exec("UPDATE clients SET balance = balance + ? WHERE id = ?", req.Amount, id)
	recordTransaction(TxClientDeposit, 0, id, currentUserID(r), currentShiftID(), 0, req.Amount, 0, req.Method, req.Note)
	logAction(r, "client_deposit", "Пополнение клиента #"+itoa(id)+" +"+moneyStr(req.Amount))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// Списание бонусов (обмен на время)
func clientUseBonusHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Points int64 `json:"points"`
	}
	if err := jsonDecode(r, &req); err != nil || req.Points <= 0 {
		writeErr(w, 400, "Укажите количество бонусов")
		return
	}
	var pts int64
	db.QueryRow("SELECT bonus_points FROM clients WHERE id = ?", id).Scan(&pts)
	if pts < req.Points {
		writeErr(w, 400, "Недостаточно бонусов")
		return
	}
	db.Exec("UPDATE clients SET bonus_points = bonus_points - ? WHERE id = ?", req.Points, id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
