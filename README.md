

## 🚀 Инструкция по запуску

1. Поднимите инфраструктуру и окружение:

   tripgoctl cluster start
   tripgoctl environment start
   tripgoctl connect


2. Примените миграции базы данных:

   make migrate

3. Запустите сервис:

   make run

### ⚙️ Переменные окружения (.env)
Сервис конфигурируется через переменные окружения (примеры значений смотрите в .env.example):

HTTP_ADDR — адрес и порт для запуска HTTP-сервера (например, :8080).

DATABASE_URL — строка подключения к PostgreSQL.

SHUTDOWN_TIMEOUT — таймаут для graceful shutdown (например, 10s).

LOG_LEVEL — уровень логирования.

Параметры пула соединений: DATABASE_MAX_CONNS, DATABASE_MIN_CONNS, DATABASE_CONNECT_TIMEOUT, DATABASE_QUERY_TIMEOUT, DATABASE_MAX_CONN_LIFETIME