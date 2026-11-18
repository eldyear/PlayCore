package main

import (
	"net/http"
	"strconv"
)

// Итоги по смене: услуги (время) + товары.
func timeSalesTotal(shiftID int64) float64 {
	var t float64
	db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM transactions
		WHERE shift_id = ? AND kind IN ('topup','package','balance_pay','client_deposit')`, shiftID).Scan(&t)
	return t
}

func goodsSalesTotal(shiftID int64) float64 {
	var t float64
	db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM sales WHERE shift_id = ?`, shiftID).Scan(&t)
	return t
}

func shiftOpenHandler(w http.ResponseWriter, r *http.Request) {
	if id := currentShiftID(); id > 0 {
		writeErr(w, 400, "Смена уже открыта")
		return
	}
	var req struct {
		OpeningBalance float64 `json:"opening_balance"`
	}
	jsonDecode(r, &req)

	id, err := insertID("INSERT INTO shifts (user_id, opened_at, opening_balance, status) VALUES (?,?,?,?)",
		currentUserID(r), nowUnix(), req.OpeningBalance, ShiftOpen)
	if err != nil {
		writeErr(w, 500, "Ошибка открытия смены")
		return
	}
	recordTransaction(TxOpening, 0, 0, currentUserID(r), id, 0, req.OpeningBalance, 0, "cash", "Открытие смены")
	notifyTelegram("🔓 Смена открыта. Начальный остаток: " + moneyStr(req.OpeningBalance))
	logAction(r, "shift_open", "Смена #"+itoa(id)+" открыта")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

func shiftCloseHandler(w http.ResponseWriter, r *http.Request) {
	id := currentShiftID()
	if id == 0 {
		writeErr(w, 400, "Нет открытой смены")
		return
	}
	var req struct {
		ClosingBalance float64 `json:"closing_balance"`
	}
	jsonDecode(r, &req)

	timeTotal := timeSalesTotal(id)
	goodsTotal := goodsSalesTotal(id)
	total := round2(timeTotal + goodsTotal)

	db.Exec("UPDATE shifts SET status = 'closed', closed_at = ?, closing_balance = ?, total_sales = ? WHERE id = ?",
		nowUnix(), req.ClosingBalance, total, id)
	recordTransaction(TxClosing, 0, 0, currentUserID(r), id, 0, req.ClosingBalance, 0, "cash", "Закрытие смены")
	notifyTelegram("🔒 Смена закрыта. Выручка: " + moneyStr(total) +
		" (услуги " + moneyStr(timeTotal) + ", товары " + moneyStr(goodsTotal) + ")")
	logAction(r, "shift_close", "Смена #"+itoa(id)+" закрыта · выручка "+moneyStr(total))
	writeJSON(w, 200, map[string]interface{}{"total_sales": total, "time_total": timeTotal, "goods_total": goodsTotal})
}

func shiftCurrentHandler(w http.ResponseWriter, r *http.Request) {
	id := currentShiftID()
	if id == 0 {
		writeJSON(w, 200, map[string]interface{}{"shift": nil})
		return
	}
	var s Shift
	err := db.QueryRow(`
		SELECT s.id, s.user_id, COALESCE(u.username,''), s.opened_at, s.closed_at,
		       s.opening_balance, s.closing_balance, s.total_sales, s.status
		FROM shifts s LEFT JOIN users u ON u.id = s.user_id
		WHERE s.id = ?`, id).
		Scan(&s.ID, &s.UserID, &s.UserName, &s.OpenedAt, &s.ClosedAt, &s.OpeningBalance, &s.ClosingBalance, &s.TotalSales, &s.Status)

	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"shift": nil})
		return
	}

	timeTotal := timeSalesTotal(id)
	goodsTotal := goodsSalesTotal(id)
	s.TotalSales = round2(timeTotal + goodsTotal)

	writeJSON(w, 200, map[string]interface{}{
		"shift":       s,
		"time_total":  timeTotal,
		"goods_total": goodsTotal,
	})
}

func shiftsHistoryHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT s.id, s.user_id, COALESCE(u.username,''), s.opened_at, s.closed_at,
		       s.opening_balance, s.closing_balance, s.total_sales, s.status
		FROM shifts s LEFT JOIN users u ON u.id = s.user_id
		ORDER BY s.id DESC LIMIT 100`)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []Shift{}
	for rows.Next() {
		var s Shift
		rows.Scan(&s.ID, &s.UserID, &s.UserName, &s.OpenedAt, &s.ClosedAt, &s.OpeningBalance, &s.ClosingBalance, &s.TotalSales, &s.Status)
		list = append(list, s)
	}
	writeJSON(w, 200, map[string]interface{}{"shifts": list})
}

func moneyStr(v float64) string {
	return strconv.FormatFloat(round2(v), 'f', 2, 64)
}
