package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// DB — обёртка над *sql.DB с перезаписью плейсхолдеров под драйвер.
// Все запросы в коде пишутся с плейсхолдерами `?`; для PostgreSQL они
// автоматически преобразуются в $1, $2, ...
type DB struct {
	*sql.DB
	driver string
}

var db *DB

func (d *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(rebind(d.driver, query), args...)
}

func (d *DB) QueryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(rebind(d.driver, query), args...)
}

func (d *DB) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(rebind(d.driver, query), args...)
}

func rebind(driver, query string) string {
	if driver != "postgres" {
		return query
	}
	var b strings.Builder
	n := 0
	for _, c := range query {
		if c == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// insertID выполняет INSERT и возвращает автоинкрементный id.
func insertID(query string, args ...any) (int64, error) {
	if db.driver == "postgres" {
		var id int64
		err := db.DB.QueryRow(rebind("postgres", query+" RETURNING id"), args...).Scan(&id)
		return id, err
	}
	res, err := db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func initDB() {
	driver := strings.ToLower(os.Getenv("DB_DRIVER"))
	if driver == "" {
		driver = "sqlite"
	}

	var raw *sql.DB
	var err error

	switch driver {
	case "postgres":
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			log.Fatal("DB_DRIVER=postgres, но не задан DATABASE_URL")
		}
		raw, err = sql.Open("postgres", dsn)
		if err != nil {
			log.Fatal("Ошибка подключения PostgreSQL:", err)
		}
		db = &DB{raw, driver}
		db.SetMaxOpenConns(20)
		db.SetMaxIdleConns(5)
		// ждём готовности БД
		for i := 0; i < 30; i++ {
			if perr := db.Ping(); perr == nil {
				break
			}
			if i == 29 {
				log.Fatal("PostgreSQL недоступен:", err)
			}
			time.Sleep(time.Second)
		}
	default:
		path := os.Getenv("DB_PATH")
		if path == "" {
			path = "./data/readyboard.db"
		}
		path = resolvePath(path)
		// создаём каталог БД, если его нет
		if dir := filepath.Dir(path); dir != "." && dir != "" {
			_ = os.MkdirAll(dir, 0755)
		}
		raw, err = sql.Open("sqlite", path)
		if err != nil {
			log.Fatal("Ошибка открытия БД:", err)
		}
		db = &DB{raw, driver}
		db.SetMaxOpenConns(1)
		// прагмы SQLite (конкурентность, WAL, внешние ключи)
		db.Exec("PRAGMA busy_timeout = 5000")
		db.Exec("PRAGMA journal_mode = WAL")
		db.Exec("PRAGMA foreign_keys = ON")
	}

	for _, q := range schema(driver) {
		if _, err := db.Exec(q); err != nil {
			log.Fatalf("Ошибка миграции [%s]: %v", driver, err)
		}
	}

	// Догрузка новых колонок для уже существующих БД
	ensureColumn("pcs", "token_hash", "TEXT NOT NULL DEFAULT ''")
	ensureColumn("sessions", "low_notified", "INTEGER NOT NULL DEFAULT 0")
	ensureColumn("pcs", "compliance", "TEXT NOT NULL DEFAULT ''")

	// Тариф: цена за час; сессия: накопленные паузы и итоговая сумма «по факту»
	numType := "REAL"
	secType := "INTEGER"
	if driver == "postgres" {
		numType = "DOUBLE PRECISION"
		secType = "BIGINT"
	}
	ensureColumn("tariffs", "price_per_hour", numType+" NOT NULL DEFAULT 0")
	ensureColumn("sessions", "paused_total", secType+" NOT NULL DEFAULT 0")
	ensureColumn("sessions", "final_seconds", secType+" NOT NULL DEFAULT 0")
	ensureColumn("sessions", "final_amount", numType+" NOT NULL DEFAULT 0")

	// Карта клуба: позиция ПК + заметка + характеристики железа
	ensureColumn("pcs", "layout", "INTEGER NOT NULL DEFAULT 0")
	ensureColumn("pcs", "comment", "TEXT NOT NULL DEFAULT ''")
	ensureColumn("pcs", "cpu", "TEXT NOT NULL DEFAULT ''")
	ensureColumn("pcs", "ram", "TEXT NOT NULL DEFAULT ''")
	ensureColumn("pcs", "gpu", "TEXT NOT NULL DEFAULT ''")

	// Роли и мульти-клуб: клуб пользователя (0 = root/глобально)
	ensureColumn("users", "club_id", "INTEGER NOT NULL DEFAULT 0")

	// Обслуживание и Wake-on-LAN
	ensureColumn("pcs", "maintenance", "INTEGER NOT NULL DEFAULT 0")
	ensureColumn("pcs", "mac", "TEXT NOT NULL DEFAULT ''")
	// Диски и железо (от агента)
	ensureColumn("pcs", "disks", "TEXT NOT NULL DEFAULT ''")
	// Пароль клиента (вход в личный кабинет)
	ensureColumn("clients", "password_hash", "TEXT NOT NULL DEFAULT ''")
	// Открытый тариф (оплата по завершении)
	ensureColumn("sessions", "open", "INTEGER NOT NULL DEFAULT 0")

	// Таблица броней
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS bookings (
			id BIGSERIAL PRIMARY KEY,
			pc_id BIGINT NOT NULL,
			client TEXT NOT NULL DEFAULT '',
			phone TEXT NOT NULL DEFAULT '',
			start_at BIGINT NOT NULL DEFAULT 0,
			end_at BIGINT NOT NULL DEFAULT 0,
			kind TEXT NOT NULL DEFAULT 'blue',
			status TEXT NOT NULL DEFAULT 'active',
			created_at BIGINT NOT NULL DEFAULT 0
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS bookings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pc_id INTEGER NOT NULL,
			client TEXT NOT NULL DEFAULT '',
			phone TEXT NOT NULL DEFAULT '',
			start_at INTEGER NOT NULL DEFAULT 0,
			end_at INTEGER NOT NULL DEFAULT 0,
			kind TEXT NOT NULL DEFAULT 'blue',
			status TEXT NOT NULL DEFAULT 'active',
			created_at INTEGER NOT NULL DEFAULT 0
		)`)
	}

	// Таблица кастомных обложек (обложки для не-Steam ярлыков)
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS covers (
			name TEXT PRIMARY KEY,
			content_type TEXT NOT NULL DEFAULT 'image/jpeg',
			data BYTEA NOT NULL,
			updated_at BIGINT NOT NULL DEFAULT 0
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS covers (
			name TEXT PRIMARY KEY,
			content_type TEXT NOT NULL DEFAULT 'image/jpeg',
			data BLOB NOT NULL,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`)
	}

	// Таблица сообщений админа для ПК
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS pc_messages (
			id BIGSERIAL PRIMARY KEY,
			pc_id BIGINT NOT NULL,
			message TEXT NOT NULL,
			created_at BIGINT NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending'
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS pc_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pc_id INTEGER NOT NULL,
			message TEXT NOT NULL,
			created_at INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending'
		)`)
	}

	// Таблица клубов (для владельцев / сети клубов)
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS clubs (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT 0
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS clubs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		)`)
	}

	// Таблица подписок (root управляет)
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS subscriptions (
			id BIGSERIAL PRIMARY KEY,
			club_id BIGINT NOT NULL DEFAULT 0,
			plan TEXT NOT NULL DEFAULT 'free',
			status TEXT NOT NULL DEFAULT 'active',
			expires_at BIGINT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL DEFAULT 0
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			club_id INTEGER NOT NULL DEFAULT 0,
			plan TEXT NOT NULL DEFAULT 'free',
			status TEXT NOT NULL DEFAULT 'active',
			expires_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT 0
		)`)
	}

	// Журнал действий (кто что делал в клубе)
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS audit_log (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL DEFAULT 0,
			username TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			club_id BIGINT NOT NULL DEFAULT 0,
			action TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT 0
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL DEFAULT 0,
			username TEXT NOT NULL DEFAULT '',
			role TEXT NOT NULL DEFAULT '',
			club_id INTEGER NOT NULL DEFAULT 0,
			action TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		)`)
	}
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at)`)

	// Таблица отзывов клиентов
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS reviews (
			id BIGSERIAL PRIMARY KEY,
			client_name TEXT NOT NULL DEFAULT '',
			rating INTEGER NOT NULL DEFAULT 5,
			comment TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL DEFAULT 0
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS reviews (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			client_name TEXT NOT NULL DEFAULT '',
			rating INTEGER NOT NULL DEFAULT 5,
			comment TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0
		)`)
	}

	// Таблица объявлений (контент на экранах ПК)
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS announcements (
			id BIGSERIAL PRIMARY KEY,
			title TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL DEFAULT '',
			active INTEGER NOT NULL DEFAULT 1,
			created_at BIGINT NOT NULL DEFAULT 0
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS announcements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL DEFAULT '',
			active INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL DEFAULT 0
		)`)
	}

	// Таблица вызовов администратора (с лаунчера)
	if driver == "postgres" {
		db.Exec(`CREATE TABLE IF NOT EXISTS calls (
			id BIGSERIAL PRIMARY KEY,
			pc_id BIGINT NOT NULL DEFAULT 0,
			pc_name TEXT NOT NULL DEFAULT '',
			client_name TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			created_at BIGINT NOT NULL DEFAULT 0
		)`)
	} else {
		db.Exec(`CREATE TABLE IF NOT EXISTS calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pc_id INTEGER NOT NULL DEFAULT 0,
			pc_name TEXT NOT NULL DEFAULT '',
			client_name TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			created_at INTEGER NOT NULL DEFAULT 0
		)`)
	}

	fmt.Printf("🗄️  База данных: %s\n", driver)
}

func schema(driver string) []string {
	if driver == "postgres" {
		return pgSchema
	}
	return sqliteSchema
}

// --- SQLite-схема ---

var sqliteSchema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		full_name TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'cashier',
		active INTEGER NOT NULL DEFAULT 1,
		created_at INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS auth_sessions (
		token TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS pcs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		zone TEXT NOT NULL DEFAULT 'Основной зал',
		tariff_id INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'free',
		current_session_id INTEGER NOT NULL DEFAULT 0,
		is_online INTEGER NOT NULL DEFAULT 0,
		last_seen INTEGER NOT NULL DEFAULT 0,
		notes TEXT NOT NULL DEFAULT '',
		token_hash TEXT NOT NULL DEFAULT '',
		compliance TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS tariffs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		kind TEXT NOT NULL DEFAULT 'hourly',
		price_per_minute REAL NOT NULL DEFAULT 0,
		duration_minutes INTEGER NOT NULL DEFAULT 0,
		package_price REAL NOT NULL DEFAULT 0,
		description TEXT NOT NULL DEFAULT '',
		active INTEGER NOT NULL DEFAULT 1
	);`,
	`CREATE TABLE IF NOT EXISTS clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		phone TEXT NOT NULL DEFAULT '',
		balance REAL NOT NULL DEFAULT 0,
		bonus_points INTEGER NOT NULL DEFAULT 0,
		discount_percent REAL NOT NULL DEFAULT 0,
		notes TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pc_id INTEGER NOT NULL,
		client_id INTEGER NOT NULL DEFAULT 0,
		tariff_id INTEGER NOT NULL DEFAULT 0,
		user_id INTEGER NOT NULL DEFAULT 0,
		shift_id INTEGER NOT NULL DEFAULT 0,
		started_at INTEGER NOT NULL DEFAULT 0,
		ends_at INTEGER NOT NULL DEFAULT 0,
		paused_at INTEGER NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		total_seconds INTEGER NOT NULL DEFAULT 0,
		total_amount REAL NOT NULL DEFAULT 0,
		ended_at INTEGER NOT NULL DEFAULT 0,
		ended_by INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT '',
		low_notified INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS shifts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		opened_at INTEGER NOT NULL DEFAULT 0,
		closed_at INTEGER NOT NULL DEFAULT 0,
		opening_balance REAL NOT NULL DEFAULT 0,
		closing_balance REAL NOT NULL DEFAULT 0,
		total_sales REAL NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'open'
	);`,
	`CREATE TABLE IF NOT EXISTS transactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		kind TEXT NOT NULL,
		session_id INTEGER NOT NULL DEFAULT 0,
		client_id INTEGER NOT NULL DEFAULT 0,
		user_id INTEGER NOT NULL DEFAULT 0,
		shift_id INTEGER NOT NULL DEFAULT 0,
		pc_id INTEGER NOT NULL DEFAULT 0,
		amount REAL NOT NULL DEFAULT 0,
		seconds INTEGER NOT NULL DEFAULT 0,
		method TEXT NOT NULL DEFAULT 'cash',
		created_at INTEGER NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS products (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		category TEXT NOT NULL DEFAULT '',
		price REAL NOT NULL DEFAULT 0,
		stock INTEGER NOT NULL DEFAULT 0,
		active INTEGER NOT NULL DEFAULT 1
	);`,
	`CREATE TABLE IF NOT EXISTS pc_commands (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pc_id INTEGER NOT NULL,
		kind TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued',
		created_at INTEGER NOT NULL DEFAULT 0,
		done_at INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS sales (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		product_id INTEGER NOT NULL,
		product_name TEXT NOT NULL,
		qty INTEGER NOT NULL DEFAULT 1,
		unit_price REAL NOT NULL DEFAULT 0,
		amount REAL NOT NULL DEFAULT 0,
		user_id INTEGER NOT NULL DEFAULT 0,
		shift_id INTEGER NOT NULL DEFAULT 0,
		method TEXT NOT NULL DEFAULT 'cash',
		created_at INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_pc ON sessions(pc_id, status);`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);`,
	`CREATE INDEX IF NOT EXISTS idx_transactions_created ON transactions(created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_transactions_shift ON transactions(shift_id);`,
	`CREATE INDEX IF NOT EXISTS idx_sales_shift ON sales(shift_id);`,
	`CREATE INDEX IF NOT EXISTS idx_sales_created ON sales(created_at);`,
}

// --- PostgreSQL-схема ---

var pgSchema = []string{
	`CREATE TABLE IF NOT EXISTS users (
		id BIGSERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		full_name TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL DEFAULT 'cashier',
		active INTEGER NOT NULL DEFAULT 1,
		created_at BIGINT NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS auth_sessions (
		token TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL,
		created_at BIGINT NOT NULL,
		expires_at BIGINT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS pcs (
		id BIGSERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		zone TEXT NOT NULL DEFAULT 'Основной зал',
		tariff_id BIGINT NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'free',
		current_session_id BIGINT NOT NULL DEFAULT 0,
		is_online INTEGER NOT NULL DEFAULT 0,
		last_seen BIGINT NOT NULL DEFAULT 0,
		notes TEXT NOT NULL DEFAULT '',
		token_hash TEXT NOT NULL DEFAULT '',
		compliance TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS tariffs (
		id BIGSERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		kind TEXT NOT NULL DEFAULT 'hourly',
		price_per_minute DOUBLE PRECISION NOT NULL DEFAULT 0,
		duration_minutes BIGINT NOT NULL DEFAULT 0,
		package_price DOUBLE PRECISION NOT NULL DEFAULT 0,
		description TEXT NOT NULL DEFAULT '',
		active INTEGER NOT NULL DEFAULT 1
	);`,
	`CREATE TABLE IF NOT EXISTS clients (
		id BIGSERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		phone TEXT NOT NULL DEFAULT '',
		balance DOUBLE PRECISION NOT NULL DEFAULT 0,
		bonus_points BIGINT NOT NULL DEFAULT 0,
		discount_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
		notes TEXT NOT NULL DEFAULT '',
		created_at BIGINT NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id BIGSERIAL PRIMARY KEY,
		pc_id BIGINT NOT NULL,
		client_id BIGINT NOT NULL DEFAULT 0,
		tariff_id BIGINT NOT NULL DEFAULT 0,
		user_id BIGINT NOT NULL DEFAULT 0,
		shift_id BIGINT NOT NULL DEFAULT 0,
		started_at BIGINT NOT NULL DEFAULT 0,
		ends_at BIGINT NOT NULL DEFAULT 0,
		paused_at BIGINT NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		total_seconds BIGINT NOT NULL DEFAULT 0,
		total_amount DOUBLE PRECISION NOT NULL DEFAULT 0,
		ended_at BIGINT NOT NULL DEFAULT 0,
		ended_by BIGINT NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT '',
		low_notified INTEGER NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS shifts (
		id BIGSERIAL PRIMARY KEY,
		user_id BIGINT NOT NULL,
		opened_at BIGINT NOT NULL DEFAULT 0,
		closed_at BIGINT NOT NULL DEFAULT 0,
		opening_balance DOUBLE PRECISION NOT NULL DEFAULT 0,
		closing_balance DOUBLE PRECISION NOT NULL DEFAULT 0,
		total_sales DOUBLE PRECISION NOT NULL DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'open'
	);`,
	`CREATE TABLE IF NOT EXISTS transactions (
		id BIGSERIAL PRIMARY KEY,
		kind TEXT NOT NULL,
		session_id BIGINT NOT NULL DEFAULT 0,
		client_id BIGINT NOT NULL DEFAULT 0,
		user_id BIGINT NOT NULL DEFAULT 0,
		shift_id BIGINT NOT NULL DEFAULT 0,
		pc_id BIGINT NOT NULL DEFAULT 0,
		amount DOUBLE PRECISION NOT NULL DEFAULT 0,
		seconds BIGINT NOT NULL DEFAULT 0,
		method TEXT NOT NULL DEFAULT 'cash',
		created_at BIGINT NOT NULL DEFAULT 0,
		note TEXT NOT NULL DEFAULT ''
	);`,
	`CREATE TABLE IF NOT EXISTS products (
		id BIGSERIAL PRIMARY KEY,
		name TEXT NOT NULL,
		category TEXT NOT NULL DEFAULT '',
		price DOUBLE PRECISION NOT NULL DEFAULT 0,
		stock BIGINT NOT NULL DEFAULT 0,
		active INTEGER NOT NULL DEFAULT 1
	);`,
	`CREATE TABLE IF NOT EXISTS pc_commands (
		id BIGSERIAL PRIMARY KEY,
		pc_id BIGINT NOT NULL,
		kind TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued',
		created_at BIGINT NOT NULL DEFAULT 0,
		done_at BIGINT NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS sales (
		id BIGSERIAL PRIMARY KEY,
		product_id BIGINT NOT NULL,
		product_name TEXT NOT NULL,
		qty BIGINT NOT NULL DEFAULT 1,
		unit_price DOUBLE PRECISION NOT NULL DEFAULT 0,
		amount DOUBLE PRECISION NOT NULL DEFAULT 0,
		user_id BIGINT NOT NULL DEFAULT 0,
		shift_id BIGINT NOT NULL DEFAULT 0,
		method TEXT NOT NULL DEFAULT 'cash',
		created_at BIGINT NOT NULL DEFAULT 0
	);`,
	`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_pc ON sessions(pc_id, status);`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);`,
	`CREATE INDEX IF NOT EXISTS idx_transactions_created ON transactions(created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_transactions_shift ON transactions(shift_id);`,
	`CREATE INDEX IF NOT EXISTS idx_sales_shift ON sales(shift_id);`,
	`CREATE INDEX IF NOT EXISTS idx_sales_created ON sales(created_at);`,
}

// ensureColumn добавляет колонку, если её ещё нет (для старых БД).
func ensureColumn(table, column, ddl string) {
	if columnExists(table, column) {
		return
	}
	db.DB.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + ddl)
}

func columnExists(table, column string) bool {
	if db.driver == "postgres" {
		var c int
		err := db.DB.QueryRow(
			"SELECT COUNT(*) FROM information_schema.columns WHERE table_name = $1 AND column_name = $2",
			table, column,
		).Scan(&c)
		return err == nil && c > 0
	}
	rows, err := db.DB.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk)
		if name == column {
			return true
		}
	}
	return false
}

func nowUnix() int64 {
	return time.Now().Unix()
}

func seedAdmin(username, password string) {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		ensureRoot(username)
		return
	}
	if username == "" {
		username = "admin"
	}
	if password == "" {
		password = "admin"
	}
	hash, err := hashPassword(password)
	if err != nil {
		log.Fatal("Ошибка хеширования пароля:", err)
	}
	if _, err := db.Exec(
		"INSERT INTO users (username, password_hash, full_name, role, active, club_id, created_at) VALUES (?,?,?,?,?,?,?)",
		username, hash, "Root", RoleRoot, 1, 0, nowUnix(),
	); err != nil {
		log.Fatal("Ошибка создания root:", err)
	}
	fmt.Printf("👤 Создан root: %s (пароль из ADMIN_PASS)\n", username)
}

// ensureRoot — если в существующей БД нет root, продвигаем первого админа (или ADMIN_USER).
func ensureRoot(username string) {
	var rootCount int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE role = ?", RoleRoot).Scan(&rootCount)
	if rootCount > 0 {
		return
	}
	if username != "" {
		db.Exec("UPDATE users SET role = ? WHERE role = ? AND username = ?", RoleRoot, RoleAdmin, username)
	}
	db.QueryRow("SELECT COUNT(*) FROM users WHERE role = ?", RoleRoot).Scan(&rootCount)
	if rootCount > 0 {
		return
	}
	db.Exec("UPDATE users SET role = ? WHERE id = (SELECT id FROM users WHERE role = ? ORDER BY id LIMIT 1)", RoleRoot, RoleAdmin)
}

// seedClub — создаёт клуб по умолчанию и подписку к нему.
func seedClub() {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM clubs").Scan(&count)
	if count == 0 {
		name := os.Getenv("CLUB_NAME")
		if name == "" {
			name = "Основной клуб"
		}
		id, err := insertID("INSERT INTO clubs (name, created_at) VALUES (?,?)", name, nowUnix())
		if err == nil {
			db.Exec("INSERT INTO subscriptions (club_id, plan, status, expires_at, created_at) VALUES (?,?,?,?,?)",
				id, "business", "active", 0, nowUnix())
		}
	}
}

func seedTariffs() {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM tariffs").Scan(&count)
	if count > 0 {
		return
	}
	seed := []Tariff{
		{Name: "Стандарт", Kind: TariffHourly, PricePerHour: 60, Active: true},
		{Name: "1 час", Kind: TariffPackage, DurationMinutes: 60, PackagePrice: 50, Active: true},
		{Name: "3 часа", Kind: TariffPackage, DurationMinutes: 180, PackagePrice: 135, Active: true},
	}
	for _, t := range seed {
		_, err := db.Exec(
			"INSERT INTO tariffs (name, kind, price_per_minute, price_per_hour, duration_minutes, package_price, description, active) VALUES (?,?,?,?,?,?,?,?)",
			t.Name, t.Kind, t.PricePerMinute, t.PricePerHour, t.DurationMinutes, t.PackagePrice, t.Description, boolToInt(t.Active),
		)
		if err != nil {
			log.Printf("Ошибка seed тарифа: %v", err)
		}
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
