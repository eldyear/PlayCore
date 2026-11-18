package main

import "net/http"

func usersListHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, username, full_name, role, active, club_id, created_at FROM users ORDER BY id")
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()
	list := []User{}
	for rows.Next() {
		var u User
		var active int
		rows.Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &active, &u.ClubID, &u.CreatedAt)
		u.Active = active == 1
		list = append(list, u)
	}
	writeJSON(w, 200, map[string]interface{}{"users": list})
}

func userCreateHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		ClubID   int64  `json:"club_id"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, 400, "Укажите логин и пароль")
		return
	}
	me := currentUser(r)
	if me == nil {
		writeErr(w, 401, "Требуется авторизация")
		return
	}
	if !validRole(req.Role) {
		req.Role = RoleCashier
	}
	// можно создавать только роли строго ниже своей (root создаёт владельцев и ниже,
	// владелец — админов и кассиров, админ — только кассиров)
	if roleLevel(req.Role) >= roleLevel(me.Role) {
		writeErr(w, 403, "Недостаточно прав для создания этой роли")
		return
	}
	clubID := me.ClubID
	if me.Role == RoleRoot {
		clubID = req.ClubID
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeErr(w, 500, "Ошибка хеширования")
		return
	}
	id, err := insertID(
		"INSERT INTO users (username, password_hash, full_name, role, active, club_id, created_at) VALUES (?,?,?,?,1,?,?)",
		req.Username, hash, req.FullName, req.Role, clubID, nowUnix(),
	)
	if err != nil {
		writeErr(w, 400, "Логин уже существует")
		return
	}
	logAction(r, "user_create", "Создан пользователь «"+req.Username+"» (роль: "+req.Role+")")
	writeJSON(w, 200, map[string]interface{}{"id": id})
}

func userUpdateHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	var req struct {
		FullName *string `json:"full_name"`
		Role     *string `json:"role"`
		Active   *bool   `json:"active"`
		Password *string `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	me := currentUser(r)
	if me == nil {
		writeErr(w, 401, "Требуется авторизация")
		return
	}
	var targetRole string
	if err := db.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&targetRole); err != nil {
		writeErr(w, 404, "Пользователь не найден")
		return
	}
	// нельзя редактировать пользователя с ролью не ниже своей
	if id != me.ID && roleLevel(targetRole) >= roleLevel(me.Role) {
		writeErr(w, 403, "Недостаточно прав")
		return
	}
	if req.Role != nil && *req.Role != targetRole {
		if !validRole(*req.Role) || roleLevel(*req.Role) >= roleLevel(me.Role) {
			writeErr(w, 403, "Недостаточно прав для этой роли")
			return
		}
	}
	if req.FullName != nil {
		db.Exec("UPDATE users SET full_name = ? WHERE id = ?", *req.FullName, id)
	}
	if req.Role != nil {
		db.Exec("UPDATE users SET role = ? WHERE id = ?", *req.Role, id)
	}
	if req.Active != nil {
		db.Exec("UPDATE users SET active = ? WHERE id = ?", boolToInt(*req.Active), id)
	}
	if req.Password != nil && *req.Password != "" {
		hash, _ := hashPassword(*req.Password)
		db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hash, id)
	}
	logAction(r, "user_update", "Обновлён пользователь #"+itoa(id))
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func userDeleteHandler(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeErr(w, 400, "Некорректный id")
		return
	}
	if id == currentUserID(r) {
		writeErr(w, 400, "Нельзя удалить самого себя")
		return
	}
	me := currentUser(r)
	if me == nil {
		writeErr(w, 401, "Требуется авторизация")
		return
	}
	var targetRole string
	if err := db.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&targetRole); err != nil {
		writeErr(w, 404, "Пользователь не найден")
		return
	}
	if roleLevel(targetRole) >= roleLevel(me.Role) {
		writeErr(w, 403, "Недостаточно прав")
		return
	}
	db.Exec("DELETE FROM users WHERE id = ?", id)
	db.Exec("DELETE FROM auth_sessions WHERE user_id = ?", id)
	logAction(r, "user_delete", "Удалён пользователь #"+itoa(id))
	writeJSON(w, 200, map[string]bool{"ok": true})
}
