package main

// --- Пользователи и роли ---

const (
	RoleRoot    = "root"    // полный доступ, создаёт владельцев, управляет подписками
	RoleOwner   = "owner"   // полный доступ к своему клубу/сети, видит журнал действий
	RoleAdmin   = "admin"   // управляет тарифами, ценами, товарами
	RoleCashier = "cashier" // видит цены/тарифы, запускает и останавливает сессии
)

// roleLevel — числовой уровень роли (выше = больше прав).
func roleLevel(role string) int {
	switch role {
	case RoleRoot:
		return 4
	case RoleOwner:
		return 3
	case RoleAdmin:
		return 2
	case RoleCashier:
		return 1
	default:
		return 0
	}
}

// validRole проверяет, что строка — одна из известных ролей.
func validRole(role string) bool {
	return role == RoleRoot || role == RoleOwner || role == RoleAdmin || role == RoleCashier
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FullName  string `json:"full_name"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
	CreatedAt int64  `json:"created_at"`
	ClubID    int64  `json:"club_id"`
}

// --- Рабочие места (ПК) ---

const (
	PCStatusFree        = "free"
	PCStatusBusy        = "busy"
	PCStatusOffline     = "offline"
	PCStatusReserved    = "reserved"
	PCStatusMaintenance = "maintenance"
)

type PC struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Zone               string `json:"zone"`
	TariffID           int64  `json:"tariff_id"`
	Status             string `json:"status"`
	CurrentSessionID   int64  `json:"current_session_id"`
	IsOnline           bool   `json:"is_online"`
	LastSeen           int64  `json:"last_seen"`
	Notes              string `json:"notes"`
	Compliance         string `json:"compliance"` // JSON-отчёт FACEIT-ready
	RemainingSeconds   int64  `json:"remaining_seconds"` // вычисляется
	SessionStartedAt   int64  `json:"session_started_at"`
	ClientName         string `json:"client_name,omitempty"`
	SessionPaused      bool   `json:"session_paused"` // сессия на паузе (для кнопки ▶)
	LastSessionAmount  float64 `json:"last_session_amount"`  // итог последнего завершённого сеанса
	LastSessionSeconds int64   `json:"last_session_seconds"` // его фактическое время
	LastSessionEndedAt int64   `json:"last_session_ended_at"`
	Layout             int64   `json:"layout"`  // позиция на карте клуба (0 = авто)
	Comment            string  `json:"comment"` // заметка администратора
	CPU                string  `json:"cpu"`
	RAM                string  `json:"ram"`
	GPU                string  `json:"gpu"`
	Maintenance        bool    `json:"maintenance"` // режим обслуживания
	MAC                string  `json:"mac"`         // MAC-адрес для Wake-on-LAN
	Disks              []Disk  `json:"disks"`       // диски (от агента)
}

// Disk — информация о диске ПК, передаваемая агентом.
type Disk struct {
	Name  string `json:"name"`
	Total int64  `json:"total"`
	Free  int64  `json:"free"`
}

// Booking — бронь ПК на время.
type Booking struct {
	ID        int64  `json:"id"`
	PCID      int64  `json:"pc_id"`
	PCName    string `json:"pc_name"`
	Client    string `json:"client"`
	Phone     string `json:"phone"`
	StartAt   int64  `json:"start_at"`
	EndAt     int64  `json:"end_at"`
	Kind      string `json:"kind"` // blue | green | purple | gray
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

// --- Тарифы ---

const (
	TariffHourly  = "hourly"
	TariffPackage = "package"
	TariffNight   = "night"
	TariffOpen    = "open" // открытый: оплата по завершении сессии
)

type Tariff struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Kind            string  `json:"kind"`
	PricePerMinute  float64 `json:"price_per_minute"`
	PricePerHour    float64 `json:"price_per_hour"`
	DurationMinutes int64   `json:"duration_minutes"`
	PackagePrice    float64 `json:"package_price"`
	Description     string  `json:"description"`
	Active          bool    `json:"active"`
}

// --- Клиенты ---

type Client struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Phone           string  `json:"phone"`
	Balance         float64 `json:"balance"`
	BonusPoints     int64   `json:"bonus_points"`
	DiscountPercent float64 `json:"discount_percent"`
	Notes           string  `json:"notes"`
	CreatedAt       int64   `json:"created_at"`
	PasswordHash    string  `json:"-"`            // хеш пароля (не отдаём наружу)
	HasPassword     bool    `json:"has_password"` // задан ли пароль (для админки)
}

// --- Сессии ---

const (
	SessionActive = "active"
	SessionPaused = "paused"
	SessionEnded  = "ended"
)

type Session struct {
	ID           int64   `json:"id"`
	PCID         int64   `json:"pc_id"`
	PCName       string  `json:"pc_name"`
	ClientID     int64   `json:"client_id"`
	ClientName   string  `json:"client_name"`
	TariffID     int64   `json:"tariff_id"`
	TariffName   string  `json:"tariff_name"`
	UserID       int64   `json:"user_id"`
	UserName     string  `json:"user_name"`
	ShiftID      int64   `json:"shift_id"`
	StartedAt    int64   `json:"started_at"`
	EndsAt       int64   `json:"ends_at"`
	PausedAt     int64   `json:"paused_at"`
	Status       string  `json:"status"`
	TotalSeconds int64   `json:"total_seconds"`
	TotalAmount  float64 `json:"total_amount"`
	EndedAt      int64   `json:"ended_at"`
	Note         string  `json:"note"`
	PausedTotal  int64   `json:"paused_total"`  // накопленное время пауз (сек)
	FinalSeconds int64   `json:"final_seconds"` // фактическое активное время (сек)
	FinalAmount  float64 `json:"final_amount"`  // итоговая сумма «по факту»
	Open         bool    `json:"open"`          // открытый тариф: оплата по завершении
	RemainingSeconds int64 `json:"remaining_seconds"` // вычисляется
}

// --- Смены (касса) ---

const (
	ShiftOpen   = "open"
	ShiftClosed = "closed"
)

type Shift struct {
	ID             int64   `json:"id"`
	UserID         int64   `json:"user_id"`
	UserName       string  `json:"user_name"`
	OpenedAt       int64   `json:"opened_at"`
	ClosedAt       int64   `json:"closed_at"`
	OpeningBalance float64 `json:"opening_balance"`
	ClosingBalance float64 `json:"closing_balance"`
	TotalSales     float64 `json:"total_sales"`
	Status         string  `json:"status"`
}

// --- Транзакции (журнал операций/кассы) ---

const (
	TxTopup         = "topup"         // пополнение времени (наличные/карта)
	TxPackage       = "package"       // пакет
	TxRefund        = "refund"        // возврат
	TxClientDeposit = "client_deposit" // пополнение баланса клиента
	TxBalancePay    = "balance_pay"   // оплата временем с баланса клиента
	TxOpening       = "opening"       // открытие смены
	TxClosing       = "closing"       // закрытие смены
)

type Transaction struct {
	ID        int64   `json:"id"`
	Kind      string  `json:"kind"`
	SessionID int64   `json:"session_id"`
	ClientID  int64   `json:"client_id"`
	ClientName string `json:"client_name,omitempty"`
	UserID    int64   `json:"user_id"`
	UserName  string  `json:"user_name,omitempty"`
	ShiftID   int64   `json:"shift_id"`
	PCID      int64   `json:"pc_id"`
	PCName    string  `json:"pc_name,omitempty"`
	Amount    float64 `json:"amount"`
	Seconds   int64   `json:"seconds"`
	Method    string  `json:"method"`
	CreatedAt int64   `json:"created_at"`
	Note      string  `json:"note"`
}

// --- Товары и продажи (склад) ---

type Product struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Price    float64 `json:"price"`
	Stock    int64   `json:"stock"`
	Active   bool    `json:"active"`
}

type Sale struct {
	ID          int64   `json:"id"`
	ProductID   int64   `json:"product_id"`
	ProductName string  `json:"product_name"`
	Qty         int64   `json:"qty"`
	UnitPrice   float64 `json:"unit_price"`
	Amount      float64 `json:"amount"`
	UserID      int64   `json:"user_id"`
	UserName    string  `json:"user_name"`
	ShiftID     int64   `json:"shift_id"`
	Method      string  `json:"method"`
	CreatedAt   int64   `json:"created_at"`
}

// --- Настройки приложения ---

type Settings struct {
	ClubName           string  `json:"club_name"`
	Currency           string  `json:"currency"`
	DefaultPricePerMin float64 `json:"default_price_per_minute"`
	MaxMinutes         int64   `json:"max_minutes"`
	BonusRate          float64 `json:"bonus_rate"`
	RequireShift       bool    `json:"require_shift"`
	TelegramEnabled    bool    `json:"telegram_enabled"`
	TelegramBotToken   string  `json:"telegram_bot_token"`
	TelegramChatID     string  `json:"telegram_chat_id"`
	NotifyLowMinutes   int64   `json:"notify_low_minutes"`
	DurationPresets    []DurationPreset `json:"duration_presets"`
}

// DurationPreset — кнопка быстрого выбора времени в групповом запуске.
// Amount = 0 означает «считать по тарифу»; иначе — фиксированная цена.
type DurationPreset struct {
	Label   string  `json:"label"`
	Minutes int64   `json:"minutes"`
	Amount  float64 `json:"amount"`
}

// --- Публичный статус для агента на ПК ---

type PendingCommand struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
}

type PendingMessage struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

type AgentStatus struct {
	PCID             int64            `json:"pc_id"`
	PCName           string           `json:"pc_name"`
	Zone             string           `json:"zone"`
	ClubName         string           `json:"club_name"`
	Locked           bool             `json:"locked"`
	Paused           bool             `json:"paused"`
	RemainingSeconds int64            `json:"remaining_seconds"`
	EndsAt           int64            `json:"ends_at"`
	ClientName       string           `json:"client_name"`
	ClientID         int64            `json:"client_id"`
	ClientBalance    float64          `json:"client_balance"`
	ClientBonus      int64            `json:"client_bonus_points"`
	SessionStartedAt int64            `json:"session_started_at"`
	SessionOpen      bool             `json:"session_open"`
	ServerTime       int64            `json:"server_time"`
	BackgroundVersion int64           `json:"background_version"`
	Commands         []PendingCommand `json:"commands"`
	Messages         []PendingMessage `json:"messages"`
	Announcements    []Announcement   `json:"announcements"`
}

// --- Вызов администратора (с лаунчера) ---

type Call struct {
	ID         int64  `json:"id"`
	PCID       int64  `json:"pc_id"`
	PCName     string `json:"pc_name"`
	ClientName string `json:"client_name"`
	Reason     string `json:"reason"`
	Status     string `json:"status"` // open | resolved
	CreatedAt  int64  `json:"created_at"`
}

// --- Отзывы клиентов ---

type Review struct {
	ID         int64  `json:"id"`
	ClientName string `json:"client_name"`
	Rating     int64  `json:"rating"` // 1..5
	Comment    string `json:"comment"`
	CreatedAt  int64  `json:"created_at"`
}

// --- Объявления (контент на экранах ПК) ---

type Announcement struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	Active    bool   `json:"active"`
	CreatedAt int64  `json:"created_at"`
}
