# Конфигурация

Все параметры задаются через переменные окружения. Скопируй `.env-example` в `.env` и заполни.

## Переменные окружения

### Основные

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | Да | — | Токен бота от BotFather |
| `ADMIN_USERS` | Нет | — | Telegram username(ы) администраторов через запятую |
| `ALLOWED_USERS` | Нет | (все) | Список разрешённых пользователей через запятую; пусто = доступ всем |
| `LOG_LEVEL` | Нет | `info` | Уровень логирования: `debug`, `info`, `warn`, `error` |
| `DEBUG` | Нет | `false` | Подробные ошибки в чатах администраторов |

### Скачивание

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `COBALT_API_URL` | Нет | `https://api.cobalt.tools` | URL Cobalt API |
| `YTDLP_PATH` | Нет | `yt-dlp` | Путь к исполняемому файлу yt-dlp |
| `TG_LIMIT` | Нет | `50` | Лимит размера файла для Telegram Bot API (МБ) |
| `LOCAL_BOT_API_URL` | Нет | — | URL локального Telegram Bot API сервера для больших файлов |

### Движки платформ

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `THREADS_ENGINE` | Нет | `default` | Движок Threads: `default` (прямой скрейпинг с uTLS) или `lovethreads` (lovethreads.net) |
| `YOUTUBE_ENGINE` | Нет | `default` | Движок YouTube: `default` (yt-dlp) или `savevids` (vidssave.com API) |
| `BROWSER_USER_AGENT` | Нет | Chrome 131 UA | User-Agent для Threads (только `default` движок) |

### Instagram

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `INSTAGRAM_FEATURES_ENABLED` | Нет | `false` | Включить все Instagram-фичи: скачивание, watchers, команды cookies |
| `WATCH_INSTAGRAM_POLL_INTERVAL` | Нет | `24h` | Интервал проверки новых сторис |
| `WATCH_INSTAGRAM_POSTS_POLL_INTERVAL` | Нет | `24h` | Интервал проверки новых постов |

### YouTube watcher

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `WATCH_POLL_INTERVAL` | Нет | `15m` | Интервал проверки новых видео |
| `WATCH_MAX_SUBSCRIPTIONS` | Нет | `20` | Макс. подписок на пользователя |
| `WATCH_MAX_CHANNELS_TOTAL` | Нет | `100` | Макс. каналов по всем пользователям |
| `WATCH_AUTO_DOWNLOAD` | Нет | `false` | `true` = отправлять видео сразу, `false` = уведомление с кнопкой «Скачать» |

### Google Drive

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `GOOGLE_CLIENT_ID` | Нет | — | OAuth 2.0 Client ID (тип «TVs and Limited Input devices») |
| `GOOGLE_CLIENT_SECRET` | Нет | — | OAuth 2.0 Client Secret |

### Хранилище

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `SQLITE_PATH` | Нет | `./data/bot.db` | Путь к базе данных SQLite |
| `SETTINGS_FILE` | Нет | `./user_settings.json` | Файл пользовательских настроек (качество, подписи) |
| `DRIVE_TOKENS_FILE` | Нет | `./user_drive_tokens.json` | Файл OAuth-токенов Google Drive |
| `ADMIN_CHATS_FILE` | Нет | `./data/admin_chats.json` | Файл маппинга admin username → chat ID |

### Плагины

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `PLUGINS_CONFIG` | Нет | — | Путь к `plugins.yml` для расширений бота |

## Команды бота

### Пользовательские

| Команда | Описание |
|---|---|
| `/start`, `/help` | Приветствие и список команд |
| `/settings` | Настройки качества и подписей |
| `/watch_youtube <url>` | Подписаться на YouTube-канал |
| `/watch_instagram_stories <url>` | Подписаться на Instagram сторис (требует `INSTAGRAM_FEATURES_ENABLED=true`) |
| `/watch_instagram_posts <url>` | Подписаться на Instagram посты (требует `INSTAGRAM_FEATURES_ENABLED=true`) |
| `/details <url>` | Показать доступные форматы и размеры |
| `/audio [url]` | Скачать только аудио (YouTube, M4A) |
| `/save [url]` | Сохранить видео в Google Drive |
| `/drive` | Управление подключением Google Drive |
| `/export` | Экспорт подписок и настроек |
| `/import` | Импорт подписок и настроек |

### Администраторские

| Команда | Описание |
|---|---|
| `/admin` | Панель администратора (статус, cookies, конфигурация) |
| `/add_youtube_cookies` | Загрузить YouTube cookies (Netscape формат) |
| `/add_instagram_cookies` | Загрузить Instagram cookies (требует `INSTAGRAM_FEATURES_ENABLED=true`) |
| `/delete_youtube_cookies` | Удалить YouTube cookies |
| `/delete_instagram_cookies` | Удалить Instagram cookies (требует `INSTAGRAM_FEATURES_ENABLED=true`) |

Для скачивания видео — просто отправьте ссылку.
