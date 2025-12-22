# Конфигурация СКД

Система поддерживает настройку через:
1. **Переменные окружения** (рекомендуется для чувствительных данных)
2. **Файл конфигурации** (`config.json`)
3. **Параметры командной строки**

## Переменные окружения

**Важно**: Чувствительные данные (IP адреса, пароли, пути) должны настраиваться через переменные окружения, а не храниться в коде или файлах конфигурации, которые попадают в git.

### Использование переменных окружения

Скопируйте файл `env.example` в `.env` и заполните значения:

```bash
# Windows (PowerShell)
Copy-Item env.example .env
# Затем отредактируйте .env файл

# Linux/macOS
cp env.example .env
# Затем отредактируйте .env файл
```

**Примечание**: Файл `.env` добавлен в `.gitignore` и не будет попадать в git.

### Основные переменные окружения

- `HTTP_SERVICE_IP` - IP адрес 1C сервиса
- `HTTP_SERVICE_NAME` - имя 1C сервиса
- `HTTP_SERVICE_USER` - логин для авторизации в 1C
- `HTTP_SERVICE_PASS` - пароль для авторизации в 1C
- `TERM_LIST_FILTER` - фильтр терминалов (regex)
- И другие (см. `env.example`)

## Файл конфигурации

По умолчанию система ищет файл `config.json` в той же директории, где находится исполняемый файл.

**Примечание**: Файл `config.json` добавлен в `.gitignore` и не будет попадать в git. Используйте `config.json.example` как шаблон.

### Создание примера конфигурации

Для создания примера файла конфигурации выполните:

```bash
skd.exe -create-config
```

Это создаст файл `config.json` с настройками по умолчанию.

### Структура config.json

```json
{
  "server": {
    "addr": "0.0.0.0",
    "port": 8999
  },
  "web": {
    "addr": "0.0.0.0",
    "port": 8080,
    "enabled": true
  },
  "http_service": {
    "active": true,
    "ip": "virt201.worldclass.nnov.ru",
    "port": 80,
    "name": "virt201.worldclass.nnov.ru",
    "termlist_path": "/gymdb/hs/ACS/terminals",
    "ident_path": "/gymdb/hs/ACS/checking",
    "solar_path": "/gymdb/hs/ACS/solarium",
    "uid_path": "/gymdb/hs/ACS/uid",
    "url_fmt_suff": "wc1c",
    "request_extra_headers": [
      "Authorization: Basic U2VydmljZVNrdWQ6RUE3ODBF"
    ]
  },
  "timeouts": {
    "service_request_expire_time": 5.0,
    "session_expire_time": 300.0,
    "terminal_connect_timeout": 10.0
  },
  "error_handling": {
    "service_autofix_expired": false,
    "service_link_err_msg": "Ошибка связи. Обратитесь на рецепцию.",
    "http_request_retry_count": 2,
    "http_request_retry_delay": 0.5
  },
  "messages": {
    "service_err_msg": "Ошибка связи с БД",
    "service_fixed_msg": "Проходите",
    "service_denied_msg": "Доступ запрещен"
  },
  "jsp": {
    "listener_port": false,
    "dev_auto_ping_enabled": true
  },
  "camera": {
    "result_msg_no": "Лицо не распознано",
    "result_msg_nf": "НЕТ ФОТО !!! Обратитесь в отдел продаж",
    "result_msg_fail": "Ошибка распознавания"
  },
  "terminal_list": {
    "check_time": 60.0,
    "filter": "/192\\.168\\.12\\.2(3|4)(2|3|4|5|6|7|8)/",
    "filter_absent": false
  },
  "logging": {
    "log_file": "log_bin.txt",
    "log_file_screen": "log_screen.txt",
    "log_file_low": "log_low.txt",
    "log_file_err": "log_err.txt",
    "rotation": {
      "enabled": true,
      "max_size": 10485760,
      "max_files": 10,
      "max_days": 30
    }
  },
  "helios": {
    "enabled": false,
    "url": "ws://localhost:8081",
    "timeout": 5.0
  }
}
```

## Параметры командной строки

### Основные опции

- `-config <путь>` - указать путь к файлу конфигурации
- `-create-config` - создать пример файла config.json
- `-help` - показать справку
- `-version` - показать версию

### Параметры в формате ключ=значение

Можно переопределить отдельные параметры через командную строку:

```bash
# Изменить порт сервера и веб-интерфейса
skd.exe server.port=9000 web.port=8081

# Изменить IP 1C сервиса
skd.exe http_service.ip=192.168.1.100

# Изменить фильтр терминалов
skd.exe term_list.filter="/192\\.168\\.12\\.*/"

# Отключить веб-интерфейс
skd.exe web.enabled=false
```

