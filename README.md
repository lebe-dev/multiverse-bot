# Multiverse Bot

Telegram-бот для скачивания видео с популярных платформ.

![Go](https://img.shields.io/badge/Go-1.20+-00ADD8?style=flat-square&logo=go)

## Описание

**Multiverse Bot** — это Telegram-бот, который позволяет пользователям скачивать видео с YouTube, Instagram, X (Twitter) и Threads. Просто отправьте ссылку боту в личные сообщения, и он вернёт вам видеофайл.

Бот полностью настраивается через переменные окружения и готов к развертыванию в Docker.

## Быстрый старт

### Предварительные требования

- Go 1.26+
- [Just](https://github.com/casey/just) (для управления командами)

### Установка

1. **Клонируйте репозиторий**

```bash
git clone https://github.com/yourusername/multiverse-bot.git
cd multiverse-bot
```

2. **Создайте файл конфигурации**

```bash
cp .env-example .env
```

3. **Установите переменные окружения**

```bash
TELEGRAM_BOT_TOKEN=your_token_from_botfather
LOG_LEVEL=info
ALLOWED_USERS=optional_usernames
COBALT_API_URL=https://api.cobalt.tools
```

4. **Запустите бота локально**

```bash
just run
```

## Конфигурация

Все настройки задаются через переменные окружения в файле `.env`:

| Переменная | Требуется | По умолчанию | Описание |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | Да | — | Токен бота от BotFather |
| `ALLOWED_USERS` | ❌ Нет | (все) | Список разрешённых пользователей (запятая-разделитель) |
| `COBALT_API_URL` | ❌ Нет | `https://api.cobalt.tools` | URL Cobalt API для видео с социальных сетей |
| `LOG_LEVEL` | ❌ Нет | `info` | Уровень логирования: `debug`, `info`, `warn`, `error` |

### Получение токена

1. Найдите в Telegram бота **@BotFather**
2. Отправьте команду `/newbot`
3. Скопируйте полученный токен в `TELEGRAM_BOT_TOKEN`

## Команды

Проект использует `Justfile` для управления командами:

```bash
just run             # Запустить бота локально
just build           # Собрать бинарный файл в bin/multiverse-bot
just test            # Запустить все тесты
just test ./path/... # Запустить тесты конкретного пакета
just lint            # Запустить golangci-lint
just docker-build    # Собрать Docker образ
just docker-up       # Запустить с docker-compose
just docker-down     # Остановить docker-compose
just docker-logs     # Просмотреть логи бота
```

## 🏗️ Архитектура

Проект следует принципам **Clean Architecture** с разделением на три слоя:

```
internal/
  domain/           # Интерфейсы и типы данных (без зависимостей)
  usecase/          # Бизнес-логика (VideoService)
  adapter/          # Реализации интерфейсов
    config/         # Загрузка конфигурации из переменных
    detector/       # Определение платформы по URL (regex)
    downloader/
      ytdlp/        # Бэкенд для YouTube (yt-dlp)
      cobalt/       # Бэкенд Cobalt API (Instagram, Twitter, Threads)
      composite/    # Итератор: пробует все бэкенды по порядку
    telegram/       # Telegram бот (telebot.v4), обработчики
cmd/bot/main.go     # Точка входа, связывание компонентов
```

### Ключевые решения

- **Домен**: Интерфейсы `domain.Downloader` и `domain.PlatformDetector` определены в `internal/domain/` и реализованы в адаптерах
- **Composite downloader**: Последовательно пробует бэкенды (сначала yt-dlp, потом Cobalt)
- **Управление ресурсами**: `VideoService.ProcessURL` возвращает функцию очистки для удаления временных файлов
- **Лимит размера**: Максимум 50 МБ (ограничение Telegram Bot API)
- **Фильтрация пользователей**: `ALLOWED_USERS` работает на уровне Telegram обработчиков

## Docker

### Собрать образ

```bash
just docker-build
```

### Запустить с docker-compose

```bash
just docker-up
```

### Остановить

```bash
just docker-down
```

### Просмотреть логи

```bash
just docker-logs
```

## Тестирование

```bash
# Все тесты
just test

# Конкретный пакет
just test ./internal/usecase/...

# С логированием
LOG_LEVEL=debug just test
```

## Использование

1. Найдите бота в Telegram (по имени от BotFather)
2. Отправьте ссылку на видео с любой из поддерживаемых платформ:
   - YouTube
   - Instagram
   - X (Twitter)
   - Threads
3. Бот обработает видео и отправит вам файл

Если вы установили `ALLOWED_USERS`, только указанные пользователи смогут использовать бота.

## Разработка

Смотри [DEV.md](DEV.md).
