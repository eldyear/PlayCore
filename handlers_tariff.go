package main

import "net/http"

func tariffsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, name, kind, price_per_minute, price_per_hour, duration_minutes, package_price, description, active FROM tariffs ORDER BY id")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []Tariff{}
	for rows.Next() {
		var t Tariff
		var active int
		rows.Scan(&t.ID, &t.Name, &t.Kind, &t.PricePerMinute, &t.PricePerHour, &t.DurationMinutes, &t.PackagePrice, &t.Description, &active)
		t.Active = active == 1
		list = append(list, t)
	}
	writeJSON(w, 200, map[string]interface{}{"tariffs": list})
}

func tariffCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string  `json:"name"`
		Kind            string  `json:"kind"`
		PricePerMinute  float64 `json:"price_per_minute"`
		PricePerHour    float64 `json:"price_per_hour"`
		DurationMinutes int64   `json:"duration_minutes"`
		PackagePrice    float64 `json:"package_price"`
		Description     string  `json:"description"`
		Active          *bool   `json:"active"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Name == "" {
		writeErr(w, 400, "Укажите название тарифа")
		return
	}
	if req.Kind == "" {
		req.Kind = TariffHourly
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	id, err := insertID(
		"INSERT INTO tariffs (name, kind, price_per_minute, price_per_hour, duration_minutes, package_price, description, active) VALUES (?,?,?,?,?,?,?,?)",
		req.Name, req.Kind, req.PricePerMinute, req.PricePerHour, req.DurationMinutes, req.PackagePrice, req.Description, boolToInt(active),
	)
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	logAction(r, "tariff_create", "Тариф «"+req.Name+"» создан")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

func tariffUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req Tariff
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	db.Exec(
		"UPDATE tariffs SET name=?, kind=?, price_per_minute=?, price_per_hour=?, duration_minutes=?, package_price=?, description=?, active=? WHERE id=?",
		req.Name, req.Kind, req.PricePerMinute, req.PricePerHour, req.DurationMinutes, req.PackagePrice, req.Description, boolToInt(req.Active), id,
	)
	logAction(r, "tariff_update", "Тариф «"+req.Name+"» обновлён")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func tariffDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	db.Exec("DELETE FROM tariffs WHERE id = ?", id)
	logAction(r, "tariff_delete", "Тариф #"+itoa(id)+" удалён")
	writeJSON(w, 200, map[string]bool{"ok": true})
}
