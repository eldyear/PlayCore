package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/joho/godotenv"
)

//go:embed templates/index.html
var indexHTML []byte

//go:embed playcorelogo.svg
var logo []byte

func main() {
	initLogFile()
	loadEnv()
	initDB()

	// Начальный администратор и демо-данные
	seedAdmin(os.Getenv("ADMIN_USER"), os.Getenv("ADMIN_PASS"))
	seedTariffs()
	seedSettings()
	seedClub()

	// Фоновые задачи
	go func() {
		offlineTick := 0
		for {
			time.Sleep(1 * time.Second)
			tickSessions()
			offlineTick++
			if offlineTick%15 == 0 {
				tickPCOffline()
			}
			if offlineTick%10 == 0 {
				tickNotifications()
			}
		}
	}()

	port := getPort()
	addr := "0.0.0.0:" + port
	cert := os.Getenv("TLS_CERT")
	key := os.Getenv("TLS_KEY")

	controller := &ServerController{}
	if err := controller.Start(addr, buildMux(), cert, key); err != nil {
		fmt.Printf("Ошибка запуска сервера: %v\n", err)
	}

	runTray(controller, "http://localhost:"+port)
}

func loadEnv() {
	// .env рядом с exe (Windows) и в рабочем каталоге (Docker)
	if exe, err := os.Executable(); err == nil {
		_ = godotenv.Load(filepath.Join(filepath.Dir(exe), ".env"))
	}
	_ = godotenv.Load()
}

// resolvePath превращает относительный путь в путь относительно каталога exe
// (чтобы БД всегда лежала рядом с exe, а не в случайном рабочем каталоге).
func resolvePath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	exe, err := os.Executable()
	if err != nil {
		return path
	}
	return filepath.Join(filepath.Dir(exe), path)
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return port
}

