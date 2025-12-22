package daemon

import (
	"net/http"
)

// handleWebIndex serves the main web page with modern Russian interface
func (d *Daemon) handleWebIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	html := `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>СКД - Система контроля доступа</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        :root {
            --bg-primary: #ffffff;
            --bg-secondary: #f8f9fa;
            --bg-card: #ffffff;
            --text-primary: #1a1a1a;
            --text-secondary: #6c757d;
            --border-color: #e9ecef;
            --accent: #0d6efd;
            --accent-hover: #0b5ed7;
            --success: #198754;
            --warning: #ffc107;
            --danger: #dc3545;
            --info: #0dcaf0;
            --shadow: 0 2px 8px rgba(0,0,0,0.08);
            --shadow-lg: 0 4px 16px rgba(0,0,0,0.12);
            --radius: 12px;
            --radius-sm: 8px;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Oxygen', 'Ubuntu', 'Cantarell', 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
            color: var(--text-primary);
            line-height: 1.6;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
        }

        .header {
            background: var(--bg-card);
            border-radius: var(--radius);
            padding: 30px;
            margin-bottom: 30px;
            box-shadow: var(--shadow-lg);
            backdrop-filter: blur(10px);
        }

        .header h1 {
            font-size: 2.5rem;
            font-weight: 700;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            margin-bottom: 10px;
        }

        .header p {
            color: var(--text-secondary);
            font-size: 1.1rem;
        }

        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }

        .stat-card {
            background: var(--bg-card);
            border-radius: var(--radius);
            padding: 25px;
            box-shadow: var(--shadow);
            transition: transform 0.2s, box-shadow 0.2s;
            border-left: 4px solid var(--accent);
        }

        .stat-card:hover {
            transform: translateY(-4px);
            box-shadow: var(--shadow-lg);
        }

        .stat-card.success { border-left-color: var(--success); }
        .stat-card.warning { border-left-color: var(--warning); }
        .stat-card.danger { border-left-color: var(--danger); }
        .stat-card.info { border-left-color: var(--info); }

        .stat-label {
            font-size: 0.875rem;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 8px;
            font-weight: 600;
        }

        .stat-value {
            font-size: 2rem;
            font-weight: 700;
            color: var(--text-primary);
        }

        .stat-icon {
            font-size: 2.5rem;
            margin-bottom: 10px;
            opacity: 0.8;
        }

        .card {
            background: var(--bg-card);
            border-radius: var(--radius);
            padding: 25px;
            margin-bottom: 30px;
            box-shadow: var(--shadow);
        }

        .card-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            padding-bottom: 15px;
            border-bottom: 2px solid var(--border-color);
        }

        .card-title {
            font-size: 1.5rem;
            font-weight: 600;
            color: var(--text-primary);
        }

        .btn {
            padding: 10px 20px;
            border: none;
            border-radius: var(--radius-sm);
            font-size: 0.9rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s;
            text-decoration: none;
            display: inline-block;
        }

        .btn-primary {
            background: var(--accent);
            color: white;
        }

        .btn-primary:hover {
            background: var(--accent-hover);
            transform: translateY(-2px);
            box-shadow: var(--shadow);
        }

        .btn-secondary {
            background: var(--bg-secondary);
            color: var(--text-primary);
            border: 1px solid var(--border-color);
        }

        .btn-secondary:hover {
            background: var(--border-color);
        }

        .btn-danger {
            background: var(--danger);
            color: white;
        }

        .btn-danger:hover {
            background: #bb2d3b;
        }

        .btn-group {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }

        .table-container {
            overflow-x: auto;
            border-radius: var(--radius-sm);
        }

        table {
            width: 100%;
            border-collapse: collapse;
        }

        thead {
            background: var(--bg-secondary);
        }

        th {
            padding: 15px;
            text-align: left;
            font-weight: 600;
            color: var(--text-primary);
            font-size: 0.875rem;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }

        td {
            padding: 15px;
            border-top: 1px solid var(--border-color);
        }

        tbody tr {
            transition: background 0.2s;
        }

        tbody tr:hover {
            background: var(--bg-secondary);
        }

        .badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 20px;
            font-size: 0.75rem;
            font-weight: 600;
            text-transform: uppercase;
        }

        .badge-success {
            background: #d1e7dd;
            color: #0f5132;
        }

        .badge-danger {
            background: #f8d7da;
            color: #842029;
        }

        .badge-warning {
            background: #fff3cd;
            color: #664d03;
        }

        .badge-secondary {
            background: #e2e3e5;
            color: #41464b;
        }

        .badge-warning {
            background: #fff3cd;
            color: #664d03;
        }

        .badge-info {
            background: #cff4fc;
            color: #055160;
        }

        .status-indicator {
            display: inline-block;
            width: 12px;
            height: 12px;
            border-radius: 50%;
            margin-right: 8px;
        }

        .status-indicator.running {
            background: var(--success);
            box-shadow: 0 0 8px var(--success);
        }

        .status-indicator.stopped {
            background: var(--danger);
        }

        .logs-container {
            max-height: 500px;
            overflow-y: auto;
            background: var(--bg-secondary);
            border-radius: var(--radius-sm);
            padding: 15px;
            font-family: 'Courier New', monospace;
            font-size: 0.875rem;
        }

        .log-entry {
            padding: 8px;
            margin-bottom: 4px;
            border-radius: 4px;
            border-left: 3px solid transparent;
        }

        .log-entry.error { border-left-color: var(--danger); background: #fff5f5; }
        .log-entry.warn { border-left-color: var(--warning); background: #fffbf0; }
        .log-entry.info { border-left-color: var(--info); background: #f0f9ff; }

        .log-time {
            color: var(--text-secondary);
            margin-right: 10px;
        }

        .log-level {
            font-weight: 700;
            margin-right: 10px;
        }

        .log-level.error { color: var(--danger); }
        .log-level.warn { color: #e67e22; }
        .log-level.info { color: var(--info); }

        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.5);
            z-index: 1000;
            backdrop-filter: blur(4px);
            align-items: center;
            justify-content: center;
        }

        .modal.active {
            display: flex;
        }

        .modal-content {
            background: var(--bg-card);
            border-radius: var(--radius);
            padding: 30px;
            max-width: 500px;
            width: 90%;
            box-shadow: var(--shadow-lg);
            animation: slideIn 0.3s;
        }

        @keyframes slideIn {
            from {
                transform: translateY(-50px);
                opacity: 0;
            }
            to {
                transform: translateY(0);
                opacity: 1;
            }
        }

        .form-group {
            margin-bottom: 20px;
        }

        .form-group label {
            display: block;
            margin-bottom: 8px;
            font-weight: 600;
            color: var(--text-primary);
        }

        .form-group input {
            width: 100%;
            padding: 12px;
            border: 2px solid var(--border-color);
            border-radius: var(--radius-sm);
            font-size: 1rem;
            transition: border-color 0.2s;
        }

        .form-group input:focus {
            outline: none;
            border-color: var(--accent);
        }

        .empty-state {
            text-align: center;
            padding: 40px;
            color: var(--text-secondary);
        }

        .empty-state-icon {
            font-size: 4rem;
            margin-bottom: 20px;
            opacity: 0.3;
        }

        .filter-group {
            display: flex;
            gap: 10px;
            margin-bottom: 20px;
            flex-wrap: wrap;
        }

        .filter-group select,
        .filter-group input {
            padding: 10px;
            border: 2px solid var(--border-color);
            border-radius: var(--radius-sm);
            font-size: 0.9rem;
        }

        @media (max-width: 768px) {
            .stats-grid {
                grid-template-columns: 1fr;
            }
            
            .card-header {
                flex-direction: column;
                align-items: flex-start;
                gap: 15px;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🚪 СКД</h1>
            <p>Система контроля доступа - Мониторинг и управление</p>
        </div>

        <div class="stats-grid">
            <div class="stat-card success">
                <div class="stat-icon">🟢</div>
                <div class="stat-label">Статус</div>
                <div class="stat-value">
                    <span class="status-indicator" id="status-indicator"></span>
                    <span id="status">Загрузка...</span>
                </div>
            </div>
            <div class="stat-card info">
                <div class="stat-icon">⏱️</div>
                <div class="stat-label">Время работы</div>
                <div class="stat-value" id="uptime">0 сек</div>
            </div>
            <div class="stat-card">
                <div class="stat-icon">🔌</div>
                <div class="stat-label">Соединения</div>
                <div class="stat-value" id="connections-count">0</div>
            </div>
            <div class="stat-card warning">
                <div class="stat-icon">🔄</div>
                <div class="stat-label">Переподключения</div>
                <div class="stat-value" id="reconnections-count">0</div>
            </div>
            <div class="stat-card">
                <div class="stat-icon">📡</div>
                <div class="stat-label">HTTP запросы</div>
                <div class="stat-value" id="http-requests-count">0</div>
            </div>
            <div class="stat-card">
                <div class="stat-icon">👤</div>
                <div class="stat-label">Активные сессии</div>
                <div class="stat-value" id="sessions-count">0</div>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <h2 class="card-title">📊 Активные соединения</h2>
                <div class="btn-group">
                    <button class="btn btn-primary" onclick="loadConnections()">🔄 Обновить</button>
                </div>
            </div>
            <div class="table-container">
                <table>
                    <thead>
                        <tr>
                            <th>Ключ</th>
                            <th>IP:Порт</th>
                            <th>Тип</th>
                            <th>Подключен</th>
                            <th>Последняя активность</th>
                        </tr>
                    </thead>
                    <tbody id="connections-body">
                        <tr><td colspan="5" class="empty-state">Загрузка...</td></tr>
                    </tbody>
                </table>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <h2 class="card-title">👥 Активные сессии</h2>
                <div class="btn-group">
                    <button class="btn btn-primary" onclick="loadSessions()">🔄 Обновить</button>
                </div>
            </div>
            <div class="table-container">
                <table>
                    <thead>
                        <tr>
                            <th>ID</th>
                            <th>Ключ</th>
                            <th>Пользователь</th>
                            <th>Этап</th>
                            <th>Время запроса</th>
                            <th>Действия</th>
                        </tr>
                    </thead>
                    <tbody id="sessions-body">
                        <tr><td colspan="6" class="empty-state">Загрузка...</td></tr>
                    </tbody>
                </table>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <h2 class="card-title">🖥️ Терминалы</h2>
                <div class="btn-group">
                    <button class="btn btn-primary" onclick="loadTerminals()">🔄 Обновить</button>
                    <button class="btn btn-secondary" onclick="showConnectDialog()">➕ Подключить терминал</button>
                </div>
            </div>
            <div class="table-container">
                <table>
                    <thead>
                        <tr>
                            <th>Ключ</th>
                            <th>IP:Порт</th>
                            <th>Тип</th>
                            <th>Подключен</th>
                            <th>Последняя активность</th>
                            <th>Действия</th>
                        </tr>
                    </thead>
                    <tbody id="terminals-body">
                        <tr><td colspan="6" class="empty-state">Загрузка...</td></tr>
                    </tbody>
                </table>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <h2 class="card-title">📝 Логи</h2>
                <div class="btn-group">
                    <button class="btn btn-primary" onclick="loadLogs()">🔄 Обновить</button>
                </div>
            </div>
            <div class="filter-group">
                <select id="log-level" onchange="loadLogs()">
                    <option value="">Все уровни</option>
                    <option value="INFO">INFO</option>
                    <option value="WARN">WARN</option>
                    <option value="ERROR">ERROR</option>
                </select>
                <input type="number" id="log-limit" value="100" min="1" max="1000" placeholder="Лимит записей" onchange="loadLogs()">
            </div>
            <div class="logs-container" id="logs-body">
                Загрузка...
            </div>
        </div>
    </div>

    <div id="connect-dialog" class="modal">
        <div class="modal-content">
            <h2 style="margin-bottom: 20px;">🔌 Подключить терминал</h2>
            <div class="form-group">
                <label>IP адрес:</label>
                <input type="text" id="connect-ip" placeholder="192.168.1.100">
            </div>
            <div class="form-group">
                <label>Порт:</label>
                <input type="number" id="connect-port" placeholder="8080">
            </div>
            <div class="btn-group" style="margin-top: 20px;">
                <button class="btn btn-primary" onclick="connectTerminal()">Подключить</button>
                <button class="btn btn-secondary" onclick="hideConnectDialog()">Отмена</button>
            </div>
        </div>
    </div>

    <script>
        function formatUptime(seconds) {
            const days = Math.floor(seconds / 86400);
            const hours = Math.floor((seconds % 86400) / 3600);
            const mins = Math.floor((seconds % 3600) / 60);
            const secs = Math.floor(seconds % 60);
            
            if (days > 0) return days + 'д ' + hours + 'ч ' + mins + 'м';
            if (hours > 0) return hours + 'ч ' + mins + 'м ' + secs + 'с';
            if (mins > 0) return mins + 'м ' + secs + 'с';
            return secs + 'с';
        }

        function formatDate(timestamp) {
            const date = new Date(timestamp);
            return date.toLocaleString('ru-RU', {
                day: '2-digit',
                month: '2-digit',
                year: 'numeric',
                hour: '2-digit',
                minute: '2-digit',
                second: '2-digit'
            });
        }

        function updateStatus() {
            fetch('/api/stats')
                .then(response => response.json())
                .then(data => {
                    const statusEl = document.getElementById('status');
                    const indicatorEl = document.getElementById('status-indicator');
                    
                    if (data.running) {
                        statusEl.textContent = 'Работает';
                        indicatorEl.className = 'status-indicator running';
                    } else {
                        statusEl.textContent = 'Остановлен';
                        indicatorEl.className = 'status-indicator stopped';
                    }
                    
                    document.getElementById('uptime').textContent = formatUptime(data.uptime);
                    document.getElementById('connections-count').textContent = data.connections;
                    document.getElementById('reconnections-count').textContent = data.reconnections;
                    document.getElementById('http-requests-count').textContent = data.http_requests;
                    document.getElementById('sessions-count').textContent = data.sessions;
                })
                .catch(error => {
                    document.getElementById('status').textContent = 'Ошибка';
                    console.error('Ошибка загрузки статистики:', error);
                });
        }

        function loadConnections() {
            fetch('/api/connections')
                .then(response => response.json())
                .then(data => {
                    const tbody = document.getElementById('connections-body');
                    tbody.innerHTML = '';

                    if (data.length === 0) {
                        tbody.innerHTML = '<tr><td colspan="5" class="empty-state"><div class="empty-state-icon">🔌</div><div>Нет активных соединений</div></td></tr>';
                        return;
                    }

                    data.forEach(conn => {
                        const row = document.createElement('tr');
                        row.innerHTML = '<td>' + conn.key + '</td><td>' + conn.ip + ':' + conn.port + '</td><td><span class="badge badge-info">' + conn.type + '</span></td><td>' + (conn.connected ? '<span class="badge badge-success">Да</span>' : '<span class="badge badge-danger">Нет</span>') + '</td><td>' + formatDate(conn.last_activity) + '</td>';
                        tbody.appendChild(row);
                    });
                })
                .catch(error => {
                    console.error('Ошибка загрузки соединений:', error);
                    document.getElementById('connections-body').innerHTML = '<tr><td colspan="5" class="empty-state">Ошибка загрузки данных</td></tr>';
                });
        }

        function loadSessions() {
            fetch('/api/sessions')
                .then(response => response.json())
                .then(data => {
                    const tbody = document.getElementById('sessions-body');
                    tbody.innerHTML = '';

                    if (data.length === 0) {
                        tbody.innerHTML = '<tr><td colspan="6" class="empty-state"><div class="empty-state-icon">👤</div><div>Нет активных сессий</div></td></tr>';
                        return;
                    }

                    data.forEach(session => {
                        const row = document.createElement('tr');
                        const stageBadge = '<span class="badge badge-info">' + session.stage + '</span>';
                        row.innerHTML = '<td><code>' + session.id.substring(0, 8) + '...</code></td><td><code>' + session.key + '</code></td><td>' + (session.uid || 'N/A') + '</td><td>' + stageBadge + '</td><td>' + formatDate(session.req_time) + '</td><td><a href="/api/session/' + session.id + '" target="_blank" class="btn btn-secondary" style="padding: 5px 10px; font-size: 0.8rem;">Детали</a></td>';
                        tbody.appendChild(row);
                    });
                })
                .catch(error => {
                    console.error('Ошибка загрузки сессий:', error);
                    document.getElementById('sessions-body').innerHTML = '<tr><td colspan="6" class="empty-state">Ошибка загрузки данных</td></tr>';
                });
        }

        function loadTerminals() {
            // Show loading indicator only if table is empty
            const tbody = document.getElementById('terminals-body');
            const isLoading = tbody.innerHTML.includes('Загрузка') || tbody.children.length === 0;
            
            if (isLoading) {
                tbody.innerHTML = '<tr><td colspan="6" class="empty-state"><div class="empty-state-icon">⏳</div><div>Загрузка...</div></td></tr>';
            }
            
            fetch('/api/terminals')
                .then(response => {
                    if (!response.ok) {
                        throw new Error('HTTP error: ' + response.status);
                    }
                    return response.json();
                })
                .then(data => {
                    tbody.innerHTML = '';

                    if (!data || data.length === 0) {
                        tbody.innerHTML = '<tr><td colspan="6" class="empty-state"><div class="empty-state-icon">🖥️</div><div>Нет терминалов в списке</div></td></tr>';
                        return;
                    }

                    // Sort terminals: connected first, then by IP
                    data.sort((a, b) => {
                        if (a.connected !== b.connected) {
                            return b.connected ? 1 : -1;
                        }
                        return (a.ip || '').localeCompare(b.ip || '');
                    });

                    data.forEach(term => {
                        const row = document.createElement('tr');
                        const key = term.key || (term.id ? term.id + '_' + term.ip + '_' + term.port : term.ip + ':' + term.port);
                        const ip = term.ip || 'N/A';
                        const port = term.port || 8080;
                        const type = term.type || 'gat';
                        const connected = term.connected || false;
                        const reconnecting = term.reconnecting || false;
                        const connectionError = term.connection_error || '';
                        const lastActivity = term.last_activity ? new Date(term.last_activity).getTime() : null;
                        
                        // Status badge
                        let statusBadge = '';
                        if (connected) {
                            statusBadge = '<span class="badge badge-success">Подключен</span>';
                        } else if (reconnecting) {
                            statusBadge = '<span class="badge badge-warning">Переподключение...</span>';
                        } else if (connectionError) {
                            statusBadge = '<span class="badge badge-danger" title="' + connectionError + '">Ошибка</span>';
                        } else {
                            statusBadge = '<span class="badge badge-secondary">Не подключен</span>';
                        }
                        
                        let actions = '';
                        if (key && connected) {
                            actions = '<a href="/api/terminal/' + encodeURIComponent(key) + '" target="_blank" class="btn btn-secondary" style="padding: 5px 10px; font-size: 0.8rem; margin-right: 5px;">Детали</a>';
                            actions += '<button class="btn btn-danger" onclick="disconnectTerminal(\'' + key + '\')" style="padding: 5px 10px; font-size: 0.8rem;">Отключить</button>';
                        } else if (term.id && !reconnecting) {
                            actions = '<button class="btn btn-primary" onclick="connectTerminalByData(\'' + ip + '\', ' + port + ')" style="padding: 5px 10px; font-size: 0.8rem;">Подключить</button>';
                        }
                        
                        row.innerHTML = '<td><code>' + (term.id || key) + '</code></td>' +
                            '<td>' + ip + ':' + port + '</td>' +
                            '<td><span class="badge badge-info">' + type + '</span></td>' +
                            '<td>' + statusBadge + (connectionError && !reconnecting ? '<br><small style="color: #dc3545;">' + connectionError + '</small>' : '') + '</td>' +
                            '<td>' + (lastActivity ? formatDate(lastActivity) : 'N/A') + '</td>' +
                            '<td>' + actions + '</td>';
                        tbody.appendChild(row);
                    });
                })
                .catch(error => {
                    console.error('Ошибка загрузки терминалов:', error);
                    document.getElementById('terminals-body').innerHTML = '<tr><td colspan="6" class="empty-state">Ошибка загрузки данных</td></tr>';
                });
        }
        
        function connectTerminalByData(ip, port) {
            fetch('/api/terminals', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({ip: ip, port: port})
            })
            .then(response => response.json())
            .then(data => {
                if (data.status === 'connected') {
                    alert('Терминал успешно подключен');
                    loadTerminals();
                } else {
                    alert('Не удалось подключить терминал: ' + (data.error || 'Неизвестная ошибка'));
                }
            })
            .catch(error => {
                alert('Ошибка при подключении терминала: ' + error);
            });
        }

        function loadLogs() {
            const level = document.getElementById('log-level').value;
            const limit = document.getElementById('log-limit').value;
            let url = '/api/logs?limit=' + limit;
            if (level) url += '&level=' + level;

            fetch(url)
                .then(response => response.json())
                .then(data => {
                    const container = document.getElementById('logs-body');
                    container.innerHTML = '';

                    if (data.logs.length === 0) {
                        container.innerHTML = '<div class="empty-state"><div class="empty-state-icon">📝</div><div>Логи отсутствуют</div></div>';
                        return;
                    }

                    data.logs.forEach(log => {
                        const div = document.createElement('div');
                        div.className = 'log-entry ' + log.level.toLowerCase();
                        const logTime = log.time ? formatDate(new Date(log.time).getTime()) : 'N/A';
                        const logLevel = log.level || 'INFO';
                        const logMessage = log.message || log;
                        div.innerHTML = '<span class="log-time">[' + logTime + ']</span><span class="log-level ' + logLevel.toLowerCase() + '">[' + logLevel + ']</span>' + logMessage;
                        container.appendChild(div);
                    });
                    
                    // Auto scroll to bottom
                    container.scrollTop = container.scrollHeight;
                })
                .catch(error => {
                    console.error('Ошибка загрузки логов:', error);
                    document.getElementById('logs-body').innerHTML = '<div class="empty-state">Ошибка загрузки данных</div>';
                });
        }

        function showConnectDialog() {
            document.getElementById('connect-dialog').classList.add('active');
        }

        function hideConnectDialog() {
            document.getElementById('connect-dialog').classList.remove('active');
        }

        function connectTerminal() {
            const ip = document.getElementById('connect-ip').value;
            const port = parseInt(document.getElementById('connect-port').value);
            if (!ip || !port) {
                alert('Пожалуйста, введите IP адрес и порт');
                return;
            }

            fetch('/api/terminals', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({ip: ip, port: port})
            })
            .then(response => response.json())
            .then(data => {
                if (data.status === 'connected') {
                    alert('✅ Терминал успешно подключен');
                    hideConnectDialog();
                    loadTerminals();
                } else {
                    alert('❌ Ошибка подключения: ' + (data.error || 'Неизвестная ошибка'));
                }
            })
            .catch(error => {
                alert('❌ Ошибка подключения терминала: ' + error);
            });
        }

        function disconnectTerminal(key) {
            if (!confirm('Отключить терминал ' + key + '?')) return;

            fetch('/api/terminals?key=' + encodeURIComponent(key), {
                method: 'DELETE'
            })
            .then(response => response.json())
            .then(data => {
                if (data.status === 'disconnected') {
                    alert('✅ Терминал отключен');
                    loadTerminals();
                } else {
                    alert('❌ Ошибка отключения: ' + (data.error || 'Неизвестная ошибка'));
                }
            })
            .catch(error => {
                alert('❌ Ошибка отключения терминала: ' + error);
            });
        }

        // Close modal on outside click
        document.getElementById('connect-dialog').addEventListener('click', function(e) {
            if (e.target === this) {
                hideConnectDialog();
            }
        });

        // Initial load - load terminals first (fast), then others
        loadTerminals();
        
        // Load other data after terminals are shown
        setTimeout(() => {
            updateStatus();
            loadConnections();
            loadSessions();
            loadLogs();
        }, 100);

        // Auto refresh - terminals more frequently for status updates
        setInterval(updateStatus, 30000);
        setInterval(loadConnections, 30000);
        setInterval(loadSessions, 30000);
        setInterval(loadTerminals, 5000); // Refresh terminals every 5 seconds to show connection status updates
        setInterval(loadLogs, 60000);
    </script>
</body>
</html>`

	w.Write([]byte(html))
}

