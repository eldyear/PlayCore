package main

import (
	"net/http"
	"strconv"
)

// bookingsListHandler — список броней (фильтр по диапазону дат from/to).
func bookingsListHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, _ := strconv.ParseInt(q.Get("from"), 10, 64)
	to, _ := strconv.ParseInt(q.Get("to"), 10, 64)

	where := "1=1"
	args := []interface{}{}
	if from > 0 {
		where += " AND b.end_at > ?"
		args = append(args, from)
	}
	if to > 0 {
		where += " AND b.start_at < ?"
		args = append(args, to)
	}

	rows, err := db.Query(`
		SELECT b.id, b.pc_id, COALESCE(p.name,''), b.client, b.phone, b.start_at, b.end_at, b.kind, b.status, b.created_at
		FROM bookings b
		LEFT JOIN pcs p ON p.id = b.pc_id
		WHERE `+where+`
		ORDER BY b.start_at ASC`, args...)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()

	list := []Booking{}
	for rows.Next() {
		var b Booking
		rows.Scan(&b.ID, &b.PCID, &b.PCName, &b.Client, &b.Phone, &b.StartAt, &b.EndAt, &b.Kind, &b.Status, &b.CreatedAt)
		list = append(list, b)
	}
	writeJSON(w, 200, map[string]interface{}{"bookings": list})
}

// bookingCreateHandler — создание брони.
func bookingCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCID    int64  `json:"pc_id"`
		Client  string `json:"client"`
		Phone   string `json:"phone"`
		StartAt int64  `json:"start_at"`
		EndAt   int64  `json:"end_at"`
		Kind    string `json:"kind"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.PCID <= 0 || req.StartAt <= 0 || req.EndAt <= req.StartAt {
		writeErr(w, 400, "Укажите ПК и корректное время")
		return
	}
	if req.Kind == "" {
		req.Kind = "blue"
	}
	id, err := insertID("INSERT INTO bookings (pc_id, client, phone, start_at, end_at, kind, status, created_at) VALUES (?,?,?,?,?,?,?,?)",
		req.PCID, req.Client, req.Phone, req.StartAt, req.EndAt, req.Kind, "active", nowUnix())
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	logAction(r, "booking_create", "Бронь «"+req.Client+"» на ПК #"+itoa(req.PCID))
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

// bookingUpdateHandler — изменение брони.
func bookingUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		PCID    int64  `json:"pc_id"`
		Client  string `json:"client"`
		Phone   string `json:"phone"`
		StartAt int64  `json:"start_at"`
		EndAt   int64  `json:"end_at"`
		Kind    string `json:"kind"`
		Status  string `json:"status"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	db.Exec("UPDATE bookings SET pc_id=?, client=?, phone=?, start_at=?, end_at=?, kind=?, status=? WHERE id=?",
		req.PCID, req.Client, req.Phone, req.StartAt, req.EndAt, req.Kind, req.Status, id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// bookingDeleteHandler — удаление брони.
func bookingDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	db.Exec("DELETE FROM bookings WHERE id = ?", id)
	logAction(r, "booking_delete", "Бронь #"+itoa(id)+" удалена")
	writeJSON(w, 200, map[string]bool{"ok": true})
}
