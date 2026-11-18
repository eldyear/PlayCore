package main

import (
	"net/http"
	"strconv"
)

// Отчёт по транзакциям (услуги + товары) с фильтрами.
func reportTransactionsHandler(w http.ResponseWriter, r *http.Request) {
	from := parseIntParam(r, "from")
	to := parseIntParam(r, "to")
	pcID := parseIntParam(r, "pc_id")
	userID := parseIntParam(r, "user_id")

	q := `
		SELECT t.id, t.kind, t.session_id, t.client_id, COALESCE(c.name,''), t.user_id, COALESCE(u.username,''),
		       t.shift_id, t.pc_id, COALESCE(p.name,''), t.amount, t.seconds, t.method, t.created_at, t.note
		FROM transactions t
		LEFT JOIN clients c ON c.id = t.client_id
		LEFT JOIN users u ON u.id = t.user_id
		LEFT JOIN pcs p ON p.id = t.pc_id
		WHERE 1=1`
	args := []interface{}{}
	if from > 0 {
		q += " AND t.created_at >= ?"
		args = append(args, from)
	}
	if to > 0 {
		q += " AND t.created_at <= ?"
		args = append(args, to)
	}
	if pcID > 0 {
		q += " AND t.pc_id = ?"
		args = append(args, pcID)
	}
	if userID > 0 {
		q += " AND t.user_id = ?"
		args = append(args, userID)
	}
	q += " ORDER BY t.id DESC LIMIT 1000"

	rows, err := db.Query(q, args...)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()

	list := []Transaction{}
	var timeCash, timeCard, timeBalance, timeTotal float64
	for rows.Next() {
		var t Transaction
		rows.Scan(&t.ID, &t.Kind, &t.SessionID, &t.ClientID, &t.ClientName, &t.UserID, &t.UserName,
			&t.ShiftID, &t.PCID, &t.PCName, &t.Amount, &t.Seconds, &t.Method, &t.CreatedAt, &t.Note)
		list = append(list, t)
		if t.Kind == TxTopup || t.Kind == TxPackage || t.Kind == TxBalancePay || t.Kind == TxClientDeposit {
			timeTotal += t.Amount
			switch t.Method {
			case "cash":
				timeCash += t.Amount
			case "card":
				timeCard += t.Amount
			case "balance":
				timeBalance += t.Amount
			}
		}
	}

	// Товары
	sq := `
		SELECT s.id, s.product_id, s.product_name, s.qty, s.unit_price, s.amount,
		       s.user_id, COALESCE(u.username,''), s.shift_id, s.method, s.created_at
		FROM sales s LEFT JOIN users u ON u.id = s.user_id
		WHERE 1=1`
	sargs := []interface{}{}
	if from > 0 {
		sq += " AND s.created_at >= ?"
		sargs = append(sargs, from)
	}
	if to > 0 {
		sq += " AND s.created_at <= ?"
		sargs = append(sargs, to)
	}
	if userID > 0 {
		sq += " AND s.user_id = ?"
		sargs = append(sargs, userID)
	}
	sq += " ORDER BY s.id DESC LIMIT 1000"

	srows, err := db.Query(sq, sargs...)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer srows.Close()

	sales := []Sale{}
	var goodsCash, goodsCard, goodsTotal float64
	for srows.Next() {
		var s Sale
		srows.Scan(&s.ID, &s.ProductID, &s.ProductName, &s.Qty, &s.UnitPrice, &s.Amount,
			&s.UserID, &s.UserName, &s.ShiftID, &s.Method, &s.CreatedAt)
		sales = append(sales, s)
		goodsTotal += s.Amount
		if s.Method == "card" {
			goodsCard += s.Amount
		} else {
			goodsCash += s.Amount
		}
	}

	writeJSON(w, 200, map[string]interface{}{
		"transactions":   list,
		"sales":          sales,
		"time_total":     round2(timeTotal),
		"goods_total":    round2(goodsTotal),
		"total_sales":    round2(timeTotal + goodsTotal),
		"total_cash":     round2(timeCash + goodsCash),
		"total_card":     round2(timeCard + goodsCard),
		"total_balance":  round2(timeBalance),
	})
}

func parseIntParam(r *http.Request, key string) int64 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
