package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// --- Список ПК с вычисляемым статусом ---

func pcsListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT p.id, p.name, p.zone, p.tariff_id, p.status, p.current_session_id, p.is_online, p.last_seen, p.notes, p.compliance,
			p.layout, p.comment, p.cpu, p.ram, p.gpu, p.maintenance, p.mac, p.disks,
			COALESCE(s.id,0), COALESCE(s.ends_at,0), COALESCE(s.started_at,0), COALESCE(s.client_id,0), COALESCE(c.name,''),
			COALESCE(s.status,''), COALESCE(s.paused_at,0)
		FROM pcs p
		LEFT JOIN sessions s ON s.id = p.current_session_id AND s.status IN ('active','paused')
		LEFT JOIN clients c ON c.id = s.client_id
		ORDER BY p.id`)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()

	now := nowUnix()
	list := []PC{}
	for rows.Next() {
		var p PC
		var sessID, endsAt, startedAt, clientID, pausedAt int64
		var clientName, sessStatus string
		var isOnline, maintenance int
		var disksJSON string
		rows.Scan(&p.ID, &p.Name, &p.Zone, &p.TariffID, &p.Status, &p.CurrentSessionID, &isOnline, &p.LastSeen, &p.Notes, &p.Compliance,
			&p.Layout, &p.Comment, &p.CPU, &p.RAM, &p.GPU, &maintenance, &p.MAC, &disksJSON,
			&sessID, &endsAt, &startedAt, &clientID, &clientName, &sessStatus, &pausedAt)
		p.IsOnline = isOnline == 1
		p.Maintenance = maintenance == 1
		p.Disks = parseDisks(disksJSON)

		if p.Maintenance {
			p.Status = PCStatusMaintenance
			p.CurrentSessionID = 0
		} else if sessID > 0 {
			p.CurrentSessionID = sessID
			p.Status = PCStatusBusy
			p.SessionStartedAt = startedAt
			p.ClientName = clientName
			p.SessionPaused = sessStatus == SessionPaused
			var rem int64
			if sessStatus == SessionPaused {
				rem = endsAt - pausedAt
			} else {
				rem = endsAt - now
			}
			if rem < 0 {
				rem = 0
			}
			p.RemainingSeconds = rem
		} else {
			p.CurrentSessionID = 0
			if p.IsOnline {
				p.Status = PCStatusFree
			} else {
				p.Status = PCStatusOffline
			}
		}
		list = append(list, p)
	}

	// последний завершённый сеанс по каждому ПК (для показа суммы на карточке)
	lastRows, err := db.Query(`
		SELECT s.pc_id, COALESCE(s.final_amount,0), COALESCE(s.final_seconds,0), s.ended_at
		FROM sessions s
		JOIN (SELECT pc_id, MAX(id) mid FROM sessions WHERE status = 'ended' GROUP BY pc_id) m ON m.mid = s.id`)
	if err == nil {
		type last struct {
			amount  float64
			seconds int64
			endedAt int64
		}
		lastMap := map[int64]last{}
		for lastRows.Next() {
			var pcID int64
			var l last
			lastRows.Scan(&pcID, &l.amount, &l.seconds, &l.endedAt)
			lastMap[pcID] = l
		}
		lastRows.Close()
		for i := range list {
			if l, ok := lastMap[list[i].ID]; ok {
				list[i].LastSessionAmount = l.amount
				list[i].LastSessionSeconds = l.seconds
				list[i].LastSessionEndedAt = l.endedAt
			}
		}
	}

	writeJSON(w, 200, map[string]interface{}{"pcs": list})
}

// --- Создание ПК ---

func pcCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Zone     string `json:"zone"`
		TariffID int64  `json:"tariff_id"`
		Notes    string `json:"notes"`
		MAC      string `json:"mac"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Name == "" {
		writeErr(w, 400, "Укажите название ПК")
		return
	}
	if req.Zone == "" {
		req.Zone = "Основной зал"
	}
	secret := randomToken()
	id, err := insertID(
		"INSERT INTO pcs (name, zone, tariff_id, notes, token_hash, mac) VALUES (?,?,?,?,?,?)",
		req.Name, req.Zone, req.TariffID, req.Notes, hashToken(secret), req.MAC,
	)
	if err != nil {
		writeErr(w, 500, "Ошибка создания")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"id": id, "agent_token": secret})
}

// --- Перевыпуск токена агента для ПК ---

func pcRotateTokenHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	secret := randomToken()
	res, err := db.Exec("UPDATE pcs SET token_hash = ? WHERE id = ?", hashToken(secret), id)
	if err != nil {
		writeErr(w, 500, "Ошибка")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "ПК не найден")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"agent_token": secret})
}

// --- Обновление ПК ---

func pcUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Name     *string `json:"name"`
		Zone     *string `json:"zone"`
		TariffID *int64  `json:"tariff_id"`
		Notes    *string `json:"notes"`
		Comment  *string `json:"comment"`
		CPU      *string `json:"cpu"`
		RAM      *string `json:"ram"`
		GPU      *string `json:"gpu"`
		Layout   *int64  `json:"layout"`
		MAC      *string `json:"mac"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Name != nil {
		db.Exec("UPDATE pcs SET name = ? WHERE id = ?", *req.Name, id)
	}
	if req.Zone != nil {
		db.Exec("UPDATE pcs SET zone = ? WHERE id = ?", *req.Zone, id)
	}
	if req.TariffID != nil {
		db.Exec("UPDATE pcs SET tariff_id = ? WHERE id = ?", *req.TariffID, id)
	}
	if req.Notes != nil {
		db.Exec("UPDATE pcs SET notes = ? WHERE id = ?", *req.Notes, id)
	}
	if req.Comment != nil {
		db.Exec("UPDATE pcs SET comment = ? WHERE id = ?", *req.Comment, id)
	}
	if req.CPU != nil {
		db.Exec("UPDATE pcs SET cpu = ? WHERE id = ?", *req.CPU, id)
	}
	if req.RAM != nil {
		db.Exec("UPDATE pcs SET ram = ? WHERE id = ?", *req.RAM, id)
	}
	if req.GPU != nil {
		db.Exec("UPDATE pcs SET gpu = ? WHERE id = ?", *req.GPU, id)
	}
	if req.Layout != nil {
		db.Exec("UPDATE pcs SET layout = ? WHERE id = ?", *req.Layout, id)
	}
	if req.MAC != nil {
		db.Exec("UPDATE pcs SET mac = ? WHERE id = ?", *req.MAC, id)
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- Режим обслуживания ПК ---

func pcMaintenanceHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Maintenance *bool `json:"maintenance"`
	}
	if err := jsonDecode(r, &req); err != nil || req.Maintenance == nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if *req.Maintenance {
		// нельзя перевести в обслуживание ПК с активной сессией
		var active int
		db.QueryRow("SELECT COUNT(*) FROM sessions WHERE pc_id = ? AND status IN ('active','paused')", id).Scan(&active)
		if active > 0 {
			writeErr(w, 400, "Завершите сессию перед переводом в обслуживание")
			return
		}
	}
	res, err := db.Exec("UPDATE pcs SET maintenance = ? WHERE id = ?", boolToInt(*req.Maintenance), id)
	if err != nil {
		writeErr(w, 500, "Ошибка")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, 404, "ПК не найден")
		return
	}
	if *req.Maintenance {
		logAction(r, "pc_maintenance", "ПК #"+itoa(id)+" переведён в обслуживание")
	} else {
		logAction(r, "pc_maintenance", "ПК #"+itoa(id)+" возвращён из обслуживания")
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- Wake-on-LAN ---

func pcWakeHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var mac string
	if err := db.QueryRow("SELECT mac FROM pcs WHERE id = ?", id).Scan(&mac); err != nil {
		writeErr(w, 404, "ПК не найден")
		return
	}
	if mac == "" {
		writeErr(w, 400, "У ПК не задан MAC-адрес для Wake-on-LAN")
		return
	}
	if err := sendWOL(mac); err != nil {
		writeErr(w, 500, "Ошибка отправки Wake-on-LAN: "+err.Error())
		return
	}
	logAction(r, "pc_wake", "Wake-on-LAN для ПК #"+itoa(id)+" ("+mac+")")
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// pcLayoutHandler — смена позиции ПК на карте (drag-and-drop).
func pcLayoutHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Layout int64 `json:"layout"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	db.Exec("UPDATE pcs SET layout = ? WHERE id = ?", req.Layout, id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- Удаление ПК ---

func pcDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var active int
	db.QueryRow("SELECT COUNT(*) FROM sessions WHERE pc_id = ? AND status IN ('active','paused')", id).Scan(&active)
	if active > 0 {
		writeErr(w, 400, "Нельзя удалить ПК с активной сессией")
		return
	}
	db.Exec("DELETE FROM pcs WHERE id = ?", id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// --- Публичный статус для агента на ПК ---

func agentStatusHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	if !checkPCAgentAuth(r, id) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}

	var p PC
	var isOnline int
	err := db.QueryRow("SELECT id, name, zone, current_session_id, is_online FROM pcs WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Zone, &p.CurrentSessionID, &isOnline)
	if err == sql.ErrNoRows {
		writeErr(w, 404, "ПК не найден")
		return
	}
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	p.IsOnline = isOnline == 1

	status := AgentStatus{
		PCID:             p.ID,
		PCName:           p.Name,
		Zone:             p.Zone,
		ClubName:         getSettings().ClubName,
		Locked:           true,
		ServerTime:       nowUnix(),
		BackgroundVersion: getSettingInt64("background_version", 0),
		Commands:         []PendingCommand{},
		Messages:         []PendingMessage{},
	}

	if p.CurrentSessionID > 0 {
		var endsAt, startedAt, pausedAt, clientID int64
		var sessStatus, clientName string
		var clientBalance float64
		var clientBonus int64
		var open int
		err := db.QueryRow(`
			SELECT s.ends_at, s.started_at, s.paused_at, s.status, s.client_id, COALESCE(c.name,''), COALESCE(c.balance,0), COALESCE(c.bonus_points,0), COALESCE(s.open,0)
			FROM sessions s LEFT JOIN clients c ON c.id = s.client_id
			WHERE s.id = ?`, p.CurrentSessionID).
			Scan(&endsAt, &startedAt, &pausedAt, &sessStatus, &clientID, &clientName, &clientBalance, &clientBonus, &open)
		if err == nil && (sessStatus == SessionActive || sessStatus == SessionPaused) {
			status.ClientName = clientName
			status.ClientID = clientID
			status.ClientBalance = clientBalance
			status.ClientBonus = clientBonus
			status.SessionStartedAt = startedAt
			status.SessionOpen = open == 1
			if sessStatus == SessionActive {
				status.RemainingSeconds = endsAt - nowUnix()
				status.EndsAt = endsAt
				if status.RemainingSeconds > 0 {
					status.Locked = false
				}
			} else {
				// Пауза — остаток заморожен
				status.RemainingSeconds = endsAt - pausedAt
				status.EndsAt = 0
				status.Locked = false
				status.Paused = true
			}
		}
	}

	// heartbeat
	db.Exec("UPDATE pcs SET is_online = 1, last_seen = ? WHERE id = ?", nowUnix(), id)

	// ожидающие команды для этого ПК
	cmdRows, err := db.Query("SELECT id, kind FROM pc_commands WHERE pc_id = ? AND status = 'queued' ORDER BY id", id)
	if err == nil {
		defer cmdRows.Close()
		for cmdRows.Next() {
			var c PendingCommand
			cmdRows.Scan(&c.ID, &c.Kind)
			status.Commands = append(status.Commands, c)
		}
	}

	// сообщения от админа для этого ПК
	msgRows, err := db.Query("SELECT id, message FROM pc_messages WHERE pc_id = ? AND status = 'pending' ORDER BY id", id)
	if err == nil {
		defer msgRows.Close()
		for msgRows.Next() {
			var m PendingMessage
			msgRows.Scan(&m.ID, &m.Message)
			status.Messages = append(status.Messages, m)
		}
	}

	// активные объявления (контент на экранах ПК)
	annRows, err := db.Query("SELECT id, title, text, active, created_at FROM announcements WHERE active = 1 ORDER BY id DESC LIMIT 5")
	if err == nil {
		defer annRows.Close()
		for annRows.Next() {
			var a Announcement
			var act int
			annRows.Scan(&a.ID, &a.Title, &a.Text, &act, &a.CreatedAt)
			a.Active = act == 1
			status.Announcements = append(status.Announcements, a)
		}
	}

	writeJSON(w, 200, status)
}

// --- Команды ПК (закрыть программы / выключить / перезагрузить) ---

func pcCommandHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Kind string `json:"kind"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if !validCommand(req.Kind) {
		writeErr(w, 400, "Неизвестная команда")
		return
	}
	// «выход в Windows» (разблокировка киоска) — только администратор
	if req.Kind == "exit_to_admin" {
		u := currentUser(r)
		if u == nil || roleLevel(u.Role) < roleLevel(RoleAdmin) {
			writeErr(w, 403, "Требуются права администратора")
			return
		}
	}
	var exists int
	db.QueryRow("SELECT COUNT(*) FROM pcs WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		writeErr(w, 404, "ПК не найден")
		return
	}
	// «выход в Windows» — ставим сессию на паузу, чтобы время не шло
	if req.Kind == "exit_to_admin" {
		pauseSessionForPC(id)
	}
	_, err := insertID("INSERT INTO pc_commands (pc_id, kind, created_at) VALUES (?,?,?)", id, req.Kind, nowUnix())
	if err != nil {
		writeErr(w, 500, "Ошибка создания команды")
		return
	}
	logAction(r, "pc_command", "Команда «"+req.Kind+"» на ПК #"+itoa(id))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// pauseSessionForPC ставит активную сессию ПК на паузу (если идёт).
func pauseSessionForPC(pcID int64) {
	var sessID int64
	var status string
	if err := db.QueryRow("SELECT current_session_id FROM pcs WHERE id = ?", pcID).Scan(&sessID); err != nil || sessID <= 0 {
		return
	}
	if err := db.QueryRow("SELECT status FROM sessions WHERE id = ?", sessID).Scan(&status); err != nil || status != SessionActive {
		return
	}
	db.Exec("UPDATE sessions SET paused_at = ?, status = 'paused' WHERE id = ?", nowUnix(), sessID)
}

func pcCommandAllHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind string `json:"kind"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if !validCommand(req.Kind) {
		writeErr(w, 400, "Неизвестная команда")
		return
	}
	if req.Kind == "exit_to_admin" {
		u := currentUser(r)
		if u == nil || roleLevel(u.Role) < roleLevel(RoleAdmin) {
			writeErr(w, 403, "Требуются права администратора")
			return
		}
	}
	rows, err := db.Query("SELECT id FROM pcs")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	var ids []int64
	for rows.Next() {
		var pcID int64
		rows.Scan(&pcID)
		ids = append(ids, pcID)
	}
	rows.Close()

	for _, pcID := range ids {
		if req.Kind == "exit_to_admin" {
			pauseSessionForPC(pcID)
		}
		insertID("INSERT INTO pc_commands (pc_id, kind, created_at) VALUES (?,?,?)", pcID, req.Kind, nowUnix())
	}
	logAction(r, "pc_command_all", "Команда «"+req.Kind+"» на все ПК ("+itoa(int64(len(ids)))+")")
	writeJSON(w, 200, map[string]interface{}{"count": len(ids)})
}

