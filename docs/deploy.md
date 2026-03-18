# Деплой на Ubuntu-сервер

## Установка зависимостей

```bash
# yt-dlp (актуальная версия — обязательно именно так, не через apt)
sudo curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp
sudo chmod a+rx /usr/local/bin/yt-dlp

# Node.js и ffmpeg
sudo apt-get install -y nodejs ffmpeg golang
```

## Установка и запуск

```bash
git clone git@github.com:amidexe/multiverse-bot.git
cd multiverse-bot
cp .env-example .env
nano .env
go build -o multiverse-bot ./cmd/bot/
./multiverse-bot
```

## systemd (автозапуск)

Создай `/etc/systemd/system/multiverse-bot.service`:

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

## Docker Compose

Для поддержки файлов >50 МБ и встроенного Cobalt — см. [local-bot-api.md](local-bot-api.md).
