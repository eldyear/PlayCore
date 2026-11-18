package main

import (
	"net/http"
	"strconv"
)

// --- Список товаров ---

func productsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name, category, price, stock, active FROM products ORDER BY id")
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

func productCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string  `json:"name"`
		Category string  `json:"category"`
		Price    float64 `json:"price"`
		Stock    int64   `json:"stock"`
		Active   *bool   `json:"active"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Name == "" {
		writeErr(w, 400, "Укажите название товара")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	id, err := insertID(
		"INSERT INTO products (name, category, price, stock, active) VALUES (?,?,?,?,?)",
		req.Name, req.Category, req.Price, req.Stock, boolToInt(active),
	)
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	logAction(r, "product_create", "Товар «"+req.Name+"» создан")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

func productUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req Product
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	db.Exec(
		"UPDATE products SET name=?, category=?, price=?, active=? WHERE id=?",
		req.Name, req.Category, req.Price, boolToInt(req.Active), id,
	)
	logAction(r, "product_update", "Товар «"+req.Name+"» обновлён")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func productDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	db.Exec("DELETE FROM products WHERE id = ?", id)
	logAction(r, "product_delete", "Товар #"+itoa(id)+" удалён")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- Продажа товара ---

func productSellHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Qty    int64  `json:"qty"`
		Method string `json:"method"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Qty <= 0 {
		req.Qty = 1
	}
	if req.Method == "" {
		req.Method = "cash"
	}
	if !ensureOpenShift(w, req.Method) {
		return
	}

	var p Product
	var active int
	err := db.QueryRow("SELECT id, name, price, stock, active FROM products WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Price, &p.Stock, &active)
	if err != nil {
		writeErr(w, 404, "Товар не найден")
		return
	}
	if p.Stock < req.Qty {
		writeErr(w, 400, "Недостаточно на складе (осталось "+itoa(p.Stock)+")")
		return
	}

	amount := round2(p.Price * float64(req.Qty))
	db.Exec("UPDATE products SET stock = stock - ? WHERE id = ?", req.Qty, id)
	insertID(
		"INSERT INTO sales (product_id, product_name, qty, unit_price, amount, user_id, shift_id, method, created_at) VALUES (?,?,?,?,?,?,?,?,?)",
		id, p.Name, req.Qty, p.Price, amount, currentUserID(r), currentShiftID(), req.Method, nowUnix(),
	)
	logAction(r, "product_sell", "Продажа «"+p.Name+"» ×"+itoa(req.Qty)+" = "+moneyStr(amount))
	writeJSON(w, 200, map[string]interface{}{"amount": amount, "product": p.Name, "qty": req.Qty})
}

// --- Приход товара на склад ---

func productRestockHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Qty int64 `json:"qty"`
	}
	if err := jsonDecode(r, &req); err != nil || req.Qty <= 0 {
		writeErr(w, 400, "Укажите количество")
		return
	}
	db.Exec("UPDATE products SET stock = stock + ? WHERE id = ?", req.Qty, id)
	logAction(r, "product_restock", "Приход товара #"+itoa(id)+" +"+itoa(req.Qty))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- Список продаж ---

func salesListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT s.id, s.product_id, s.product_name, s.qty, s.unit_price, s.amount,
		       s.user_id, COALESCE(u.username,''), s.shift_id, s.method, s.created_at
		FROM sales s LEFT JOIN users u ON u.id = s.user_id
		ORDER BY s.id DESC LIMIT 500`)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []Sale{}
	for rows.Next() {
		var s Sale
		rows.Scan(&s.ID, &s.ProductID, &s.ProductName, &s.Qty, &s.UnitPrice, &s.Amount,
			&s.UserID, &s.UserName, &s.ShiftID, &s.Method, &s.CreatedAt)
		list = append(list, s)
	}
	writeJSON(w, 200, map[string]interface{}{"sales": list})
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
