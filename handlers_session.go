package main

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
)

// --- Вспомогательные функции ---

func resolveTariff(tariffID, pcID int64) Tariff {
	var t Tariff
	if tariffID > 0 {
		var active int
		err := db.QueryRow("SELECT id, name, kind, price_per_minute, price_per_hour, duration_minutes, package_price, active FROM tariffs WHERE id = ?", tariffID).
			Scan(&t.ID, &t.Name, &t.Kind, &t.PricePerMinute, &t.PricePerHour, &t.DurationMinutes, &t.PackagePrice, &active)
		if err == nil {
			t.Active = active == 1
			return t
		}
	}
	if pcID > 0 {
		var pid int64
		db.QueryRow("SELECT tariff_id FROM pcs WHERE id = ?", pcID).Scan(&pid)
		if pid > 0 {
			return resolveTariff(pid, 0)
		}
	}
	// Дефолтный тариф
	s := getSettings()
	t = Tariff{ID: 0, Name: "Стандарт", Kind: TariffHourly, PricePerMinute: s.DefaultPricePerMin, Active: true}
	return t
}

// tariffPpm — цена за минуту по тарифу (цена за час → /60; пакет → цена/длительность).
func tariffPpm(t Tariff) float64 {
	if t.Kind == TariffPackage {
		if t.DurationMinutes > 0 {
			return t.PackagePrice / float64(t.DurationMinutes)
		}
		return 0
	}
	if t.PricePerHour > 0 {
		return t.PricePerHour / 60
	}
	return t.PricePerMinute
}

// computeSecondsAmount возвращает секунды и сумму по тарифу и входным данным
func computeSecondsAmount(t Tariff, minutes int64, amount float64) (int64, float64, error) {
	maxMin := getSettings().MaxMinutes
	capSeconds := func(sec int64) int64 {
		if maxMin > 0 && sec > maxMin*60 {
			return maxMin * 60
		}
		return sec
	}

	if t.Kind == TariffPackage {
		sec := t.DurationMinutes * 60
		return capSeconds(sec), round2(t.PackagePrice), nil
	}
	// hourly / night — цена за час → за минуту
	ppm := tariffPpm(t)
	if ppm <= 0 {
		ppm = getSettings().DefaultPricePerMin
	}
	if minutes > 0 {
		sec := minutes * 60
		return capSeconds(sec), round2(ppm * float64(minutes)), nil
	}
	if amount > 0 {
		sec := int64(amount / ppm * 60)
		return capSeconds(sec), round2(amount), nil
	}
	return 0, 0, errNeedAmount
}

var errNeedAmount = errInvalidInput

var errInvalidInput = errors.New("Укажите минуты или сумму")

func currentShiftID() int64 {
	var id int64
	db.QueryRow("SELECT id FROM shifts WHERE status = 'open' ORDER BY id DESC LIMIT 1").Scan(&id)
	return id
}

// ensureOpenShift проверяет, что для продажи (наличные/карта) открыта смена.
func ensureOpenShift(w http.ResponseWriter, method string) bool {
	if !getSettings().RequireShift {
		return true
	}
	if method == "balance" {
		return true // оплата с предоплаченного баланса — без наличных
	}
	if currentShiftID() == 0 {
		writeErr(w, 400, "Откройте смену, чтобы проводить продажи")
		return false
	}
	return true
}

func recordTransaction(kind string, sessionID, clientID, userID, shiftID, pcID int64, amount float64, seconds int64, method, note string) int64 {
	id, err := insertID(
		"INSERT INTO transactions (kind, session_id, client_id, user_id, shift_id, pc_id, amount, seconds, method, created_at, note) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		kind, sessionID, clientID, userID, shiftID, pcID, amount, seconds, method, nowUnix(), note,
	)
	if err != nil {
		return 0
	}
	return id
}

