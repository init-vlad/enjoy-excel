# 🌀 github.com/init-pkg/nova-template

## 🇬🇧 EN

**github.com/init-pkg/nova-template** is a base project template for fast and clean Go backend development.

---

### 🛠️ Commands

- `cmd/main` — start the application
- `cmd/automigrate` — run GORM automigrations
- `cmd/migrate` — run SQL migrations using Goose
- `cmd/seed` — run seeders

---

### 📁 Structure

- `config/` — environment and app configuration
  - `example.yaml` — config example
- `docs/` — Swagger documentation
- `domain/`
  - `models` — database models
  - `dtos` — DTOs
  - `modules` — module interfaces, constants
  - `vo` — Value Objects (e.g., time range logic)
- `internal/`
  - `config/` — Go config structs
  - `clients/` — external clients (e.g., Firebase, TG broadcast)
  - `infra/` — infrastructure clients (Redis, DB, ...)
  - `modules/` — feature modules
  - `transports/` — incoming traffic handlers (HTTP, AMQP, gRPC)
- `migrations/` — Postgres/MySQL SQL migrations
- `storage/` — local file storage

---

### 📦 Module

Module root is `/modules/<module>/`. Example: `/modules/product/`

#### 🚦 Transports

Handlers for incoming traffic. All registered in `module.go` under the "transports" comment.

- `grpc/` — gRPC handler. Recommended package name: `<entity>_grpc_handler`. Must have `New()` and `Register()`.
- `amqp/` — AMQP handler. Recommended package name: `<entity>_amqp_handler`. Must have `New()` and `Register()`.
- `http/` — HTTP handler. Recommended package name: `<entity>_http_handler`. Must have `New()` and `Register()`.
  - By default, nova provides a CRUD handler. Use `http/<entity>_crud_http_adapter.go` to adapt your service to `nova/crud_handler.UntypedCrud`.

#### ⚙️ Service

Service layer. Should not depend on specific handler logic (e.g., no direct use of fiber ctx or protos.Req).

#### 🗄️ Repo

Database layer. Implement atomic, pure DB operations. Logic-free, just interfaces. Service layer decides what is allowed.

---

### 📝 Naming Conventions

Names should be as clear and unambiguous as possible. Up to 4 words allowed.

#### Functions & Methods

- Avoid `func (s *Service)` — use `this` instead.
- Declare variables with `var` wherever possible.

#### Lifecycle

- 🌀 `Run()` — launches a goroutine, non-blocking
- ⛔ `Start()` — blocking, stays active until finished
- 📡 `Listen()` / `Serve()` — blocking, often runs a connection loop
- 🧹 `Stop()` / `Shutdown()` — stops the process, should not block for long (may be async or wait for graceful shutdown)

#### Getters

- Avoid `Get` prefix: `GetName()` → `Name()`

#### Variables

- For bools, use prefixes: `is`, `can`, `should`, etc. E.g., `var isOpen` (not `var open`)

---

### 🛠️ Taskfile

Below are the main commands from `Taskfile.yml`:

| Command                                    | Description                                    |
| ------------------------------------------ | ---------------------------------------------- |
| `task run`                                 | Run the app with config                        |
| `task build`                               | Build Linux binary to `build/app`              |
| `task fresh`                               | Run automigrate fresh                          |
| `task automigrate`                         | Run automigrate                                |
| `task refresh`                             | Fresh automigrate and run seeders              |
| `task migrate-up`                          | Run migrations up                              |
| `task migrate-down`                        | Run migrations down                            |
| `task seed`                                | Run seeders                                    |
| `task gendoc`                              | Generate Swagger docs                          |
| `task test`                                | Run tests with test config                     |
| `task make-postgres-migration TABLE=table` | Create new Postgres migration for table        |
| `task make-mysql-migration TABLE=table`    | Create new MySQL (MariaDB) migration for table |

---

---

### 🚀 Boilerplates

#### Database Usage

