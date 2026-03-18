# Cookies для YouTube

YouTube блокирует запросы с IP-адресов дата-центров и требует аутентификацию. Без cookies бот будет возвращать ошибку `Sign in to confirm you're not a bot`.

## Как экспортировать cookies

> Используйте **Firefox** — Chrome инвалидирует cookies сразу после экспорта.

1. Установите расширение **cookies.txt** для Firefox:
   https://addons.mozilla.org/en-US/firefox/addon/cookies-txt/

2. Войдите на `youtube.com` под своим Google аккаунтом

3. Нажмите на иконку расширения → **Current Site** → сохранится файл `youtube.com_cookies.txt`

4. Переименуйте файл в `cookies.txt`

5. **После экспорта не используйте этот браузер для YouTube** — иначе cookies ротируются

## Как передать cookies боту

**Способ 1 — через Telegram (рекомендуется):**

Отправьте файл `cookies.txt` боту в личные сообщения. Бот сохранит его на сервере и ответит подтверждением. Доступно только для пользователей из `ADMIN_USERS`.

**Способ 2 — вручную на сервере:**

```bash
# Скопировать файл в директорию бота
scp cookies.txt user@your-server:/path/to/bot/cookies.txt
# Или через переменную окружения
YTDLP_COOKIES_FILE=/path/to/cookies.txt
```
