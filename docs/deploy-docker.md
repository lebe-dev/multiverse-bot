# Деплой через Docker Compose

## Требования

- Docker 24+ и Docker Compose v2
- Ubuntu / Debian (или любой Linux с Docker)

## Быстрый старт

```bash
git clone git@github.com:lebe-dev/multiverse-bot.git
cd multiverse-bot

cp .env-example .env
nano .env              # минимум — вписать TELEGRAM_BOT_TOKEN

touch cookies.txt      # пустой файл нужен до первого запуска

docker compose up -d --build
docker compose logs -f bot
```

## Настройка .env

Обязательные поля:

```env
TELEGRAM_BOT_TOKEN=...   # от @BotFather
ADMIN_USERS=yourusername # ваш Telegram username без @
```

Опциональные (раскомментировать при необходимости):

```env
ALLOWED_USERS=user1,user2   # оставить пустым — бот открыт для всех
LOG_LEVEL=info
```

> **Важно:** не оставляйте инлайн-комментарии после значений — некоторые
> парсеры читают их как часть значения:
> ```env
> ALLOWED_USERS=alice   # BAD — " alice   # ..." попадёт в список
> ALLOWED_USERS=alice   # GOOD — комментарий на отдельной строке
> ```

## Сервисы в составе docker-compose

| Сервис | Образ | Назначение |
|---|---|---|
| `bot` | сборка из Dockerfile | Telegram-бот |
| `cobalt` | `ghcr.io/imputnet/cobalt` | Скачивание Instagram / Twitter / Threads |
| `telegram-bot-api` | `aiogram/telegram-bot-api` | Локальный Bot API (файлы > 50 МБ) |
| `watchtower` | `ghcr.io/containrrr/watchtower` | Авто-обновление Cobalt |

## Cookies для YouTube

YouTube периодически требует авторизацию. Для этого нужны cookies браузера.

1. Экспортируйте cookies из браузера в файл `cookies.txt` (формат Netscape).
   Расширения: [Get cookies.txt LOCALLY](https://chrome.google.com/webstore/detail/get-cookiestxt-locally) для Chrome,
   [cookies.txt](https://addons.mozilla.org/firefox/addon/cookies-txt/) для Firefox.

2. Отправьте файл боту через Telegram — команда `/admin` → загрузить cookies.

Подробнее: [cookies.md](cookies.md)

## Google Drive (опционально)

Позволяет сохранять скачанные видео в личный Drive пользователя.

### 1. Раскомментировать volume-маунты в docker-compose.yml

```yaml
volumes:
  - bot-data:/data
  - ./cookies.txt:/app/cookies.txt
  - ./user_drive_tokens.json:/app/user_drive_tokens.json   # раскомментировать
  - ./user_settings.json:/app/user_settings.json           # раскомментировать
```

Создать пустые файлы до первого запуска:

```bash
echo '{}' > user_drive_tokens.json
echo '{}' > user_settings.json
```

### 2. Добавить Google OAuth credentials в .env

```env
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
```

Как получить — [google-drive.md](google-drive.md).

### 3. Перезапустить бот

```bash
docker compose up -d bot
```

После этого команда `/drive` покажет кнопку «Подключить Google Drive».

## Локальный Telegram Bot API (файлы > 50 МБ)

По умолчанию лимит Telegram — 50 МБ. Для больших файлов нужен локальный API.

Добавить в `.env`:

```env
TELEGRAM_API_ID=...    # с https://my.telegram.org
TELEGRAM_API_HASH=...
```

Сервис `telegram-bot-api` в docker-compose уже настроен и запустится автоматически.

Подробнее: [local-bot-api.md](local-bot-api.md)

## Известные нюансы при первом деплое

### SQLite не стартует — `out of memory (14)`

Не нехватка памяти — это `SQLITE_CANTOPEN`. Docker volume создаётся с
владельцем `root`, но бот работает от uid 10001. Если вы используете
старую версию образа без фикса:

```bash
docker run --rm -v multiverse-bot_bot-data:/data alpine \
  chown -R 10001:10001 /data
docker compose restart bot
```

### Кнопки «Подключить Google Drive» / «Отключить» не реагируют

telebot.v4 добавляет невидимый символ `\f` к данным inline-кнопок.
Исправлено в текущей версии. Если кнопки не работают — убедитесь что
используете актуальный образ.

### `/watch @channelname` возвращает «канал не найден»

yt-dlp в режиме `--flat-playlist` не заполняет поля `channel_id` /
`channel`. Исправлено в текущей версии (используются `playlist_channel_id`
/ `playlist_channel`). Обновите образ и попробуйте снова.

### cookies.txt — ошибка при сохранении через бота

В старых версиях файл монтировался как `:ro` (read-only). Проверьте
`docker-compose.yml` — строка должна быть без `:ro`:

```yaml
- ./cookies.txt:/app/cookies.txt   # правильно
- ./cookies.txt:/app/cookies.txt:ro  # неправильно — бот не сможет обновить
```

## Полезные команды

```bash
docker compose up -d --build   # пересборка и запуск
docker compose logs -f bot     # логи в реальном времени
docker compose restart bot     # перезапуск бота (без пересборки)
docker compose down            # остановить всё
```
