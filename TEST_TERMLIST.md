# Тестирование получения списка терминалов из 1C

## Описание

Система поддерживает автоматическое и ручное получение списка терминалов из 1C через HTTP API.

## Конфигурация

В `config/config.go` настроены параметры подключения к 1C (соответствуют PHP конфигурации):

```go
HTTPServiceActive:       true,
HTTPServiceIP:           "virt201.worldclass.nnov.ru",
HTTPServicePort:         80,
HTTPServiceName:         "virt201.worldclass.nnov.ru",
HTTPServiceTermlistPath: "/gymdb/hs/ACS/terminals",
HTTPServiceIdentPath:    "/gymdb/hs/ACS/checking",
HTTPServiceSolarPath:    "/gymdb/hs/ACS/solarium",
HTTPServiceUIDPath:      "/gymdb/hs/ACS/uid",
HTTPServiceUrlFmtSuff:   "wc1c",
ServiceRequestExpireTime: 5.0,
TermListFilter:          `/192\.168\.12\.2(3|4)(2|3|4|5|6|7|8)/`,
TermListFilterAbsent:    false,
```

**Авторизация**: Basic Auth с учетными данными `ServiceSkud:EA780E` (настроена автоматически)

**Фильтр терминалов**: Регулярное выражение для фильтрации терминалов по IP адресу
- `TermListFilter`: Регулярное выражение (формат PHP: `/pattern/`)
- `TermListFilterAbsent`: `false` = включать только совпадающие, `true` = исключать совпадающие

Пример фильтра `/192\.168\.12\.2(3|4)(2|3|4|5|6|7|8)/` включает терминалы:
- 192.168.12.232-238
- 192.168.12.242-248

## Автоматическая проверка

Система автоматически запрашивает список терминалов каждые 60 секунд (настраивается через `TermListCheckTime`).

При получении списка терминалов система:
1. Парсит ответ от 1C
2. **Применяет фильтр по IP адресу** (если настроен)
3. Создает соединения только с отфильтрованными терминалами
4. Логирует процесс в `logs/log_bin.txt`

**Пример лога с фильтрацией:**
```
[INFO] Received 10 terminals from 1C
[INFO] Processed 8 terminals after filtering (from 10 total)
```

## Ручная проверка

### Через TCP команду

```bash
# Отправить команду через ncat или telnet
echo "system check_db" | ncat localhost 8999
```

Или используйте готовый скрипт:

**PowerShell:**
```powershell
.\test_termlist.ps1
```

**Batch:**
```batch
.\test_termlist.bat
```

### Через веб-интерфейс

Откройте `http://localhost:8080` и проверьте список активных соединений.

## Форматы ответа от 1C

Система поддерживает несколько форматов ответа:

1. **Объект с массивом terminals:**
```json
{
  "terminals": [
    {"ID": "1", "IP": "192.168.1.100", "PORT": 8080, "TYPE": "pocket"}
  ]
}
```

2. **Объект с массивом DEVICES:**
```json
{
  "DEVICES": [
    {"ID": "1", "IP": "192.168.1.100", "PORT": 8080}
  ]
}
```

3. **Прямой массив:**
```json
[
  {"ID": "1", "IP": "192.168.1.100", "PORT": 8080},
  {"id": "2", "ip": "192.168.1.101", "port": 8080}
]
```

## Поля терминала

Система автоматически нормализует поля (поддерживает как uppercase, так и lowercase):

- **ID/id** - идентификатор терминала (обязательно)
- **IP/ip** - IP адрес терминала (обязательно)
- **PORT/port** - порт терминала (по умолчанию 8080)
- **TYPE/type** - тип терминала: `gat`, `pocket`, `sphinx`, `jsp` (по умолчанию `gat`)

## Логирование

Все операции логируются в:
- `logs/log_bin.txt` - основной лог
- `logs/log_screen.txt` - экранный лог

Пример лога:
```
[INFO] Checking terminal list...
[INFO] Received 5 terminals from 1C
[INFO] Processing terminal: 1 (192.168.1.100:8080, type: pocket)
```

## Обработка ошибок

Если получение списка терминалов не удалось:
- Проверьте доступность 1C сервера
- Проверьте правильность пути `HTTPServiceTermlistPath`
- Проверьте авторизацию (если требуется)
- Проверьте формат ответа от 1C

Ошибки логируются с уровнем `ERROR`:
```
[ERROR] Failed to get terminal list: failed to get terminal list: ...
```

## Пример успешного ответа

```
Terminal list check successful. Found 3 terminals:
  1. ID: 1, IP: 192.168.1.100, Port: 8080
  2. ID: 2, IP: 192.168.1.101, Port: 8080
  3. ID: 3, IP: 192.168.1.102, Port: 8080
```