func loadSession(id int64) (*Session, bool) {
	var s Session
	err := db.QueryRow(`
		SELECT s.id, s.pc_id, COALESCE(p.name,''), s.client_id, COALESCE(c.name,''),
		       s.tariff_id, COALESCE(t.name,''), s.user_id, COALESCE(u.username,''),
		       s.shift_id, s.started_at, s.ends_at, s.paused_at, s.status,
		       s.total_seconds, s.total_amount, s.ended_at, s.note,
		       COALESCE(s.paused_total,0), COALESCE(s.final_seconds,0), COALESCE(s.final_amount,0),
		       COALESCE(s.open,0)
		FROM sessions s
		LEFT JOIN pcs p ON p.id = s.pc_id
		LEFT JOIN clients c ON c.id = s.client_id
		LEFT JOIN tariffs t ON t.id = s.tariff_id
		LEFT JOIN users u ON u.id = s.user_id
		WHERE s.id = ?`, id).
		Scan(&s.ID, &s.PCID, &s.PCName, &s.ClientID, &s.ClientName,
			&s.TariffID, &s.TariffName, &s.UserID, &s.UserName,
			&s.ShiftID, &s.StartedAt, &s.EndsAt, &s.PausedAt, &s.Status,
			&s.TotalSeconds, &s.TotalAmount, &s.EndedAt, &s.Note,
			&s.PausedTotal, &s.FinalSeconds, &s.FinalAmount, &s.Open)
	if err != nil {
		return nil, false
	}
	now := nowUnix()
	switch s.Status {
	case SessionActive:
		s.RemainingSeconds = s.EndsAt - now
	case SessionPaused:
		s.RemainingSeconds = s.EndsAt - s.PausedAt
	default:
		s.RemainingSeconds = 0
	}
	if s.RemainingSeconds < 0 {
		s.RemainingSeconds = 0
	}
	return &s, true
}

func endSession(sessionID, endedBy int64) {
	sess, ok := loadSession(sessionID)
	if !ok || sess.Status == SessionEnded {
		return
	}
	now := nowUnix()

	finalSeconds := sess.TotalSeconds
	finalAmount := sess.TotalAmount
	if sess.Open {
		// открытый тариф: списываем по фактическому времени
		finalSeconds = now - sess.StartedAt - sess.PausedTotal
		if finalSeconds < 0 {
			finalSeconds = 0
		}
		if finalSeconds > sess.TotalSeconds {
			finalSeconds = sess.TotalSeconds
		}
		finalAmount = chargeOpenSession(sess, finalSeconds)
	}

	db.Exec("UPDATE sessions SET status = 'ended', ended_at = ?, ended_by = ?, final_seconds = ?, final_amount = ? WHERE id = ?",
		now, endedBy, finalSeconds, finalAmount, sessionID)
	db.Exec("UPDATE pcs SET status = 'free', current_session_id = 0 WHERE id = ? AND current_session_id = ?", sess.PCID, sessionID)
}

// --- Старт сессии ---

type sessionStartRequest struct {
	PCID     int64   `json:"pc_id"`
	TariffID int64   `json:"tariff_id"`
	ClientID int64   `json:"client_id"`
	Minutes  int64   `json:"minutes"`
	Amount   float64 `json:"amount"`
	Method   string  `json:"method"`
	Note     string  `json:"note"`
}

// computeSession рассчитывает тариф, секунды и сумму (с учётом скидки клиента) без записи в БД.
func computeSession(tariffID, pcID, clientID, minutes int64, amount float64) (Tariff, int64, float64, error) {
	tariff := resolveTariff(tariffID, pcID)
	seconds, amt, err := computeSecondsAmount(tariff, minutes, amount)
	if err != nil {
		return tariff, 0, 0, err
	}
	if clientID > 0 {
		var disc float64
		db.QueryRow("SELECT discount_percent FROM clients WHERE id = ?", clientID).Scan(&disc)
		if disc > 0 {
			amt = round2(amt * (1 - disc/100))
		}
	}
	return tariff, seconds, amt, nil
}

