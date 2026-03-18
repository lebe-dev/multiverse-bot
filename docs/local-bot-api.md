# Local Telegram Bot API (файлы больше 50 МБ)

По умолчанию Telegram ограничивает размер файлов до 50 МБ. Чтобы снять это ограничение и отправлять файлы до 2 ГБ, нужно поднять локальный сервер Telegram Bot API.

> Без этой настройки бот предложит сохранить большие видео в Google Drive через `/save`.

## Требования

- Docker и Docker Compose
- `TELEGRAM_API_ID` и `TELEGRAM_API_HASH` с сайта `my.telegram.org`

## Как получить API ID и API Hash

1. Зайди на `my.telegram.org` и войди под своим номером телефона
2. Перейди в раздел **API development tools**
3. Создай приложение (название и платформа — любые)
4. Скопируй `App api_id` и `App api_hash`

## Настройка

Пропиши в `.env`:

```bash
TELEGRAM_API_ID=your_api_id
TELEGRAM_API_HASH=your_api_hash
```

Переменная `LOCAL_BOT_API_URL` уже прописана в `docker-compose.yml` — вручную её указывать не нужно.

## Запуск

```bash
docker compose up -d
```

Docker Compose поднимет три сервиса:
- `telegram-bot-api` — локальный сервер Telegram (файлы до 2 ГБ)
- `cobalt` — бэкенд для Instagram и Twitter
- `bot` — сам бот

```bash
docker compose logs -f bot
```