// pcMessageHandler — отправка сообщения на конкретный ПК.
func pcMessageHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := jsonDecode(r, &req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeErr(w, 400, "Введите сообщение")
		return
	}
	var exists int
	db.QueryRow("SELECT COUNT(*) FROM pcs WHERE id = ?", id).Scan(&exists)
	if exists == 0 {
		writeErr(w, 404, "ПК не найден")
		return
	}
	if _, err := insertID("INSERT INTO pc_messages (pc_id, message, created_at) VALUES (?,?,?)", id, req.Message, nowUnix()); err != nil {
		writeErr(w, 500, "Ошибка отправки")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// pcMessageAllHandler — отправка сообщения на все ПК.
func pcMessageAllHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := jsonDecode(r, &req); err != nil || strings.TrimSpace(req.Message) == "" {
		writeErr(w, 400, "Введите сообщение")
		return
	}
	rows, err := db.Query("SELECT id FROM pcs")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	var ids []int64
	for rows.Next() {
		var pcID int64
		rows.Scan(&pcID)
		ids = append(ids, pcID)
	}
	rows.Close()
	for _, pcID := range ids {
		insertID("INSERT INTO pc_messages (pc_id, message, created_at) VALUES (?,?,?)", pcID, req.Message, nowUnix())
	}
	writeJSON(w, 200, map[string]interface{}{"count": len(ids)})
}

