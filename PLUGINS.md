# Плагины

Плагины расширяют бот новыми командами и обработчиками URL без изменения основного кода. Каждый плагин — отдельный HTTP-сервис (свой контейнер), объявленный в `plugins.yml`.

## Принципы

- **Изоляция** — плагин не имеет доступа к состоянию бота, базе данных или Telegram API. Он получает запрос и возвращает набор действий.
- **Статическая регистрация** — все плагины объявляются в `plugins.yml` и загружаются при старте. Для добавления/удаления плагина требуется перезапуск.
- **Приоритет встроенного** — встроенные команды и платформы всегда имеют приоритет над плагинами.
- **Отказоустойчивость** — сбой плагина никогда не ломает бота. Пользователь получает сообщение об ошибке, встроенная функциональность продолжает работать.

## Быстрый старт

1. Создайте `plugins.yml` на основе примера:

```bash
cp plugins.yml.example plugins.yml
```

2. Укажите путь в `.env`:

```bash
PLUGINS_CONFIG=./plugins.yml
```

3. Перезапустите бота.

## Конфигурация (`plugins.yml`)

```yaml
plugins:
  - name: tiktok                    # уникальный идентификатор
    url: http://plugin-tiktok:8080  # базовый URL HTTP-сервиса
    enabled: true                   # опционально, по умолчанию true
    timeout: 30s                    # таймаут на запрос, по умолчанию 30s

  - name: summarize
    url: http://plugin-summarize:8080
    timeout: 60s
```

| Поле | Обязательно | По умолчанию | Описание |
|------|-------------|--------------|----------|
| `name` | Да | — | Уникальное имя: `^[a-z0-9][a-z0-9-]*$`, максимум 32 символа |
| `url` | Да | — | Базовый URL сервиса; бот добавляет `/health`, `/manifest` и т.д. |
| `enabled` | Нет | `true` | Отключить плагин без удаления из конфига |
| `timeout` | Нет | `30s` | Таймаут на каждый HTTP-запрос к плагину |

## HTTP API плагина

Каждый плагин должен реализовать 4 эндпоинта.

### `GET /health`

Проверка здоровья. Вызывается при старте бота.

**Ответ `200 OK`:**
```json
{ "status": "ok" }
```

Любой не-200 ответ или таймаут — плагин считается недоступным и пропускается.

### `GET /manifest`

Описание возможностей плагина. Вызывается один раз при старте после успешного health check.

**Ответ `200 OK`:**
```json
{
  "name": "tiktok",
  "description": "Скачивание видео из TikTok",
  "commands": [
    {
      "command": "/tiktok",
      "description": "Скачать видео по ссылке TikTok"
    }
  ],
  "url_patterns": [
    "(?i)tiktok\\.com/@[\\w.]+/video/\\d+",
    "(?i)vm\\.tiktok\\.com/\\w+"
  ]
}
```

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `name` | string | Да | Должно совпадать с `name` в `plugins.yml` |
| `description` | string | Да | Для пользователей, показывается в `/start` |
| `commands` | array | Нет | Slash-команды плагина |
| `commands[].command` | string | Да | Начинается с `/`, строчные буквы |
| `commands[].description` | string | Да | Описание команды |
| `url_patterns` | array | Нет | Регулярные выражения Go для сопоставления URL |

**Валидация при старте:**
- Команды не должны совпадать с встроенными (`/start`, `/settings`, `/config`, `/cookies`, `/details`, `/save`, `/auth`, `/disconnect`, `/watch`)
- Команды не должны совпадать между плагинами
- URL-паттерны должны быть валидными Go regexp

### `POST /execute`

Вызывается при получении команды или URL, совпавшего с паттерном.

**Запрос (команда):**
```json
{
  "trigger": {
    "type": "command",
    "command": "/tiktok",
    "args": "https://tiktok.com/@user/video/123",
    "raw_text": "/tiktok https://tiktok.com/@user/video/123"
  },
  "user": { "id": 123456789, "username": "johndoe" },
  "message_id": 42
}
```

**Запрос (URL):**
```json
{
  "trigger": {
    "type": "url",
    "url": "https://tiktok.com/@user/video/123",
    "matched_pattern": "(?i)tiktok\\.com/@[\\w.]+/video/\\d+",
    "raw_text": "https://tiktok.com/@user/video/123"
  },
  "user": { "id": 123456789, "username": "johndoe" },
  "message_id": 42
}
```

**Ответ `200 OK`:**
```json
{
  "actions": [
    { "type": "text", "text": "Скачиваю видео...", "parse_mode": "HTML" }
  ]
}
```

**Ответ с ошибкой (`4xx`/`5xx`):**
```json
{ "error": "Видео приватное или не существует" }
```

Поле `error` показывается пользователю как есть. HTTP 5xx без тела — генерируется общее сообщение об ошибке.

### `POST /callback`

Вызывается при нажатии inline-кнопки с `callback`.

**Запрос:**
```json
{
  "callback_id": "more",
  "user": { "id": 123456789, "username": "johndoe" },
  "message_id": 55
}
```

