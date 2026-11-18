# PlayCore — Game Club Control

Система управления игровым (компьютерным) клубом. Один Go-бинарник со встроенной
админ-панелью + Windows-клиент для игровых мест на C#/.NET 8 (WPF).

- **Сервер** — Go, чистый single exe: SQLite (pure-Go драйвер) или PostgreSQL,
  админ-панель встроена через `go:embed`.
- **Админ-панель** — тёмная тема, Tailwind, акцент `#2E7CF6` на фоне `#11151C`.
- **Клиент** — `ClubShell` (лаунчер) + `ClubAgent.Service` (служба-сторож),
  киберпанк-стеклянная тема с неоном.

## Возможности

### Админ-панель
- **Карта зала**: ПК, зоны, статусы (свободен/занят/offline), live-дашборд,
  drag-раскладка («✏️ Редактировать»), сброс ПК, WOL, обслуживание, аппаратная
  информация с агента (CPU/GPU/диски).
- **Сессии**: старт, пополнение времени, пауза/возобновление, завершение, история;
  почасовые / пакетные / открытые (постоплата) тарифы.
- **Тарифы и пакеты**: почасовой / пакет / открытый, привязка к ПК.
- **Касса и смены**: открытие/закрытие смены, выручка по услугам и товарам.
- **Товары и склад**: каталог (категории → товары), продажа, приход, остатки.
- **Клиенты**: баланс, депозит, скидка, бонусы; вход через лаунчер (bcrypt).
- **Отчёты**: выручка по датам/ПК/кассиру, наличные/карта/баланс + товары, CSV.
- **Пользователи и роли**: `root` > `owner` > `admin` > `cashier`.
- **Подписки, клубы, отзывы, объявления, журнал аудита**.
- **Telegram-уведомления**: низкий остаток, истечение сессии, смены.

### Клиент (Windows)
- Вход/регистрация клиента (пароль — bcrypt), магазин (категории → товары),
  покупка с баланса, вызов администратора.
- Нижняя панель: язык, интернет, громкость (слайдер), NVIDIA.
- Сессия по открытому тарифу: оплата по факту по завершении.

## Архитектура

```
                        ┌─────────────────────────────────────────┐
  Администратор ──────► │  PlayCore Server (Go, single exe)       │
  (браузер)        HTTP │  • админ-панель (go:embed)              │
                        │  • SQLite (pure-Go) или PostgreSQL       │
                        │  • API + роли + аудит                    │
                        └──────────────────┬──────────────────────┘
                                           │ /api/agent/* (X-PC-Token)
                     ┌─────────────────────┼─────────────────────┐
                     ▼                     ▼                     ▼
                 ПК #1                 ПК #2                 ПК #N
            ClubShell +             ClubShell +             ClubShell +
            ClubAgent.Service       ClubAgent.Service       ClubAgent.Service
```

Каждый ПК опрашивает сервер и показывает заглушку, когда время вышло.
Открытый тариф — постоплата: баланс списывается по фактическому времени.

## Запуск сервера

### Вариант 1 — Docker (SQLite, по умолчанию)

```bash
docker build -t playcore .
docker run -d --name playcore \
  -p 8080:8080 \
  -v "$PWD/data:/app/data" \
  --env-file .env \
  playcore
```

### Вариант 2 — Docker + PostgreSQL

```bash
docker compose --profile postgres up -d
# в .env: DB_DRIVER=postgres, DATABASE_URL=postgres://gameclub:gameclub@db:5432/gameclub?sslmode=disable
```

### Вариант 3 — вручную (нужен Go 1.24+, без gcc/cgo)

```bash
go mod tidy
go build -o easylock .
./easylock
```

### Вариант 4 — Windows (нативно, single exe)

```bash
./build-windows.sh   # -> easylock.exe (иконка из playcorelogo.svg, GUI без консоли)
```

На Windows положите `easylock.exe` в папку, рядом `.env`, запустите — `data\` создастся
сама. Для автозапуска — ярлык в `shell:startup` или задача планировщика. Порт 8080
открыть в брандмауэре. Админка: `http://localhost:8080`.

## Конфигурация (`.env`)

Скопируйте `.env.example` в `.env` и заполните.