### Поддерживаемые параметры

- `server.addr` / `server_addr` - адрес TCP сервера
- `server.port` / `server_port` - порт TCP сервера
- `web.addr` / `web_addr` - адрес Web интерфейса
- `web.port` / `web_port` - порт Web интерфейса
- `web.enabled` / `web_enabled` - включить/выключить Web интерфейс (true/false)
- `http_service.active` / `http_service_active` - включить/выключить HTTP сервис
- `http_service.ip` / `http_service_ip` - IP адрес 1C сервиса
- `http_service.port` / `http_service_port` - порт 1C сервиса
- `http_service.name` / `http_service_name` - имя 1C сервиса
- `term_list.filter` / `term_list_filter` - фильтр терминалов (regex)
- `term_list.filter_absent` / `term_list_filter_absent` - инвертировать фильтр (true/false)
- `log.file` / `log_file` - файл логов

### Параметры ротации логов

- `logging.rotation.enabled` - включить/выключить ротацию (по умолчанию: true)
- `logging.rotation.max_size` - максимальный размер файла в байтах (по умолчанию: 10485760 = 10 MB)
- `logging.rotation.max_files` - максимальное количество ротированных файлов (по умолчанию: 10)
- `logging.rotation.max_days` - максимальный возраст файлов в днях (по умолчанию: 30)

**Примечание**: Ротация происходит автоматически при достижении максимального размера файла. Старые файлы переименовываются с добавлением timestamp (например, `log_bin_20231220_143025.txt`) и удаляются при превышении лимитов.

## Приоритет настроек

1. **Параметры командной строки** (наивысший приоритет)
2. **Файл конфигурации** (config.json)
3. **Значения по умолчанию** (если ничего не указано)

## Примеры использования

### Использование файла конфигурации

```bash
# Использовать config.json из текущей директории
skd.exe

# Использовать другой файл конфигурации
skd.exe -config /path/to/myconfig.json
```

### Переопределение через параметры

```bash
# Запуск с другим портом веб-интерфейса
skd.exe web.port=9090

# Запуск с другим IP 1C сервиса и фильтром терминалов
skd.exe http_service.ip=192.168.1.50 term_list.filter="/192\\.168\\.1\\.*/"
```

### Создание конфигурации для разных окружений

```bash
# Создать конфигурацию
skd.exe -create-config

# Отредактировать config.json для вашего окружения
# Затем запустить
skd.exe
```

## Важные настройки

### Фильтр терминалов

Фильтр терминалов использует регулярные выражения. Примеры:

- `/192\.168\.12\.2(3|4)(2|3|4|5|6|7|8)/` - терминалы с IP 192.168.12.23x-28x
- `/192\.168\.12\.*/` - все терминалы в подсети 192.168.12.*
- `/192\.168\.(12|13)\.*/` - терминалы в подсетях 192.168.12.* и 192.168.13.*

### Авторизация 1C

Для авторизации в 1C используется Basic Authentication. Значение в `request_extra_headers` должно быть закодировано в Base64:

```
Authorization: Basic <base64(логин:пароль)>
```

Пример: `ServiceSkud:EA780E` → `U2VydmljZVNrdWQ6RUE3ODBF`

## Ротация логов

Система поддерживает автоматическую ротацию логов для предотвращения переполнения диска.

### Параметры ротации

- **enabled** (bool) - включить/выключить ротацию логов
- **max_size** (int) - максимальный размер файла в байтах. При достижении этого размера файл ротируется
- **max_files** (int) - максимальное количество ротированных файлов. Старые файлы удаляются при превышении
- **max_days** (int) - максимальный возраст файлов в днях. Файлы старше указанного количества дней удаляются

### Примеры конфигурации

```json
{
  "logging": {
    "log_file": "log_bin.txt",
    "rotation": {
      "enabled": true,
      "max_size": 10485760,
      "max_files": 10,
      "max_days": 30
    }
  }
}
```

### Как работает ротация

1. При записи в лог-файл система проверяет его размер
2. Если размер превышает `max_size`, текущий файл переименовывается с добавлением timestamp
3. Создается новый файл с оригинальным именем
4. Старые ротированные файлы удаляются, если:
   - Количество файлов превышает `max_files`
   - Возраст файла превышает `max_days`

### Формат имен ротированных файлов

Ротированные файлы получают имена в формате:
```
<оригинальное_имя>_<YYYYMMDD>_<HHMMSS>.<расширение>
```

Пример:
- `log_bin.txt` → `log_bin_20231220_143025.txt`
- `log_screen.txt` → `log_screen_20231220_143025.txt`

