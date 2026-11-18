package main

import "net/http"

// agentExitAuthHandler — проверка пароля администратора для «выхода в Windows»
// с клиентского ПК (агента). Проверяет пароль среди активных админов.
func agentExitAuthHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := jsonDecode(r, &req); err != nil || req.Password == "" {
		writeErr(w, 400, "Укажите пароль")
		return
	}

	rows, err := db.Query("SELECT password_hash FROM users WHERE role = ? AND active = 1", RoleAdmin)
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var hash string
		rows.Scan(&hash)
		if checkPassword(hash, req.Password) {
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
	}
	writeErr(w, 401, "Неверный пароль")
}