// agentMessageAckHandler — агент подтверждает показ сообщения.
func agentMessageAckHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int64 `json:"id"`
	}
	if err := jsonDecode(r, &req); err != nil || req.ID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	db.Exec("UPDATE pc_messages SET status = 'acked' WHERE id = ?", req.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func agentCommandAckHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := jsonDecode(r, &req); err != nil || req.ID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	var pcID int64
	if err := db.QueryRow("SELECT pc_id FROM pc_commands WHERE id = ?", req.ID).Scan(&pcID); err != nil {
		writeErr(w, 404, "Команда не найдена")
		return
	}
	if !checkPCAgentAuth(r, pcID) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	status := req.Status
	if status != "done" && status != "error" {
		status = "done"
	}
	db.Exec("UPDATE pc_commands SET status = ?, done_at = ? WHERE id = ?", status, nowUnix(), req.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func validCommand(kind string) bool {
	return kind == "close_apps" || kind == "shutdown" || kind == "reboot" || kind == "exit_to_admin"
}

// agentComplianceHandler — приём отчёта готовности FACEIT от агента.
func agentComplianceHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCID   int64           `json:"pc_id"`
		Report json.RawMessage `json:"report"`
	}
	if err := jsonDecode(r, &req); err != nil || req.PCID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if !checkPCAgentAuth(r, req.PCID) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	db.Exec("UPDATE pcs SET compliance = ?, last_seen = ? WHERE id = ?", string(req.Report), nowUnix(), req.PCID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// agentHardwareHandler — приём данных о железе и дисках от агента.
func agentHardwareHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PCID  int64  `json:"pc_id"`
		CPU   string `json:"cpu"`
		RAM   string `json:"ram"`
		GPU   string `json:"gpu"`
		Disks []Disk `json:"disks"`
	}
	if err := jsonDecode(r, &req); err != nil || req.PCID <= 0 {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if !checkPCAgentAuth(r, req.PCID) {
		writeErr(w, 401, "Неверный токен агента")
		return
	}
	disksJSON, _ := json.Marshal(req.Disks)
	db.Exec("UPDATE pcs SET cpu = ?, ram = ?, gpu = ?, disks = ?, last_seen = ? WHERE id = ?",
		req.CPU, req.RAM, req.GPU, string(disksJSON), nowUnix(), req.PCID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// parseDisks разбирает JSON-строку дисков в []Disk.
func parseDisks(raw string) []Disk {
	if raw == "" {
		return []Disk{}
	}
	var disks []Disk
	if err := json.Unmarshal([]byte(raw), &disks); err != nil {
		return []Disk{}
	}
	return disks
}

// checkPCAgentAuth проверяет аутентификацию агента конкретного ПК.
// Приоритет — персональный токен ПК (X-PC-Token); затем глобальный AGENT_TOKEN (legacy).
func checkPCAgentAuth(r *http.Request, pcID int64) bool {
	provided := r.Header.Get("X-PC-Token")
	if provided == "" {
		provided = r.Header.Get("Authorization")
		provided = trimBearer(provided)
	}

	// 1) Персональный токен ПК
	if provided != "" {
		var tokenHash string
		err := db.QueryRow("SELECT token_hash FROM pcs WHERE id = ?", pcID).Scan(&tokenHash)
		if err == nil && tokenHash != "" && hashToken(provided) == tokenHash {
			return true
		}
		// 2) Глобальный токен (legacy)
		global := os.Getenv("AGENT_TOKEN")
		if global != "" && provided == global {
			return true
		}
		return false
	}

	// Токен не передан: разрешаем только если аутентификация вообще не настроена
	var tokenHash string
	err := db.QueryRow("SELECT token_hash FROM pcs WHERE id = ?", pcID).Scan(&tokenHash)
	if err != nil {
		return false
	}
	return tokenHash == "" && os.Getenv("AGENT_TOKEN") == ""
}

func trimBearer(s string) string {
	if strings.HasPrefix(s, "Bearer ") {
		return strings.TrimPrefix(s, "Bearer ")
	}
	return s
}

// --- Offline-детекция (в тикере) ---

func tickPCOffline() {
	threshold := nowUnix() - 60 // 60 сек без heartbeat
	db.Exec("UPDATE pcs SET is_online = 0 WHERE is_online = 1 AND last_seen < ? AND last_seen > 0", threshold)
}

// --- Таймер: завершение истёкших сессий ---

func tickSessions() {
	now := nowUnix()
	rows, err := db.Query(`
		SELECT s.id, s.pc_id, COALESCE(p.name,'')
		FROM sessions s LEFT JOIN pcs p ON p.id = s.pc_id
		WHERE s.status = 'active' AND s.ends_at <= ?`, now)
	if err != nil {
		return
	}
	type exp struct {
		id, pcID int64
		pcName   string
	}
	var expired []exp
	for rows.Next() {
		var e exp
		rows.Scan(&e.id, &e.pcID, &e.pcName)
		expired = append(expired, e)
	}
	rows.Close()

	for _, e := range expired {
		endSession(e.id, 0)
		notifyTelegram("⛔ " + e.pcName + ": время сессии истекло.")
	}
}
