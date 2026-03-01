## Разработка

### Линтинг кода

```bash
just lint
```

### Добавление новой платформы

1. Реализуйте интерфейс `domain.Downloader` в новом пакете `internal/adapter/downloader/newplatform/`
2. Добавьте логику определения платформы в `internal/adapter/detector/`
3. Зарегистрируйте новый бэкенд в `cmd/bot/main.go`

## Источники

- https://cobalt.tools