To use the database, inject `*gorm.DB` and implement a `getDb(ctx)` method to retrieve the database through this method:

```go
getDb(ctx context.Context) *gorm.DB {
  db, ok := nova_gorm.GetDb(ctx)
  if ok {
    return db
  }

  return this.db
}
```

#### Transactions

To create a transaction, use `this.GetDb().Transaction()` and inside the transaction create `var txctx = nova_gorm.WithDb(ctx, tx)` then pass this context within the transaction.
If a transaction is already started at a higher level, `Transaction()` will simply create a checkpoint.

#### Relations (Has Many, Many-to-Many)

**DTOs:**

- Accept only `[]ID` for relations in DTOs

**Repository Layer:**

- Implement two methods: `ClearRelations(ctx, id)` and `AssignRelations(ctx, id, relationIds)`
- **Many-to-Many:** Methods should interact with the junction table, creating/deleting records
- **Has Many (nullable FK):** `Assign` and `Clear` should update foreign keys in related records
- **Has Many (non-nullable FK):** Full creation of related records is required (via nested fields or separate CRUD, the latter being preferable in most cases)

**Service Layer:**

- **Create method:** Use transactions to assign relations after creating the main entity. Keep `len == 0` checks in the service
- **Update method:**
  - Do nothing if `relationIds == nil`
  - If `!= nil && len == 0`: call `ClearRelations`
  - If `len > 0`: call `ClearRelations` then `AssignRelations`
  - All within a single transaction
  - Remove preloads for update method; after update + (clear, assign), perform `GetByID` with required preloads

---

## 🇷🇺 RU

**github.com/init-pkg/nova-template** — это базовый шаблон для быстрого и чистого старта Go backend-проектов.

---

### 🛠️ Команды

- `cmd/main` — запуск приложения
- `cmd/automigrate` — автомиграции через GORM
- `cmd/migrate` — SQL-миграции через Goose
- `cmd/seed` — запуск сидеров

---

### 📁 Структура

- `config/` — окружение и конфиг
  - `example.yaml` — пример конфига
- `docs/` — Swagger-документация
- `domain/`
  - `models` — модели БД
  - `dtos` — DTO
  - `modules` — интерфейсы, константы
  - `vo` — Value Objects (например, логика диапазона времени)
- `internal/`
  - `config/` — структуры конфига на Go
  - `clients/` — внешние клиенты (Firebase, TG broadcast и др.)
  - `infra/` — инфраструктурные клиенты (Redis, БД и др.)
  - `modules/` — модули-фичи
  - `transports/` — обработчики входящего трафика (HTTP, AMQP, gRPC)
- `migrations/` — SQL-миграции для Postgres/MySQL
- `storage/` — локальное файловое хранилище

---

### 📦 Модуль

Корень модуля — `/modules/<module>/`. Пример: `/modules/product/`

#### 🚦 Transports

Папка для обработчиков входящего трафика. Все регистрируется в `module.go` под комментарием "transports".

- `grpc/` — gRPC-обработчик. Рекомендованное имя пакета: `<entity>_grpc_handler`. Должен иметь `New()` и `Register()`.
- `amqp/` — AMQP-обработчик. Рекомендованное имя пакета: `<entity>_amqp_handler`. Должен иметь `New()` и `Register()`.
- `http/` — HTTP-обработчик. Рекомендованное имя пакета: `<entity>_http_handler`. Должен иметь `New()` и `Register()`.
  - По умолчанию nova предоставляет CRUD-обработчик. Для этого создается `http/<entity>_crud_http_adapter.go`, адаптирующий сервис к `nova/crud_handler.UntypedCrud`.

#### ⚙️ Service

Сервисный слой. По возможности не должен зависеть от логики конкретного обработчика (например, не использовать fiber ctx или protos.Req).

#### 🗄️ Repo

Слой работы с БД. Рекомендуется реализовывать чистые атомарные операции. Логика — только в сервисе.

