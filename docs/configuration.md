# Конфигурация

Все параметры задаются через переменные окружения. Скопируй `.env-example` в `.env` и заполни.

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | Да | — | Токен бота от BotFather |
| `ADMIN_USERS` | Нет | — | Telegram username(ы) администраторов через запятую |
| `ALLOWED_USERS` | Нет | (все) | Список разрешённых пользователей через запятую |
| `COBALT_API_URL` | Нет | `https://api.cobalt.tools` | URL Cobalt API |
| `YTDLP_PATH` | Нет | `yt-dlp` | Путь к исполняемому файлу yt-dlp |
| `YTDLP_COOKIES_FILE` | Нет | `./cookies.txt` | Путь к файлу cookies для yt-dlp |
| `LOG_LEVEL` | Нет | `info` | Уровень логирования: `debug`, `info`, `warn`, `error` |
| `DEBUG` | Нет | `false` | Подробные ошибки в чатах администраторов |
| `THREADS_ENGINE` | Нет | `default` | Движок Threads: `default` или `lovethreads` |
| `YOUTUBE_ENGINE` | Нет | `default` | Движок YouTube: `default` (yt-dlp) или `savevids` |
| `BROWSER_USER_AGENT` | Нет | Chrome 131 UA | User-Agent для Threads (только `default` движок) |
| `PLUGINS_CONFIG` | Нет | — | Путь к `plugins.yml` для плагинов |

## Команды бота

| Команда | Доступ | Описание |
|---|---|---|
| `/start` | Все | Приветствие и список платформ |
| `/cookies` | Только admin | Показать статус cookies файла |
| `/config` | Только admin | Показать конфигурацию бота |

Для скачивания видео — просто отправьте ссылку.
