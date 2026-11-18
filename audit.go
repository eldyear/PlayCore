package main

import "net/http"

// logAction записывает действие пользователя в журнал (без ошибок — тихо).
func logAction(r *http.Request, action, detail string) {
	u := currentUser(r)
	uid := int64(0)
	username := ""
	role := ""
	clubID := int64(0)
	if u != nil {
		uid = u.ID
		username = u.Username
		role = u.Role
		clubID = u.ClubID
	}
	db.Exec("INSERT INTO audit_log (user_id, username, role, club_id, action, detail, created_at) VALUES (?,?,?,?,?,?,?)",
		uid, username, role, clubID, action, detail, nowUnix())
}

// auditLogListHandler — журнал действий (владелец+root).
func auditLogListHandler(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeErr(w, 401, "Требуется авторизация")
		return
	}
	q := `SELECT id, user_id, username, role, club_id, action, detail, created_at FROM audit_log`
	args := []interface{}{}
	// root видит всё; владелец видит свой клуб + глобальные записи
	if u.Role != RoleRoot {
		q += " WHERE club_id = ? OR club_id = 0"
		args = append(args, u.ClubID)
	}
	q += " ORDER BY id DESC LIMIT 300"

	rows, err := db.Query(q, args...)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()

	list := []map[string]interface{}{}
	for rows.Next() {
		var id, userID, clubID, createdAt int64
		var username, role, action, detail string
		if err := rows.Scan(&id, &userID, &username, &role, &clubID, &action, &detail, &createdAt); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id": id, "user_id": userID, "username": username, "role": role,
			"club_id": clubID, "action": action, "detail": detail, "created_at": createdAt,
		})
	}
	writeJSON(w, 200, map[string]interface{}{"log": list})
}