// createSession записывает сессию, помечает ПК занятым, пишет транзакцию и начисляет бонусы.
func createSession(req sessionStartRequest, tariff Tariff, seconds int64, amount float64, userID, shiftID int64) (int64, error) {
	started := nowUnix()
	ends := started + seconds

	sessID, err := insertID(`
		INSERT INTO sessions (pc_id, client_id, tariff_id, user_id, shift_id, started_at, ends_at, status, total_seconds, total_amount, note)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		req.PCID, req.ClientID, tariff.ID, userID, shiftID, started, ends, SessionActive, seconds, amount, req.Note)
	if err != nil {
		return 0, err
	}

	db.Exec("UPDATE pcs SET status = 'busy', current_session_id = ? WHERE id = ?", sessID, req.PCID)

	kind := TxTopup
	if tariff.Kind == TariffPackage {
		kind = TxPackage
	}
	if req.Method == "balance" {
		kind = TxBalancePay
	}
	recordTransaction(kind, sessID, req.ClientID, userID, shiftID, req.PCID, amount, seconds, req.Method, req.Note)

	if req.ClientID > 0 && (req.Method == "cash" || req.Method == "card") {
		bonus := int64(amount * getSettings().BonusRate)
		if bonus > 0 {
			db.Exec("UPDATE clients SET bonus_points = bonus_points + ? WHERE id = ?", bonus, req.ClientID)
		}
	}

	return sessID, nil
}

// createOpenSession — открытая сессия (пост-оплата): баланс спишется по завершении.
func createOpenSession(pcID, clientID, tariffID int64, maxSeconds int64, note string) (int64, error) {
	started := nowUnix()
	ends := started + maxSeconds
	sessID, err := insertID(`
		INSERT INTO sessions (pc_id, client_id, tariff_id, user_id, shift_id, started_at, ends_at, status, total_seconds, total_amount, note, open)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,1)`,
		pcID, clientID, tariffID, 0, currentShiftID(), started, ends, SessionActive, maxSeconds, 0, note)
	if err != nil {
		return 0, err
	}
	db.Exec("UPDATE pcs SET status = 'busy', current_session_id = ? WHERE id = ?", sessID, pcID)
	return sessID, nil
}

// chargeOpenSession — списывает с баланса клиента стоимость фактического времени
// открытой сессии (пост-оплата). Возвращает списанную сумму.
func chargeOpenSession(sess *Session, actualSeconds int64) float64 {
	if !sess.Open || sess.ClientID <= 0 || actualSeconds <= 0 {
		return 0
	}
	ppm := tariffPpm(resolveTariff(sess.TariffID, sess.PCID))
	if ppm <= 0 {
		ppm = getSettings().DefaultPricePerMin
	}
	var disc float64
	db.QueryRow("SELECT discount_percent FROM clients WHERE id = ?", sess.ClientID).Scan(&disc)
	if disc > 0 {
		ppm = ppm * (1 - disc/100)
	}
	if ppm <= 0 {
		return 0
	}
	charge := round2(float64(actualSeconds) / 60 * ppm)
	var bal float64
	db.QueryRow("SELECT balance FROM clients WHERE id = ?", sess.ClientID).Scan(&bal)
	if charge > bal {
		charge = round2(bal)
	}
	if charge > 0 {
		db.Exec("UPDATE clients SET balance = balance - ? WHERE id = ?", charge, sess.ClientID)
		recordTransaction(TxBalancePay, sess.ID, sess.ClientID, 0, sess.ShiftID, sess.PCID, charge, actualSeconds, "balance", "Открытый тариф: оплата по завершении")
	}
	return charge
}

func sessionStartHandler(w http.ResponseWriter, r *http.Request) {
	var req sessionStartRequest
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.PCID <= 0 {
		writeErr(w, 400, "Укажите ПК")
		return
	}
	if req.Method == "" {
		req.Method = "cash"
	}
	if !ensureOpenShift(w, req.Method) {
		return
	}

	// ПК свободен?
	var curSess int64
	err := db.QueryRow("SELECT current_session_id FROM pcs WHERE id = ?", req.PCID).Scan(&curSess)
	if err == sql.ErrNoRows {
		writeErr(w, 404, "ПК не найден")
		return
	}
	if curSess > 0 {
		writeErr(w, 400, "На этом ПК уже идёт сессия")
		return
	}

	tariff, seconds, amount, err := computeSession(req.TariffID, req.PCID, req.ClientID, req.Minutes, req.Amount)
	if err != nil {
		writeErr(w, 400, "Укажите минуты или сумму")
		return
	}

	// Оплата с баланса клиента
	if req.Method == "balance" {
		if req.ClientID <= 0 {
			writeErr(w, 400, "Оплата с баланса требует клиента")
			return
		}
		var bal float64
		db.QueryRow("SELECT balance FROM clients WHERE id = ?", req.ClientID).Scan(&bal)
		if bal < amount {
			writeErr(w, 400, "Недостаточно средств на балансе клиента")
			return
		}
		db.Exec("UPDATE clients SET balance = balance - ? WHERE id = ?", amount, req.ClientID)
	}

	userID := currentUserID(r)
	shiftID := currentShiftID()

	sessID, err := createSession(req, tariff, seconds, amount, userID, shiftID)
	if err != nil {
		writeErr(w, 500, "Ошибка создания сессии")
		return
	}

	s, _ := loadSession(sessID)
	logAction(r, "session_start", "Сеанс на ПК #"+itoa(req.PCID)+" · "+tariff.Name+" · "+itoa(seconds/60)+" мин · "+moneyStr(amount))
	writeJSON(w, 200, map[string]interface{}{"session": s, "amount": amount, "seconds": seconds})
}

// sessionBulkStartHandler — групповой запуск: одинаковые условия на несколько ПК сразу.
func sessionBulkStartHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCIDs    []int64 `json:"pc_ids"`
		TariffID int64   `json:"tariff_id"`
		ClientID int64   `json:"client_id"`
		Minutes  int64   `json:"minutes"`
		Amount   float64 `json:"amount"`
		Method   string  `json:"method"`
		Note     string  `json:"note"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if len(req.PCIDs) == 0 {
		writeErr(w, 400, "Выберите хотя бы один ПК")
		return
	}
	if req.Method == "" {
		req.Method = "cash"
	}
	if !ensureOpenShift(w, req.Method) {
		return
	}

	userID := currentUserID(r)
	shiftID := currentShiftID()

	type planned struct {
		req     sessionStartRequest
		tariff  Tariff
		seconds int64
		amount  float64
	}
	var plans []planned
	failed := []map[string]interface{}{}

	// предварительный расчёт и проверка занятости по каждому ПК
	for _, pcID := range req.PCIDs {
		var curSess int64
		err := db.QueryRow("SELECT current_session_id FROM pcs WHERE id = ?", pcID).Scan(&curSess)
		if err == sql.ErrNoRows {
			failed = append(failed, map[string]interface{}{"pc_id": pcID, "error": "ПК не найден"})
			continue
		}
		if curSess > 0 {
			failed = append(failed, map[string]interface{}{"pc_id": pcID, "error": "Уже идёт сессия"})
			continue
		}
		tariff, seconds, amount, err := computeSession(req.TariffID, pcID, req.ClientID, req.Minutes, req.Amount)
		if err != nil {
			failed = append(failed, map[string]interface{}{"pc_id": pcID, "error": "Укажите минуты или сумму"})
			continue
		}
		plans = append(plans, planned{
			req:     sessionStartRequest{PCID: pcID, TariffID: req.TariffID, ClientID: req.ClientID, Minutes: req.Minutes, Amount: req.Amount, Method: req.Method, Note: req.Note},
			tariff:  tariff,
			seconds: seconds,
			amount:  amount,
		})
	}

	// Оплата с баланса — проверяем общую сумму по всем выбранным ПК
	if req.Method == "balance" && len(plans) > 0 {
		if req.ClientID <= 0 {
			writeErr(w, 400, "Оплата с баланса требует клиента")
			return
		}
		var total float64
		for _, p := range plans {
			total += p.amount
		}
		var bal float64
		db.QueryRow("SELECT balance FROM clients WHERE id = ?", req.ClientID).Scan(&bal)
		if bal < total {
			writeErr(w, 400, "Недостаточно средств на балансе клиента")
			return
		}
		db.Exec("UPDATE clients SET balance = balance - ? WHERE id = ?", total, req.ClientID)
	}

	// Создаём сессии
	started := []map[string]interface{}{}
	for _, p := range plans {
		sessID, err := createSession(p.req, p.tariff, p.seconds, p.amount, userID, shiftID)
		if err != nil {
			failed = append(failed, map[string]interface{}{"pc_id": p.req.PCID, "error": "Ошибка создания сессии"})
			continue
		}
		started = append(started, map[string]interface{}{"pc_id": p.req.PCID, "session_id": sessID, "seconds": p.seconds, "amount": p.amount})
	}

	writeJSON(w, 200, map[string]interface{}{"started": started, "failed": failed})
}

