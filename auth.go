package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName = "el_session"
	sessionTTL = 12 * time.Hour
)

type ctxKey string

const contextUserID ctxKey = "user_id"

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// --- Логин / логаут ---

func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "Некорректный запрос")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, 400, "Введите логин и пароль")
		return
	}

	var user User
	var hash string
	var active int
	err := db.QueryRow(
		"SELECT id, username, full_name, role, active, password_hash, club_id, created_at FROM users WHERE username = ?",
		req.Username,
	).Scan(&user.ID, &user.Username, &user.FullName, &user.Role, &active, &hash, &user.ClubID, &user.CreatedAt)
	if err == sql.ErrNoRows {
		writeErr(w, 401, "Неверный логин или пароль")
		return
	}
	if err != nil {
		writeErr(w, 500, "Ошибка БД")
		return
	}
	user.Active = active == 1
	if !user.Active {
		writeErr(w, 403, "Пользователь заблокирован")
		return
	}
	if !checkPassword(hash, req.Password) {
		writeErr(w, 401, "Неверный логин или пароль")
		return
	}

	token := randomToken()
	expires := nowUnix() + int64(sessionTTL.Seconds())
	if _, err := db.Exec(
		"INSERT INTO auth_sessions (token, user_id, created_at, expires_at) VALUES (?,?,?,?)",
		token, user.ID, nowUnix(), expires,
	); err != nil {
		writeErr(w, 500, "Ошибка создания сессии")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})

	writeJSON(w, 200, map[string]interface{}{"user": user})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		db.Exec("DELETE FROM auth_sessions WHERE token = ?", c.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:   cookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if user == nil {
		writeErr(w, 401, "Требуется авторизация")
		return
	}
	writeJSON(w, 200, map[string]interface{}{"user": user})
}

// --- Middleware ---

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeErr(w, 401, "Требуется авторизация")
			return
		}
		var userID int64
		var expires int64
		err = db.QueryRow("SELECT user_id, expires_at FROM auth_sessions WHERE token = ?", c.Value).Scan(&userID, &expires)
		if err == sql.ErrNoRows || nowUnix() > expires {
			writeErr(w, 401, "Сессия истекла")
			return
		}
		if err != nil {
			writeErr(w, 500, "Ошибка БД")
			return
		}
		ctx := context.WithValue(r.Context(), contextUserID, userID)
		next(w, r.WithContext(ctx))
	}
}

func adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil || roleLevel(u.Role) < roleLevel(RoleAdmin) {
			writeErr(w, 403, "Требуются права администратора")
			return
		}
		next(w, r)
	})
}

func ownerMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil || roleLevel(u.Role) < roleLevel(RoleOwner) {
			writeErr(w, 403, "Требуются права владельца")
			return
		}
		next(w, r)
	})
}

func rootMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil || roleLevel(u.Role) < roleLevel(RoleRoot) {
			writeErr(w, 403, "Требуются права root")
			return
		}
		next(w, r)
	})
}

func currentUser(r *http.Request) *User {
	v := r.Context().Value(contextUserID)
	if v == nil {
		return nil
	}
	userID, ok := v.(int64)
	if !ok {
		return nil
	}
	var u User
	var active int
	err := db.QueryRow(
		"SELECT id, username, full_name, role, active, club_id, created_at FROM users WHERE id = ?",
		userID,
	).Scan(&u.ID, &u.Username, &u.FullName, &u.Role, &active, &u.ClubID, &u.CreatedAt)
	if err != nil {
		return nil
	}
	u.Active = active == 1
	return &u
}

func currentUserID(r *http.Request) int64 {
	v := r.Context().Value(contextUserID)
	if v == nil {
		return 0
	}
	id, _ := v.(int64)
	return id
}
