let ws;
let myUser = null;
let queueData = [];
let isAdminMode = false;

// Подключение к сокетам
function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
    ws = new WebSocket(protocol + window.location.host + '/ws');

    ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        
        if (msg.type === 'registered') {
            myUser = msg.user;
            if (myUser.is_admin) {
                showScreen('admin-screen');
            } else {
                showScreen('user-screen');
                document.getElementById('my-ticket').textContent = myUser.ticket;
            }
        } else if (msg.type === 'update') {
            queueData = msg.queue || [];
            renderApp(msg.queue, msg.current);
        }
    };

    ws.onclose = () => setTimeout(connect, 1000);
}

// --- UI Logic ---

// Обработка нажатия ENTER
document.addEventListener('keydown', function(event) {
    if (event.key === 'Enter') {
        // Если мы на экране логина
        if (!document.getElementById('login-screen').classList.contains('hidden')) {
            joinQueue();
        }
    }
});

function toggleAdminMode() {
    isAdminMode = !isAdminMode;
    const adminField = document.getElementById('admin-field');
    const btn = document.getElementById('join-btn');
    const toggleLink = document.getElementById('toggle-auth');

    if (isAdminMode) {
        adminField.classList.remove('hidden');
        btn.textContent = "Войти в панель";
        toggleLink.textContent = "Вернуться к получению талона";
    } else {
        adminField.classList.add('hidden');
        btn.textContent = "Получить талон";
        toggleLink.textContent = "Я организатор";
    }
}

function joinQueue() {
    const name = document.getElementById('username').value;
    const pass = document.getElementById('password').value;
    
    if (!name) return alert("Введите имя!");
    if (isAdminMode && !pass) return alert("Введите пароль!");

    ws.send(JSON.stringify({
        type: 'join',
        payload: JSON.stringify({ name: name, password: pass })
    }));
}

function togglePause() {
    if (!myUser) return;
    const me = queueData.find(u => u.id === myUser.id);
    const action = (me && me.status === 'frozen') ? 'resume' : 'pause';
    
    ws.send(JSON.stringify({
        type: 'action',
        payload: JSON.stringify({ action: action, user_id: myUser.id })
    }));
}

function leaveQueue() {
    if (!confirm("Точно выйти?")) return;
    ws.send(JSON.stringify({
        type: 'action',
        payload: JSON.stringify({ action: 'leave', user_id: myUser.id })
    }));
    location.reload();
}

function callNext() {
    ws.send(JSON.stringify({
        type: 'action',
        payload: JSON.stringify({ action: 'next', user_id: '' })
    }));
}

function resetQueue() {
    if (!confirm("Сбросить всё?")) return;
    ws.send(JSON.stringify({
        type: 'action',
        payload: JSON.stringify({ action: 'reset', user_id: '' })
    }));
}

// --- Rendering ---

function renderApp(queue, current) {
    // Обновляем табло текущего
    const curTicket = document.getElementById('current-serving-ticket');
    const curName = document.getElementById('current-serving-name');
    
    if (current) {
        curTicket.textContent = current.ticket;
        curName.textContent = current.name;
        curTicket.style.color = "#219653"; 
    } else {
        curTicket.textContent = "---";
        curName.textContent = "Свободно";
        curTicket.style.color = "#333";
    }

    // Если я юзер
    if (myUser && !myUser.is_admin) {
        // Вибрация, если это Я
        if (current && current.id === myUser.id) {
            curTicket.textContent = "ВЫ!";
            curName.textContent = "Проходите к стойке";
            if(navigator.vibrate) navigator.vibrate([300, 100, 300]);
        }

        const myIdx = queue.findIndex(u => u.id === myUser.id);
        if (myIdx !== -1) {
            const me = queue[myIdx];
            document.getElementById('people-before').textContent = myIdx + " чел.";
            document.getElementById('est-time').textContent = "~" + ((myIdx + 1) * 3) + " мин";
            
            const badge = document.getElementById('status-badge');
            const btnPause = document.getElementById('btn-pause');
            
            if (me.status === 'frozen') {
                badge.textContent = "ПАУЗА";
                badge.className = "badge frozen";
                btnPause.textContent = "▶️ Вернуться";
            } else {
                badge.textContent = "В ОЧЕРЕДИ";
                badge.className = "badge waiting";
                btnPause.textContent = "⏸ Отойти";
            }
        }
    }

    // Если я админ
    if (myUser && myUser.is_admin) {
        document.getElementById('queue-count').textContent = queue.length;
        if(current) {
            document.getElementById('admin-current-ticket').textContent = current.ticket;
            document.getElementById('admin-current-name').textContent = current.name;
        } else {
            document.getElementById('admin-current-ticket').textContent = "---";
        }

        const list = document.getElementById('admin-list');
        list.innerHTML = '';
        queue.forEach((u) => {
            const li = document.createElement('li');
            li.className = 'queue-item ' + (u.status === 'frozen' ? 'item-frozen' : '');
            li.innerHTML = `
                <div class="t-info">
                    <span class="t-number">${u.ticket}</span>
                    <span class="t-name">${u.name}</span>
                </div>
                <div class="t-status">
                    ${u.status === 'frozen' ? '🧊 Пауза' : '⏳'}
                    ${u.tg_chat_id ? '📱' : ''} 
                </div>
            `;
            list.appendChild(li);
        });
    }
}

function showScreen(id) {
    document.querySelectorAll('.screen').forEach(s => s.classList.add('hidden'));
    document.getElementById(id).classList.remove('hidden');
}

connect();