func buildMux() *http.ServeMux {
	mux := http.NewServeMux()

	// --- Публичное ---
	mux.HandleFunc("POST /api/login", loginHandler)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /agent/status/{id}", agentStatusHandler)

	// --- Авторизованное ---
	mux.HandleFunc("GET /api/me", authMiddleware(meHandler))
	mux.HandleFunc("POST /api/logout", authMiddleware(logoutHandler))

	// ПК
	mux.HandleFunc("GET /api/pcs", authMiddleware(pcsListHandler))
	mux.HandleFunc("POST /api/pcs", adminMiddleware(pcCreateHandler))
	mux.HandleFunc("PUT /api/pcs/{id}", adminMiddleware(pcUpdateHandler))
	mux.HandleFunc("DELETE /api/pcs/{id}", adminMiddleware(pcDeleteHandler))
	mux.HandleFunc("POST /api/pcs/{id}/rotate-token", adminMiddleware(pcRotateTokenHandler))
	mux.HandleFunc("POST /api/pcs/{id}/command", authMiddleware(pcCommandHandler))
	mux.HandleFunc("POST /api/pcs/command-all", authMiddleware(pcCommandAllHandler))
	mux.HandleFunc("POST /api/pcs/{id}/message", authMiddleware(pcMessageHandler))
	mux.HandleFunc("POST /api/pcs/message-all", authMiddleware(pcMessageAllHandler))
	mux.HandleFunc("PUT /api/pcs/{id}/layout", authMiddleware(pcLayoutHandler))
	mux.HandleFunc("POST /api/pcs/{id}/maintenance", adminMiddleware(pcMaintenanceHandler))
	mux.HandleFunc("POST /api/pcs/{id}/wake", authMiddleware(pcWakeHandler))
	mux.HandleFunc("POST /api/agent/command-ack", agentCommandAckHandler)
	mux.HandleFunc("POST /api/agent/message-ack", agentMessageAckHandler)
	mux.HandleFunc("POST /api/agent/compliance", agentComplianceHandler)
	mux.HandleFunc("POST /api/agent/hardware", agentHardwareHandler)
	mux.HandleFunc("POST /api/agent/client-login", agentClientLoginHandler)
	mux.HandleFunc("POST /api/agent/client-register", agentClientRegisterHandler)
	mux.HandleFunc("POST /api/agent/client-logout", agentClientLogoutHandler)
	mux.HandleFunc("POST /api/agent/call", agentCallHandler)
	mux.HandleFunc("POST /api/agent/buy", agentBuyHandler)
	mux.HandleFunc("GET /agent/products/{id}", agentProductsHandler)

	// Вызовы администратора (админка)
	mux.HandleFunc("GET /api/calls", authMiddleware(callsListHandler))
	mux.HandleFunc("POST /api/calls/{id}/resolve", authMiddleware(callResolveHandler))

	// Отзывы клиентов
	mux.HandleFunc("GET /api/reviews", authMiddleware(reviewsListHandler))
	mux.HandleFunc("POST /api/reviews", authMiddleware(reviewCreateHandler))
	mux.HandleFunc("DELETE /api/reviews/{id}", adminMiddleware(reviewDeleteHandler))

	// Объявления (контент на экранах ПК)
	mux.HandleFunc("GET /api/announcements", authMiddleware(announcementsListHandler))
	mux.HandleFunc("POST /api/announcements", adminMiddleware(announcementCreateHandler))
	mux.HandleFunc("PUT /api/announcements/{id}", adminMiddleware(announcementUpdateHandler))
	mux.HandleFunc("DELETE /api/announcements/{id}", adminMiddleware(announcementDeleteHandler))

	// Бронирование
	mux.HandleFunc("GET /api/bookings", authMiddleware(bookingsListHandler))
	mux.HandleFunc("POST /api/bookings", authMiddleware(bookingCreateHandler))
	mux.HandleFunc("PUT /api/bookings/{id}", authMiddleware(bookingUpdateHandler))
	mux.HandleFunc("DELETE /api/bookings/{id}", authMiddleware(bookingDeleteHandler))

	// Сессии
	mux.HandleFunc("POST /api/sessions", authMiddleware(sessionStartHandler))
	mux.HandleFunc("POST /api/sessions/bulk", authMiddleware(sessionBulkStartHandler))
	mux.HandleFunc("POST /api/sessions/{id}/add-time", authMiddleware(sessionAddTimeHandler))
	mux.HandleFunc("POST /api/sessions/{id}/pause", authMiddleware(sessionPauseHandler))
	mux.HandleFunc("POST /api/sessions/{id}/resume", authMiddleware(sessionResumeHandler))
	mux.HandleFunc("POST /api/sessions/{id}/stop", authMiddleware(sessionStopHandler))
	mux.HandleFunc("GET /api/sessions/active", authMiddleware(sessionsActiveHandler))
	mux.HandleFunc("GET /api/sessions/history", authMiddleware(sessionsHistoryHandler))

	// Тарифы
	mux.HandleFunc("GET /api/tariffs", authMiddleware(tariffsListHandler))
	mux.HandleFunc("POST /api/tariffs", adminMiddleware(tariffCreateHandler))
	mux.HandleFunc("PUT /api/tariffs/{id}", adminMiddleware(tariffUpdateHandler))
	mux.HandleFunc("DELETE /api/tariffs/{id}", adminMiddleware(tariffDeleteHandler))

	// Клиенты
	mux.HandleFunc("GET /api/clients", authMiddleware(clientsListHandler))
	mux.HandleFunc("POST /api/clients", adminMiddleware(clientCreateHandler))
	mux.HandleFunc("PUT /api/clients/{id}", adminMiddleware(clientUpdateHandler))
	mux.HandleFunc("DELETE /api/clients/{id}", adminMiddleware(clientDeleteHandler))
	mux.HandleFunc("POST /api/clients/{id}/deposit", authMiddleware(clientDepositHandler))
	mux.HandleFunc("POST /api/clients/{id}/use-bonus", authMiddleware(clientUseBonusHandler))

	// Смены
	mux.HandleFunc("POST /api/shifts/open", authMiddleware(shiftOpenHandler))
	mux.HandleFunc("POST /api/shifts/close", authMiddleware(shiftCloseHandler))
	mux.HandleFunc("GET /api/shifts/current", authMiddleware(shiftCurrentHandler))
	mux.HandleFunc("GET /api/shifts/history", authMiddleware(shiftsHistoryHandler))

	// Отчёты
	mux.HandleFunc("GET /api/reports/transactions", authMiddleware(reportTransactionsHandler))

	// Настройки
	mux.HandleFunc("GET /api/settings", authMiddleware(settingsGetHandler))
	mux.HandleFunc("PUT /api/settings", ownerMiddleware(settingsPutHandler))

	// Фоновое изображение
	mux.HandleFunc("POST /api/background", ownerMiddleware(backgroundUploadHandler))
	mux.HandleFunc("GET /api/background", backgroundGetHandler)

	// Кастомные обложки (для не-Steam ярлыков)
	mux.HandleFunc("POST /api/covers", ownerMiddleware(coversUploadHandler))
	mux.HandleFunc("GET /api/covers", authMiddleware(coversListHandler))
	mux.HandleFunc("DELETE /api/covers/{name}", ownerMiddleware(coversDeleteHandler))
	mux.HandleFunc("GET /api/covers/{name}", coversGetHandler)

	// Товары / склад
	mux.HandleFunc("GET /api/products", authMiddleware(productsListHandler))
	mux.HandleFunc("POST /api/products", adminMiddleware(productCreateHandler))
	mux.HandleFunc("PUT /api/products/{id}", adminMiddleware(productUpdateHandler))
	mux.HandleFunc("DELETE /api/products/{id}", adminMiddleware(productDeleteHandler))
	mux.HandleFunc("POST /api/products/{id}/sell", authMiddleware(productSellHandler))
	mux.HandleFunc("POST /api/products/{id}/restock", adminMiddleware(productRestockHandler))
	mux.HandleFunc("GET /api/sales", authMiddleware(salesListHandler))

	// Telegram-тест
	mux.HandleFunc("POST /api/telegram/test", ownerMiddleware(telegramTestHandler))

	// Журнал действий (владелец + root)
	mux.HandleFunc("GET /api/audit", ownerMiddleware(auditLogListHandler))

	// Клубы и подписки (root)
	mux.HandleFunc("GET /api/clubs", rootMiddleware(clubsListHandler))
	mux.HandleFunc("POST /api/clubs", rootMiddleware(clubCreateHandler))
	mux.HandleFunc("POST /api/subscriptions", rootMiddleware(subscriptionUpdateHandler))

	// Выход в Windows с агента (проверка пароля админа)
	mux.HandleFunc("POST /api/agent/exit-auth", agentExitAuthHandler)

	// Пользователи (только админ)
	mux.HandleFunc("GET /api/users", adminMiddleware(usersListHandler))
	mux.HandleFunc("POST /api/users", adminMiddleware(userCreateHandler))
	mux.HandleFunc("PUT /api/users/{id}", adminMiddleware(userUpdateHandler))
	mux.HandleFunc("DELETE /api/users/{id}", adminMiddleware(userDeleteHandler))

	// --- Фронтенд ---
	mux.HandleFunc("GET /playcorelogo.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "max-age=86400")
		w.Write(logo)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	return mux
}
