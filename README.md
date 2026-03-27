# Multiverse Bot

Telegram-бот для скачивания видео с популярных платформ.

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go)

## Что умеет

- Качать с YouTube, Instagram, X (twitter) и Threads
- Instagram: качать картинки из поста
- YouTube: следить за выходом новых видео на канале и давать возможность их скачать

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
- [Cookies для YouTube](docs/cookies.md) — обязательно для серверного деплоя (загружаются через бота)
- [Cookies для Instagram](docs/instagram-cookies.md) — для закрытых постов, Reels и сторис (загружаются через бота)
- [Деплой на Ubuntu](docs/deploy.md) — systemd, зависимости
- [Файлы >50 МБ (Local Bot API)](docs/local-bot-api.md) — снятие ограничения Telegram
- [Google Drive](docs/google-drive.md) — сохранение видео в личный Drive
- [Плагины](PLUGINS.md) — расширение бота через внешние HTTP-сервисы
- [Разработка](DEV.md) — сборка, тесты, линтер
