# Google Drive

Бот умеет сохранять скачанные видео на Google Drive пользователя. Каждый пользователь авторизуется отдельно через команду `/auth` — файлы сохраняются в его личный Drive.

## Пошаговая настройка OAuth

1. Зайди на `console.cloud.google.com`

2. **Создай проект** (или выбери существующий) через меню вверху страницы

3. **Включи Google Drive API:**
   - Слева `APIs & Services` → `Enable APIs and Services`
   - Найди `Google Drive API` → нажми `Enable`

4. **Настрой OAuth consent screen:**
   - `APIs & Services` → `OAuth consent screen`
   - User Type: `External` → `Create`
   - Заполни название приложения (например, `multiverse-bot`)
   - В разделе `Scopes` → `Add or remove scopes` → найди и добавь `drive.file`
   - В разделе `Test users` → добавь свой Google-аккаунт
   - Сохрани

5. **Создай OAuth credentials:**
   - `APIs & Services` → `Credentials` → `Create Credentials` → `OAuth client ID`
   - Тип: **TV and Limited Input devices** (важно именно этот — не требует redirect URI)
   - Название: любое
   - Нажми `Create`

6. **Скопируй `Client ID` и `Client Secret`** и пропиши в `.env`:

```bash
GOOGLE_CLIENT_ID=your_client_id
GOOGLE_CLIENT_SECRET=your_client_secret
```

7. Перезапусти бота.

## Использование

- `/auth` — привязать свой Google Drive (бот выдаст ссылку и код для входа)
- После авторизации каждое скачанное видео бот дополнительно сохранит в Drive

> Бот запрашивает только разрешение `drive.file` — доступ только к файлам, созданным самим ботом. Остальные файлы Drive недоступны.
