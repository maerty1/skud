# JSP Protocol Implementation

## Описание

JSP (JavaScript Protocol) - это JSON-based протокол для связи с контроллерами доступа (дверей) на базе JavaScript терминалов.

## Формат пакетов

### Структура пакета
```
<SOF><LENGTH_HEX><JSON_DATA><EOF>
```

- **SOF** (Start of Frame): `0x03`
- **LENGTH_HEX**: 4 hex символа (длина JSON в байтах)
- **JSON_DATA**: JSON объект с данными команды/ответа
- **EOF** (End of Frame): `0x02`

### Пример пакета
```
0x03 000F {"cmd":"ping","rid":"RID000001"} 0x02
```

## Основные команды

### От терминала к серверу

#### `tag_read` - Чтение RFID карты
```json
{
  "cmd": "tag_read",
  "tid": "terminal_id",
  "uid": "04AEECFA9B2680",
  "rid": "RID000001",
  "lockers_data": "62:180,33:26"
}
```

#### `pass_report` - Отчет о проходе
```json
{
  "cmd": "pass_report",
  "sid": "session_id",
  "uid": "04AEECFA9B2680",
  "passed": true
}
```

#### `pong` - Ответ на ping
```json
{
  "cmd": "pong",
  "rid": "RID000001"
}
```

### От сервера к терминалу

#### `relay_open` - Открыть реле
```json
{
  "cmd": "relay_open",
  "uid": "04AEECFA9B2680",
  "caption": "Проходите",
  "time": 3000,
  "cid": "client_id",
  "rid": "RID000001"
}
```

#### `relay_close` - Закрыть реле
```json
{
  "cmd": "relay_close",
  "rid": "RID000001"
}
```

#### `message` - Отправить сообщение
```json
{
  "cmd": "message",
  "text": "Доступ запрещен",
  "time": 1500,
  "rid": "RID000001"
}
```

#### `ping` - Проверка соединения
```json
{
  "cmd": "ping",
  "rid": "RID000001"
}
```

## Request/Response система

Каждый запрос имеет уникальный `rid` (Request ID) в формате `RIDXXXXXX` (6 hex символов).

Ответы приходят с тем же `rid`:
```json
{
  "rid": "RID000001",
  "result": true,
  "message": "OK"
}
```

## Auto-ping

Система автоматически отправляет ping каждые N секунд для проверки соединения:
- **Ping Interval**: Интервал между ping (по умолчанию 10 сек)
- **Ping Timeout**: Таймаут ожидания pong (по умолчанию 15 сек)

Если pong не получен в течение timeout, соединение закрывается.

## Обработка lockers_data

Поле `lockers_data` может быть строкой или массивом:
- **Строка**: `"62:180,33:26"` - преобразуется в массив объектов
- **Массив**: Уже в правильном формате

Формат объекта шкафчика:
```json
{
  "auth_err": 0,
  "read_err": 0,
  "is_passtech": false,
  "block_no": 62,
  "locked": true,
  "cab_no": 180
}
```

## Интеграция с сессиями

JSP протокол полностью интегрирован с системой сессий:
- `tag_read` → создает новую сессию доступа
- `pass_report` → обновляет состояние сессии
- Ответы на запросы доступа → отправляются через JSP команды

## Примеры использования

### Отправка команды открытия реле
```go
packet, err := jsp.SendRelayOpen(&ridCounter, uid, "Проходите", 3000, cid)
pool.Send(connKey, packet)
```

### Обработка входящего пакета
```go
data, err := jsp.TryReadPacket(jspConn)
if err == nil {
    packetType, packet, _ := jsp.ProcessPacket(data.(map[string]interface{}))
    // Обработать пакет
}
```