---

### 📝 Именование

Название должно быть максимально точным и однозначным. Допускается до 4 слов.

#### Функции и методы

- Не используйте `func (s *Service)`, только `this`.
- Переменные объявлять через `var` где возможно.

#### Жизненный цикл

- 🌀 `Run()` — запускает горутину, не блокирует
- ⛔ `Start()` — блокирующая функция
- 📡 `Listen()` / `Serve()` — блокирующая, часто запускает цикл приёма соединений
- 🧹 `Stop()` / `Shutdown()` — завершает работу, не должен долго блокировать (может быть асинхронным или ждать graceful shutdown)

#### Геттеры

- Избегать префикса `Get`: `GetName()` → `Name()`

#### Переменные

- Для bool — префиксы: `is`, `can`, `should` и т.д. Например, `var isOpen` (не `var open`)

---

### 🛠️ Taskfile

Ниже приведены основные команды из `Taskfile.yml`:

| Команда                                    | Описание                                     |
| ------------------------------------------ | -------------------------------------------- |
| `task run`                                 | Запуск приложения с конфигом                 |
| `task build`                               | Сборка Linux-бинаря в `build/app`            |
| `task fresh`                               | Полная автомиграция                          |
| `task automigrate`                         | Автомиграция                                 |
| `task refresh`                             | Полная автомиграция и запуск сидеров         |
| `task migrate-up`                          | Применить миграции                           |
| `task migrate-down`                        | Откатить миграцию                            |
| `task seed`                                | Запуск сидеров                               |
| `task gendoc`                              | Генерация Swagger-документации               |
| `task test`                                | Запуск тестов с тестовым конфигом            |
| `task make-postgres-migration TABLE=table` | Создать миграцию для Postgres-таблицы        |
| `task make-mysql-migration TABLE=table`    | Создать миграцию для MySQL-таблицы (MariaDB) |

---

### 🚀 Шаблоны (Boilerplates)

#### Использование db

для использования db стоит сделать inject `*gorm.DB` и реализовать метод getDb(ctx) и доставать db через этот метод

```go
getDb(ctx context.Context) *gorm.DB {
  db, ok := nova_gorm.GetDb(ctx)
  if ok {
    return db
  }

  return this.db
}
```

#### Транзакции

для созданая транзакции нужно сделать this.GetDb().Transaction() и внутри транзакции сделать var txctx = nova_gorm.WithDb(ctx, tx) и далее передавать этот контекст внутри данной транзакции.
если транзакция уже начата на более верхнем уровне, Transaction() просто создаст чекпоинт

#### Связи (Has Many, Many-to-Many)

**DTO:**

- В DTO принимать только `[]ID` для связей

**Слой репозитория:**

- Реализовать два метода: `ClearRelations(ctx, id)` и `AssignRelations(ctx, id, relationIds)`
- **Many-to-Many:** Методы должны взаимодействовать с промежуточной таблицей, создавая/удаляя записи
- **Has Many (nullable FK):** `Assign` и `Clear` должны обновлять внешние ключи в связанных записях
- **Has Many (non-nullable FK):** Требуется полноценное создание связанных записей (через вложенные поля или отдельный CRUD, последнее предпочтительнее в большинстве случаев)

**Сервисный слой:**

- **Метод Create:** В транзакции выполнять assign relations после создания основной сущности. Проверку `len == 0` оставить в сервисе
- **Метод Update:**
  - Ничего не делать, если `relationIds == nil`
  - Если `!= nil && len == 0`: вызвать `ClearRelations`
  - Если `len > 0`: вызвать `ClearRelations`, затем `AssignRelations`
  - Из update убрать opts из параметров метода и возвращать только errs.Error. В http adapter делать getById после update.
  - Или убрать preloads для метода update; после update + (clear, assign) выполнить `GetById` с нужными preloads
  - Всё в рамках одной транзакции (при отдельном GetById, транзакция также должна быть внутри http adapter)