// --- Пополнение времени сессии ---

func sessionAddTimeHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Minutes int64   `json:"minutes"`
		Amount  float64 `json:"amount"`
		Method  string  `json:"method"`
		Note    string  `json:"note"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Method == "" {
		req.Method = "cash"
	}
	if !ensureOpenShift(w, req.Method) {
		return
	}

	sess, ok := loadSession(id)
	if !ok {
		writeErr(w, 404, "Сессия не найдена")
		return
	}
	if sess.Status == SessionEnded {
		writeErr(w, 400, "Сессия завершена")
		return
	}

	tariff := resolveTariff(sess.TariffID, sess.PCID)
	seconds, amount, err := computeSecondsAmount(tariff, req.Minutes, req.Amount)
	if err != nil {
		writeErr(w, 400, "Укажите минуты или сумму")
		return
	}

	if sess.ClientID > 0 {
		var disc float64
		db.QueryRow("SELECT discount_percent FROM clients WHERE id = ?", sess.ClientID).Scan(&disc)
		if disc > 0 {
			amount = round2(amount * (1 - disc/100))
		}
	}

	if req.Method == "balance" {
		if sess.ClientID <= 0 {
			writeErr(w, 400, "Оплата с баланса требует клиента")
			return
		}
		var bal float64
		db.QueryRow("SELECT balance FROM clients WHERE id = ?", sess.ClientID).Scan(&bal)
		if bal < amount {
			writeErr(w, 400, "Недостаточно средств на балансе")
			return
		}
		db.Exec("UPDATE clients SET balance = balance - ? WHERE id = ?", amount, sess.ClientID)
	}

	// Продлеваем ends_at и сбрасываем флаг уведомления
	db.Exec("UPDATE sessions SET ends_at = ends_at + ?, total_seconds = total_seconds + ?, total_amount = total_amount + ?, low_notified = 0 WHERE id = ?",
		seconds, seconds, amount, id)

	userID := currentUserID(r)
	kind := TxTopup
	if tariff.Kind == TariffPackage {
		kind = TxPackage
	}
	if req.Method == "balance" {
		kind = TxBalancePay
	}
	recordTransaction(kind, id, sess.ClientID, userID, sess.ShiftID, sess.PCID, amount, seconds, req.Method, req.Note)

	if sess.ClientID > 0 && (req.Method == "cash" || req.Method == "card") {
		bonus := int64(amount * getSettings().BonusRate)
		if bonus > 0 {
			db.Exec("UPDATE clients SET bonus_points = bonus_points + ? WHERE id = ?", bonus, sess.ClientID)
		}
	}

	s, _ := loadSession(id)
	writeJSON(w, 200, map[string]interface{}{"session": s, "amount": amount, "seconds": seconds})
}

// --- Пауза ---

func sessionPauseHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	sess, ok := loadSession(id)
	if !ok {
		writeErr(w, 404, "Сессия не найдена")
		return
	}
	if sess.Status != SessionActive {
		writeErr(w, 400, "Сессию нельзя поставить на паузу")
		return
	}
	db.Exec("UPDATE sessions SET paused_at = ?, status = 'paused' WHERE id = ?", nowUnix(), id)
	db.Exec("UPDATE pcs SET status = 'busy' WHERE id = ?", sess.PCID)
	s, _ := loadSession(id)
	writeJSON(w, 200, map[string]interface{}{"session": s})
}

// --- Возобновление ---

func sessionResumeHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	sess, ok := loadSession(id)
	if !ok {
		writeErr(w, 404, "Сессия не найдена")
		return
	}
	if sess.Status != SessionPaused {
		writeErr(w, 400, "Сессия не на паузе")
		return
	}
	now := nowUnix()
	// замёрзший остаток = ends_at - paused_at
	remaining := sess.EndsAt - sess.PausedAt
	if remaining <= 0 {
		remaining = 0
	}
	// накапливаем длительность этой паузы в paused_total
	pausedDuration := now - sess.PausedAt
	if pausedDuration < 0 {
		pausedDuration = 0
	}
	db.Exec("UPDATE sessions SET paused_at = 0, status = 'active', ends_at = ?, paused_total = paused_total + ? WHERE id = ?", now+remaining, pausedDuration, id)
	s, _ := loadSession(id)
	writeJSON(w, 200, map[string]interface{}{"session": s})
}

// --- Завершение ---

// stopSession завершает сессию, считает «по факту» и возвращает возврат (refund).
func stopSession(id int64, userID int64) (int64, float64, float64, int64, bool) {
	sess, ok := loadSession(id)
	if !ok || sess.Status == SessionEnded {
		return 0, 0, 0, 0, false
	}

	now := nowUnix()
	paused := sess.PausedTotal
	if sess.PausedAt > 0 && now > sess.PausedAt {
		paused += now - sess.PausedAt
	}
	actualSeconds := (now - sess.StartedAt) - paused
	if actualSeconds < 0 {
		actualSeconds = 0
	}
	if actualSeconds > sess.TotalSeconds {
		actualSeconds = sess.TotalSeconds
	}

	finalAmount := 0.0
	refund := 0.0

	if sess.Open {
		// открытый тариф: списываем по факту, возврата нет
		finalAmount = chargeOpenSession(sess, actualSeconds)
	} else {
		// предоплаченная: итог по эффективной ставке + возврат неиспользованного
		if sess.TotalSeconds > 0 {
			finalAmount = round2(sess.TotalAmount * float64(actualSeconds) / float64(sess.TotalSeconds))
		}
		refund = round2(sess.TotalAmount - finalAmount)
		if refund < 0 {
			refund = 0
		}
		if refund > 0 && sess.ClientID > 0 && sessionPaidByBalance(id) {
			db.Exec("UPDATE clients SET balance = balance + ? WHERE id = ?", refund, sess.ClientID)
			recordTransaction(TxRefund, id, sess.ClientID, userID, sess.ShiftID, sess.PCID, refund, 0, "balance", "Возврат за неиспользованное время")
		}
	}

	db.Exec("UPDATE sessions SET status = 'ended', ended_at = ?, ended_by = ?, final_seconds = ?, final_amount = ? WHERE id = ?",
		now, userID, actualSeconds, finalAmount, id)
	db.Exec("UPDATE pcs SET status = 'free', current_session_id = 0 WHERE id = ? AND current_session_id = ?", sess.PCID, id)

	return actualSeconds, finalAmount, refund, sess.PCID, true
}

func sessionStopHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	sess, ok := loadSession(id)
	if !ok {
		writeErr(w, 404, "Сессия не найдена")
		return
	}
	if sess.Status == SessionEnded {
		writeErr(w, 400, "Сессия уже завершена")
		return
	}

	actualSeconds, finalAmount, refund, pcID, _ := stopSession(id, currentUserID(r))
	s, _ := loadSession(id)
	logAction(r, "session_stop", "Сеанс на ПК #"+itoa(pcID)+" завершён · "+itoa(actualSeconds/60)+" мин · "+moneyStr(finalAmount)+" (возврат "+moneyStr(refund)+")")
	writeJSON(w, 200, map[string]interface{}{
		"session":       s,
		"final_amount":  finalAmount,
		"final_seconds": actualSeconds,
		"total_amount":  sess.TotalAmount,
		"refund":        refund,
	})
}

func sessionPaidByBalance(sessionID int64) bool {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM transactions WHERE session_id = ? AND method = 'balance'", sessionID).Scan(&n)
	return n > 0
}

// --- Списки сессий ---

func sessionsActiveHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT s.id, s.pc_id, COALESCE(p.name,''), s.client_id, COALESCE(c.name,''),
		       s.tariff_id, COALESCE(t.name,''), s.user_id, COALESCE(u.username,''),
		       s.shift_id, s.started_at, s.ends_at, s.paused_at, s.status,
		       s.total_seconds, s.total_amount, s.ended_at, s.note
		FROM sessions s
		LEFT JOIN pcs p ON p.id = s.pc_id
		LEFT JOIN clients c ON c.id = s.client_id
		LEFT JOIN tariffs t ON t.id = s.tariff_id
		LEFT JOIN users u ON u.id = s.user_id
		WHERE s.status IN ('active','paused')
		ORDER BY s.id DESC`)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()

	list := []Session{}
	now := nowUnix()
	for rows.Next() {
		var s Session
		rows.Scan(&s.ID, &s.PCID, &s.PCName, &s.ClientID, &s.ClientName,
			&s.TariffID, &s.TariffName, &s.UserID, &s.UserName,
			&s.ShiftID, &s.StartedAt, &s.EndsAt, &s.PausedAt, &s.Status,
			&s.TotalSeconds, &s.TotalAmount, &s.EndedAt, &s.Note)
		if s.Status == SessionActive {
			s.RemainingSeconds = s.EndsAt - now
		} else {
			s.RemainingSeconds = s.EndsAt - s.PausedAt
		}
		if s.RemainingSeconds < 0 {
			s.RemainingSeconds = 0
		}
		list = append(list, s)
	}
	writeJSON(w, 200, map[string]interface{}{"sessions": list})
}

