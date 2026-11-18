package main

import (
	"database/sql"
	"net/http"
	"strings"
)

// --- Вход клиента с лаунчера (привязка к сессии) ---

// agentClientLoginHandler — клиент вводит логин (телефон/имя) и пароль.
func agentClientLoginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCID     int64  `json:"pc_id"`
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil || req.PCID <= 0 || strings.TrimSpace(req.Login) == "" {
		writeErr(w, 400, "Укажите ПК, логин и пароль")
		return
	}
	if !checkPCAgentAuth(r, req.PCID) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	login := strings.TrimSpace(req.Login)

	var c Client
	err := db.QueryRow(
		"SELECT id, name, phone, balance, bonus_points, discount_percent, notes, password_hash, created_at FROM clients WHERE phone = ? OR LOWER(name) = LOWER(?)",
		login, login,
	).Scan(&c.ID, &c.Name, &c.Phone, &c.Balance, &c.BonusPoints, &c.DiscountPercent, &c.Notes, &c.PasswordHash, &c.CreatedAt)
	if err == sql.ErrNoRows {
		writeErr(w, 404, "Клиент не найден")
		return
	}
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	if c.PasswordHash == "" {
		writeErr(w, 403, "У клиента не задан пароль. Обратитесь к администратору.")
		return
	}
	if !checkPassword(c.PasswordHash, req.Password) {
		writeErr(w, 401, "Неверный пароль")
		return
	}

	// привязываем клиента к активной сессии на этом ПК
	db.Exec("UPDATE sessions SET client_id = ? WHERE pc_id = ? AND status IN ('active','paused')", c.ID, req.PCID)

	// автозапуск почасовой сессии с баланса (если ПК свободен и есть баланс)
	started := startSelfSession(req.PCID, c)

	// сообщение, если вход без запуска сессии (недостаточно баланса)
	message := ""
	if !started {
		var curSess int64
		db.QueryRow("SELECT current_session_id FROM pcs WHERE id = ?", req.PCID).Scan(&curSess)
		if curSess == 0 {
			ppm := effectivePpmForClient(c, req.PCID)
			if c.Balance <= 0 {
				message = "Баланс пуст. Пополните баланс у администратора."
			} else if ppm > 0 && c.Balance < ppm*5 {
				message = "Недостаточно баланса: нужно минимум на 5 минут (" + moneyStr(round2(ppm*5)) + " сом)."
			}
		}
	}
	logAction(r, "client_login", "Клиент «"+c.Name+"» вошёл на ПК #"+itoa(req.PCID))
	writeJSON(w, 200, map[string]interface{}{"client": c, "session_started": started, "message": message})
}

// effectivePpmForClient — цена за минуту почасового тарифа с учётом скидки клиента.
func effectivePpmForClient(c Client, pcID int64) float64 {
	tariff := resolveTariff(0, pcID)
	if tariff.Kind != TariffHourly {
		s := getSettings()
		tariff = Tariff{ID: 0, Name: "Стандарт", Kind: TariffHourly, PricePerMinute: s.DefaultPricePerMin, Active: true}
	}
	ppm := tariffPpm(tariff)
	if ppm <= 0 {
		ppm = getSettings().DefaultPricePerMin
	}
	if c.DiscountPercent > 0 {
		ppm = ppm * (1 - c.DiscountPercent/100)
	}
	return ppm
}

// startSelfSession — автозапуск открытой сессии (пост-оплата) при входе.
// Баланс НЕ списывается сразу — снимается по завершении по фактическому времени.
// Возвращает true, если сессия была запущена.
func startSelfSession(pcID int64, c Client) bool {
	var curSess int64
	db.QueryRow("SELECT current_session_id FROM pcs WHERE id = ?", pcID).Scan(&curSess)
	if curSess > 0 || c.Balance <= 0 {
		return false
	}

	ppm := effectivePpmForClient(c, pcID)
	if ppm <= 0 {
		return false
	}

	// минимальный вход — на 5 минут
	if c.Balance < ppm*5 {
		return false
	}

	// мягкий лимит: на сколько времени хватит баланса (по завершении спишется по факту)
	maxSeconds := int64(c.Balance / ppm * 60)
	if maxMin := getSettings().MaxMinutes; maxMin > 0 && maxSeconds > maxMin*60 {
		maxSeconds = maxMin * 60
	}
	if maxSeconds < 5*60 {
		return false
	}

	tariff := resolveTariff(0, pcID)
	if tariff.Kind != TariffHourly {
		tariff = Tariff{ID: 0, Name: "Стандарт", Kind: TariffHourly}
	}

	if _, err := createOpenSession(pcID, c.ID, tariff.ID, maxSeconds, "Открытый тариф (оплата по завершении)"); err != nil {
		return false
	}
	return true
}

