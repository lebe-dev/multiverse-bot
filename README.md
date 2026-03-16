# Multiverse Bot

Telegram-бот для скачивания видео с популярных платформ.

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)

## Поддерживаемые платформы

| Платформа | Бэкенд |
|---|---|
| YouTube | yt-dlp |
| Instagram | Cobalt API |
| X (Twitter) | Cobalt API |
| Threads | yt-dlp |

## Быстрый старт

### Требования

- Go 1.26+
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) (последняя версия)
- Node.js 20+ (для решения YouTube JS-challenge)
- ffmpeg

### Установка зависимостей (Ubuntu/Debian)

```bash
# yt-dlp (актуальная версия — обязательно именно так, не через apt)
sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp
sudo chmod a+rx /usr/local/bin/yt-dlp

# Node.js и ffmpeg
sudo apt-get install -y nodejs ffmpeg
```

### Установка бота

1. **Клонируйте репозиторий**

```bash
git clone git@github.com:amidexe/multiverse-bot.git
cd multiverse-bot
```

2. **Создайте файл конфигурации**

```bash
cp .env-example .env
```

3. **Настройте переменные окружения в `.env`**

```bash
TELEGRAM_BOT_TOKEN=your_token_from_botfather
ADMIN_USERS=your_telegram_username
LOG_LEVEL=info
```

4. **Соберите и запустите**

```bash
go build -o multiverse-bot ./cmd/bot/
./multiverse-bot
```

## Конфигурация

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | Да | — | Токен бота от BotFather |
| `ADMIN_USERS` | Нет | — | Telegram username(ы) администраторов через запятую |
| `ALLOWED_USERS` | Нет | (все) | Список разрешённых пользователей через запятую |
| `COBALT_API_URL` | Нет | `https://api.cobalt.tools` | URL Cobalt API |
| `YTDLP_COOKIES_FILE` | Нет | `./cookies.txt` | Путь к файлу cookies для yt-dlp |
| `LOG_LEVEL` | Нет | `info` | Уровень логирования: `debug`, `info`, `warn`, `error` |

## Команды бота

| Команда | Доступ | Описание |
|---|---|---|
| `/start` | Все | Приветствие и список платформ |
| `/cookies` | Только admin | Показать статус cookies файла |
| `/config` | Только admin | Показать конфигурацию бота |

Для скачивания видео — просто отправьте ссылку.

## Cookies для YouTube (важно для серверного деплоя)

YouTube блокирует запросы с IP-адресов дата-центров и требует аутентификацию. Без cookies бот будет возвращать ошибку `Sign in to confirm you're not a bot`.

### Как экспортировать cookies

> Используйте **Firefox** — Chrome инвалидирует cookies сразу после экспорта.

1. Установите расширение **cookies.txt** для Firefox:
   https://addons.mozilla.org/en-US/firefox/addon/cookies-txt/

2. Войдите на `youtube.com` под своим Google аккаунтом

3. Нажмите на иконку расширения → **Current Site** → сохранится файл `youtube.com_cookies.txt`

4. Переименуйте файл в `cookies.txt`

5. **После экспорта не используйте этот браузер для YouTube** — иначе cookies ротируются

### Как передать cookies боту

**Способ 1 — через Telegram (рекомендуется):**

Отправьте файл `cookies.txt` боту в личные сообщения. Бот сохранит его на сервере и ответит подтверждением. Доступно только для пользователей из `ADMIN_USERS`.

**Способ 2 — вручную на сервере:**

```bash
# Скопировать файл в директорию бота
scp cookies.txt user@your-server:/path/to/bot/cookies.txt
# Или через переменную окружения
YTDLP_COOKIES_FILE=/path/to/cookies.txt
```

## Деплой на Ubuntu-сервер

```bash
# Клонировать репозиторий
git clone git@github.com:amidexe/multiverse-bot.git
cd multiverse-bot

# Установить зависимости
sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp
sudo chmod a+rx /usr/local/bin/yt-dlp
sudo apt-get install -y nodejs ffmpeg golang

# Собрать
go build -o multiverse-bot ./cmd/bot/

# Создать .env
cp .env-example .env
nano .env

# Запустить
./multiverse-bot
```

### systemd (для автозапуска)

Создайте `/etc/systemd/system/multiverse-bot.service`:

```ini
[Unit]
Description=Multiverse Telegram Bot
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/opt/multiverse-bot
EnvironmentFile=/opt/multiverse-bot/.env
ExecStart=/opt/multiverse-bot/multiverse-bot
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable multiverse-bot
sudo systemctl start multiverse-bot
sudo systemctl status multiverse-bot
```

## Архитектура

```
internal/
  domain/           # Интерфейсы и типы данных
  usecase/          # Бизнес-логика (VideoService)
  adapter/
    config/         # Загрузка конфигурации из env
    detector/       # Определение платформы по URL
    downloader/
      ytdlp/        # Бэкенд yt-dlp (YouTube, Threads)
      cobalt/       # Бэкенд Cobalt API (Instagram, Twitter)
      composite/    # Пробует бэкенды по порядку
    telegram/       # Telegram бот (telebot.v4)
cmd/bot/main.go     # Точка входа
```

## Разработка

```bash
just run          # запуск локально
just build        # сборка бинарника
just test         # тесты
just lint         # линтер
just docker-up    # запуск в Docker
just docker-logs  # логи
```

Смотри [DEV.md](DEV.md).

## Changelog

### Текущая версия (форк от lebe-dev/multiverse-bot)

- **fix:** убран YouTube из поддерживаемых платформ Cobalt — он возвращал `error.api.youtube.login`; YouTube обрабатывается только через yt-dlp
- **fix:** ошибка компиляции в `handler.go` — `c.Send()` в telebot.v4 возвращает 1 значение
- **fix:** yt-dlp теперь использует Node.js как JS-runtime (`--js-runtimes node`) для решения YouTube signature/n-challenge на серверных IP
- **feat:** управление cookies через Telegram — команда `/cookies` и загрузка файла прямо в чат с ботом
- **feat:** переменная `YTDLP_COOKIES_FILE` для указания пути к cookies