| Переменная | Назначение |
|---|---|
| `DB_DRIVER` | `sqlite` (по умолчанию) или `postgres` |
| `DB_PATH` | путь к SQLite-файлу |
| `DATABASE_URL` | DSN PostgreSQL |
| `ADMIN_USER` / `ADMIN_PASS` | начальный администратор (root, при первом запуске) |
| `AGENT_TOKEN` | глобальный токен агента (legacy, если у ПК нет персонального) |
| `DEFAULT_PRICE` | цена за минуту по умолчанию |
| `MAX_MINUTES` | макс. лимит сессии (0 = без лимита) |
| `CLUB_NAME` / `CURRENCY` | название клуба и валюта |
| `REQUIRE_SHIFT` | `1` — требовать открытую смену перед продажей |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` | настройки Telegram |
| `NOTIFY_LOW_MINUTES` | порог «низкого остатка» в минутах |
| `TLS_CERT` / `TLS_KEY` | пути к сертификату/ключу для HTTPS |
| `PORT` | порт (по умолчанию 8080) |

При первом запуске создаются администратор из `ADMIN_USER`/`ADMIN_PASS`, демо-тарифы
и настройки. БД автоматически мигрирует при старте.

## Клиент (Windows)

### Сборка

```bash
cd client && ./build.sh   # -> client/out/ (ClubShell.exe + ClubAgent.Service.exe, self-contained)
```

### Установка на игровое место

1. В админке создайте ПК и скопируйте его **токен агента** (показывается один раз;
   кнопка «🔑» на карточке ПК перевыпускает его).
2. Скопируйте всю папку `client/out/` на ПК.
3. Рядом положите `agent.json` (образец — `client/agent.example.json`):
   `server_url`, `pc_id`, `pc_token`.
4. Выполните от администратора:
   ```bat
   ClubAgent.Service.exe -install
   ```
   затем перезагрузите ПК (или `-uninstall`, чтобы удалить службу).

Служба-сторож следит за сессией, показывает заглушку «Время истекло» и блокирует
ввод; после продления в админке разблокируется автоматически.

## Роли

- **root** — всё, включая клубы и подписки (мультиклуб).
- **owner** — всё в рамках клуба, включая пользователей и настройки.
- **admin** — ПК, тарифы, товары, пользователи, настройки.
- **cashier** — рабочие операции: сессии, клиенты, касса, продажи, отчёты.

Иерархия строгая (`roleLevel`), меню и кнопки в админке показываются по роли.

## API (кратко)

| Метод | Путь | Описание |
|---|---|---|
| POST | `/api/login`, `/api/logout`, GET `/api/me` | авторизация |
| GET/POST | `/api/pcs`, PUT/DELETE `/api/pcs/{id}` | ПК |
| POST | `/api/pcs/{id}/rotate-token` | перевыпуск токена (admin) |
| POST | `/api/sessions`, `/api/sessions/{id}/add-time` | сессии |
| POST | `/api/sessions/{id}/pause\|resume\|stop` | управление |
| GET | `/api/sessions/active`, `/api/sessions/history` | списки |
| GET/POST | `/api/tariffs`, PUT/DELETE `/api/tariffs/{id}` | тарифы |
| GET/POST | `/api/clients`, POST `/api/clients/{id}/deposit` | клиенты |
| GET/POST | `/api/products`, PUT/DELETE `/api/products/{id}` | товары |
| POST | `/api/products/{id}/sell`, `/api/products/{id}/restock` | продажа/приход |
| POST | `/api/shifts/open`, `/api/shifts/close`, GET `/api/shifts/current` | смены |
| GET | `/api/reports/transactions?from&to&pc_id&user_id` | отчёт |
| GET/PUT | `/api/settings` | настройки |
| GET | `/api/audit` | журнал аудита (owner/root) |
| GET/POST | `/api/clubs`, POST `/api/subscriptions` | клубы и подписки (root) |
| GET/POST | `/api/agent/*` | вход/выход/покупка/вызов/железо (заголовок `X-PC-Token`) |

Рабочие эндпоинты требуют авторизации (cookie), кроме `/api/login`, `/api/health`
и агентских `/api/agent/*` (токен).

## Структура проекта

```
main.go                  — маршруты, старт, фоновые тикеры, TLS, go:embed
db.go                    — слой БД (SQLite/PostgreSQL), схема, миграции, сиды
models.go                — структуры данных
auth.go                  — пароли (bcrypt), cookie-сессии, middleware ролей
settings.go              — настройки приложения
notify.go                — Telegram-уведомления
audit.go                 — журнал действий
wol.go                   — Wake-on-LAN
handlers_*.go            — ПК, сессии, тарифы, клиенты, касса, товары, отчёты,
                           пользователи, клубы, агент, отзывы, объявления
templates/index.html     — админ-панель (SPA, встроена в бинарник)
playcorelogo.svg         — логотип (отдаётся по /playcorelogo.svg)

client/
  ClubShell/             — WPF-лаунчер (вход, магазин, нижняя панель)
  ClubAgent.Service/     — служба-сторож (Windows service)
  Club.Core/             — общие DTO, серверный клиент, детектор Steam
  Club.Core.Tests/       — тесты
  build.sh               — сборка self-contained под win-x64
  agent.example.json     — образец конфига клиента
```