// agentClientRegisterHandler — саморегистрация «стать клиентом».
func agentClientRegisterHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCID     int64  `json:"pc_id"`
		Name     string `json:"name"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil || req.PCID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if !checkPCAgentAuth(r, req.PCID) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	name := strings.TrimSpace(req.Name)
	phone := strings.TrimSpace(req.Phone)
	if name == "" {
		writeErr(w, 400, "Укажите имя")
		return
	}
	if req.Password == "" {
		writeErr(w, 400, "Укажите пароль")
		return
	}
	// проверка уникальности (по имени или телефону)
	var exists int
	if phone != "" {
		db.QueryRow("SELECT COUNT(*) FROM clients WHERE phone = ?", phone).Scan(&exists)
	}
	if exists == 0 {
		db.QueryRow("SELECT COUNT(*) FROM clients WHERE LOWER(name) = LOWER(?)", name).Scan(&exists)
	}
	if exists > 0 {
		writeErr(w, 409, "Такой клиент уже существует. Войдите в аккаунт.")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeErr(w, 500, "Ошибка хеширования")
		return
	}
	id, err := insertID(
		"INSERT INTO clients (name, phone, balance, bonus_points, discount_percent, notes, password_hash, created_at) VALUES (?,?,?,?,?,?,?,?)",
		name, phone, 0, 0, 0, "", hash, nowUnix(),
	)
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	// привязываем к сессии
	db.Exec("UPDATE sessions SET client_id = ? WHERE pc_id = ? AND status IN ('active','paused')", id, req.PCID)
	logAction(r, "client_register", "Клиент «"+name+"» зарегистрировался с ПК #"+itoa(req.PCID))

	var c Client
	db.QueryRow("SELECT id, name, phone, balance, bonus_points, discount_percent, notes, password_hash, created_at FROM clients WHERE id = ?", id).
		Scan(&c.ID, &c.Name, &c.Phone, &c.Balance, &c.BonusPoints, &c.DiscountPercent, &c.Notes, &c.PasswordHash, &c.CreatedAt)
	writeJSON(w, 200, map[string]interface{}{"client": c})
}

// agentClientLogoutHandler — выход: останавливает сессию (с возвратом остатка) и отвязывает клиента.
func agentClientLogoutHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCID int64 `json:"pc_id"`
	}
	if err := jsonDecode(r, &req); err != nil || req.PCID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if !checkPCAgentAuth(r, req.PCID) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	// завершаем активную сессию (для открытого тарифа — списание по факту)
	var sessID int64
	db.QueryRow("SELECT current_session_id FROM pcs WHERE id = ?", req.PCID).Scan(&sessID)
	var charged, refund float64
	var stopped bool
	if sessID > 0 {
		_, charged, refund, _, stopped = stopSession(sessID, 0)
	}
	logAction(r, "client_logout", "Клиент вышел с ПК #"+itoa(req.PCID))
	writeJSON(w, 200, map[string]interface{}{"ok": true, "charged": charged, "refund": refund, "stopped": stopped})
}

// --- Вызов администратора ---

// agentCallHandler — клиент нажимает кнопку вызова/заказа.
func agentCallHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCID   int64  `json:"pc_id"`
		Reason string `json:"reason"`
	}
	if err := jsonDecode(r, &req); err != nil || req.PCID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if !checkPCAgentAuth(r, req.PCID) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		req.Reason = "Вызов администратора"
	}
	var pcName, clientName string
	db.QueryRow("SELECT name FROM pcs WHERE id = ?", req.PCID).Scan(&pcName)
	db.QueryRow(`
		SELECT COALESCE(c.name,'')
		FROM pcs p
		LEFT JOIN sessions s ON s.id = p.current_session_id AND s.status IN ('active','paused')
		LEFT JOIN clients c ON c.id = s.client_id
		WHERE p.id = ?`, req.PCID).Scan(&clientName)

	id, err := insertID("INSERT INTO calls (pc_id, pc_name, client_name, reason, status, created_at) VALUES (?,?,?,?,?,?)",
		req.PCID, pcName, clientName, req.Reason, "open", nowUnix())
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

// callsListHandler — список открытых вызовов (для админки).
func callsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, pc_id, pc_name, client_name, reason, status, created_at FROM calls WHERE status = 'open' ORDER BY id")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []Call{}
	for rows.Next() {
		var c Call
		rows.Scan(&c.ID, &c.PCID, &c.PCName, &c.ClientName, &c.Reason, &c.Status, &c.CreatedAt)
		list = append(list, c)
	}
	writeJSON(w, 200, map[string]interface{}{"calls": list})
}

// callResolveHandler — закрыть вызов (для админки).
func callResolveHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	db.Exec("UPDATE calls SET status = 'resolved' WHERE id = ?", id)
	logAction(r, "call_resolve", "Вызов #"+itoa(id)+" обработан")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- Товары для лаунчера (меню «поесть/попить») ---

// agentProductsHandler — список активных товаров (auth по токену агента ПК).
func agentProductsHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	if !checkPCAgentAuth(r, id) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	rows, err := db.Query("SELECT id, name, category, price, stock, active FROM products WHERE active = 1 ORDER BY name")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []Product{}
	for rows.Next() {
		var p Product
		var active int
		rows.Scan(&p.ID, &p.Name, &p.Category, &p.Price, &p.Stock, &active)
		p.Active = active == 1
		list = append(list, p)
	}
	writeJSON(w, 200, map[string]interface{}{"products": list})
}

// agentBuyHandler — покупка товара клиентом с баланса (лаунчер).
func agentBuyHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCID      int64 `json:"pc_id"`
		ProductID int64 `json:"product_id"`
		Qty       int64 `json:"qty"`
	}
	if err := jsonDecode(r, &req); err != nil || req.PCID <= 0 || req.ProductID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if !checkPCAgentAuth(r, req.PCID) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}

	// клиент активной сессии (гость не может покупать с баланса)
	var clientID int64
	db.QueryRow(`
		SELECT COALESCE(s.client_id,0)
		FROM pcs p
		LEFT JOIN sessions s ON s.id = p.current_session_id AND s.status IN ('active','paused')
		WHERE p.id = ?`, req.PCID).Scan(&clientID)
	if clientID <= 0 {
		writeErr(w, 400, "Гость не может покупать с баланса — оплата через кассу")
		return
	}

	var p Product
	if err := db.QueryRow("SELECT id, name, category, price, stock, active FROM products WHERE id = ?", req.ProductID).
		Scan(&p.ID, &p.Name, &p.Category, &p.Price, &p.Stock, &p.Active); err != nil {
		writeErr(w, 404, "Товар не найден")
		return
	}
	if p.Stock < req.Qty {
		writeErr(w, 400, "Недостаточно на складе (осталось "+itoa(p.Stock)+")")
		return
	}

	amount := round2(p.Price * float64(req.Qty))
	var bal float64
	db.QueryRow("SELECT balance FROM clients WHERE id = ?", clientID).Scan(&bal)
	if bal < amount {
		writeErr(w, 400, "Недостаточно баланса (нужно "+moneyStr(amount)+", есть "+moneyStr(bal)+")")
		return
	}

	db.Exec("UPDATE clients SET balance = balance - ? WHERE id = ?", amount, clientID)
	db.Exec("UPDATE products SET stock = stock - ? WHERE id = ?", req.Qty, req.ProductID)
	insertID("INSERT INTO sales (product_id, product_name, qty, unit_price, amount, user_id, shift_id, method, created_at) VALUES (?,?,?,?,?,?,?,?,?)",
		p.ID, p.Name, req.Qty, p.Price, amount, 0, currentShiftID(), "balance", nowUnix())
	logAction(r, "product_buy", "Покупка «"+p.Name+"» ×"+itoa(req.Qty)+" с баланса = "+moneyStr(amount))
	writeJSON(w, 200, map[string]interface{}{"amount": amount, "product": p.Name, "balance": round2(bal - amount)})
}