func sessionsHistoryHandler(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	// фильтр по дате (Unix-границы: from <= ended_at < to)
	q := r.URL.Query()
	from, _ := strconv.ParseInt(q.Get("from"), 10, 64)
	to, _ := strconv.ParseInt(q.Get("to"), 10, 64)

	where := "s.status = 'ended'"
	args := []interface{}{}
	if from > 0 {
		where += " AND s.ended_at >= ?"
		args = append(args, from)
	}
	if to > 0 {
		where += " AND s.ended_at < ?"
		args = append(args, to)
	}
	args = append(args, limit)

	rows, err := db.Query(`
		SELECT s.id, s.pc_id, COALESCE(p.name,''), s.client_id, COALESCE(c.name,''),
		       s.tariff_id, COALESCE(t.name,''), s.user_id, COALESCE(u.username,''),
		       s.shift_id, s.started_at, s.ends_at, s.paused_at, s.status,
		       s.total_seconds, s.total_amount, s.ended_at, s.note,
		       COALESCE(s.final_seconds,0), COALESCE(s.final_amount,0)
		FROM sessions s
		LEFT JOIN pcs p ON p.id = s.pc_id
		LEFT JOIN clients c ON c.id = s.client_id
		LEFT JOIN tariffs t ON t.id = s.tariff_id
		LEFT JOIN users u ON u.id = s.user_id
		WHERE `+where+`
		ORDER BY s.id DESC LIMIT ?`, args...)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()

	list := []Session{}
	for rows.Next() {
		var s Session
		rows.Scan(&s.ID, &s.PCID, &s.PCName, &s.ClientID, &s.ClientName,
			&s.TariffID, &s.TariffName, &s.UserID, &s.UserName,
			&s.ShiftID, &s.StartedAt, &s.EndsAt, &s.PausedAt, &s.Status,
			&s.TotalSeconds, &s.TotalAmount, &s.EndedAt, &s.Note,
			&s.FinalSeconds, &s.FinalAmount)
		s.RemainingSeconds = 0
		list = append(list, s)
	}
	writeJSON(w, 200, map[string]interface{}{"sessions": list})
}