**Ответ `200 OK`:**
```json
{
  "toast": "Загрузка...",
  "actions": [
    { "type": "text", "text": "Дополнительные опции" }
  ]
}
```

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `toast` | string | Нет | Всплывающее уведомление Telegram (callback toast) |
| `actions` | array | Нет | Те же типы действий, что и в `/execute` |

## Типы действий

### `text` — отправить текстовое сообщение

```json
{
  "type": "text",
  "text": "Результат",
  "parse_mode": "HTML",
  "buttons": [
    { "text": "Открыть", "url": "https://example.com" },
    { "text": "Ещё", "callback": "more" }
  ]
}
```

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `text` | string | Да | Текст сообщения |
| `parse_mode` | string | Нет | `"HTML"` или `"Markdown"` |
| `buttons` | array | Нет | Inline-кнопки (один ряд) |
| `buttons[].text` | string | Да | Текст кнопки |
| `buttons[].url` | string | Нет | URL (взаимоисключающе с `callback`) |
| `buttons[].callback` | string | Нет | Callback data (вызовет `/callback` эндпоинт) |

### `file` — отправить файл по URL

```json
{
  "type": "file",
  "url": "https://cdn.example.com/video.mp4",
  "filename": "video.mp4",
  "caption": "TikTok видео от @user",
  "mime_type": "video/mp4"
}
```

| Поле | Тип | Обязательно | Описание |
|------|-----|-------------|----------|
| `url` | string | Да | Публичный URL для скачивания |
| `filename` | string | Нет | Имя файла |
| `caption` | string | Нет | Подпись к файлу |
| `mime_type` | string | Нет | `video/*` → видео, `image/*` → фото, иначе → документ |

Бот сам скачивает файл и отправляет пользователю. Плагин не общается с Telegram напрямую.

### `edit` — редактировать сообщение

```json
{
  "type": "edit",
  "message_id": 42,
  "text": "Готово!"
}
```

### `delete` — удалить сообщение

```json
{
  "type": "delete",
  "message_id": 42
}
```

Несколько действий выполняются последовательно. Ответ может содержать любую комбинацию типов.

## Приоритет маршрутизации

1. **Встроенные команды** — `/start`, `/settings` и т.д. Всегда приоритетны.
2. **Команды плагинов** — `/tiktok` и т.д. Регистрируются при старте.
3. **Встроенные URL-обработчики** — YouTube, Instagram, Twitter, Threads.
4. **URL-паттерны плагинов** — в порядке объявления в `plugins.yml`.
5. **Fallback** — сообщение «платформа не поддерживается».

## Docker Compose

```yaml
services:
  bot:
    build: .
    env_file: .env
    environment:
      PLUGINS_CONFIG: /app/plugins.yml
    volumes:
      - ./plugins.yml:/app/plugins.yml:ro
    depends_on:
      plugin-tiktok:
        condition: service_healthy

  plugin-tiktok:
    image: ghcr.io/example/multiverse-plugin-tiktok:latest
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 15s
      timeout: 5s
      retries: 3
```

Плагины — отдельные контейнеры в одной Docker-сети. Бот обращается к ним по имени сервиса.

## Обработка ошибок

| Ситуация | Поведение |
|----------|-----------|
| Плагин недоступен при старте | Пропускается с предупреждением в логе |
| Невалидный манифест | Плагин пропускается с предупреждением |
| `/execute` вернул `4xx` с полем `error` | Текст ошибки показывается пользователю |
| `/execute` вернул `5xx` или таймаут | Общее сообщение «ошибка плагина» |
| Файл по URL не скачивается | Сообщение «не удалось загрузить файл» |
| Файл превышает лимит Telegram | Сообщение «файл слишком большой» |
| `/callback` ошибка | Toast «ошибка плагина» |
| Неизвестный тип действия | Действие пропускается, логируется предупреждение |

## Безопасность

- Плагин получает только: тип триггера, данные пользователя (ID, username), ID сообщения. Никаких токенов, истории чатов, настроек.
- Плагин не может отправлять сообщения произвольным пользователям — он возвращает действия, бот их выполняет.
- Callback-данные плагинов имеют префикс `p_<name>|` — плагин не может подделать callback другого плагина или встроенного обработчика.
- Каждый запрос к плагину имеет настраиваемый таймаут.
- Плагины регистрируются только через конфиг — самостоятельная регистрация в runtime невозможна.

## Разработка плагина

Минимальный плагин на Go:

```go
package main

import (
    "encoding/json"
    "net/http"
)

func main() {
    http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    })

    http.HandleFunc("/manifest", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]any{
            "name":        "hello",
            "description": "Приветствие",
            "commands": []map[string]string{
                {"command": "/hello", "description": "Сказать привет"},
            },
        })
    })

    http.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]any{
            "actions": []map[string]string{
                {"type": "text", "text": "Привет! 👋"},
            },
        })
    })

    http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
        json.NewEncoder(w).Encode(map[string]any{})
    })

    http.ListenAndServe(":8080", nil)
}
```

Подробная техническая спецификация — в [ARCH.md](ARCH.md).
