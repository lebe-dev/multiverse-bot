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

```bash
cp .env-example .env

# Редактируешь .env под свои нужды

# Запускаешь
docker compose up -d
```

Для скачивания видео — просто отправьте ссылку боту.

## Документация

- [Конфигурация](docs/configuration.md) — все переменные окружения и команды бота
- [Cookies для YouTube](docs/cookies.md) — обязательно для серверного деплоя
- [Деплой на Ubuntu](docs/deploy.md) — systemd, зависимости
- [Файлы >50 МБ (Local Bot API)](docs/local-bot-api.md) — снятие ограничения Telegram
- [Google Drive](docs/google-drive.md) — сохранение видео в личный Drive
- [Плагины](PLUGINS.md) — расширение бота через внешние HTTP-сервисы
- [Архитектура](ARCH.md) — устройство кода
- [Разработка](DEV.md) — сборка, тесты, линтер
