let ws;
let myUser = null;
let queueData = [];
let isAdminMode = false;
let wasServed = false;

function clearInputFields() {
    if(document.getElementById('username')) document.getElementById('username').value = '';
    if(document.getElementById('password')) document.getElementById('password').value = '';
}

function manualRestoreByIP() {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'restore_by_ip', payload: '' }));
    } else {
        alert("ОШИБКА: Нет соединения с сервером (Сокет закрыт)");
    }
}

function connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss://' : 'ws://';
    // Добавляем порт, если он нестандартный, хотя window.location.host обычно его содержит
    const wsUrl = protocol + window.location.host + '/ws';
    
    console.log("Подключение к:", wsUrl);

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        console.log('✅ WebSocket подключен');
        // Попробуем восстановить сессию из LocalStorage тихо
        const savedID = localStorage.getItem('tqueue_user_id');
        if (savedID) {
            ws.send(JSON.stringify({
                type: 'restore_session',
                payload: JSON.stringify({ user_id: savedID })
            }));
        }
    };

    ws.onerror = (error) => {
        // ЕСЛИ ВЫЛЕЗЕТ ЭТОТ АЛЕРТ - ПРОБЛЕМА СЕТЕВАЯ (БЕЛЫЙ IP БЛОЧИТСЯ)
        alert("❌ ОШИБКА СОКЕТА! Связь заблокирована.");
        console.error('WS Error:', error);
    };

    ws.onmessage = (event) => {
        const msg = JSON.parse(event.data);
        
        if (msg.type === 'error') {
            alert(msg.payload);
            if (msg.payload.includes("Ваш талон не найден")) fullReset();
        }
        else if (msg.type === 'registered' || msg.type === 'session_restored') {
            myUser = msg.user;
            localStorage.setItem('tqueue_user_id', myUser.id);
            clearInputFields();
            
            if (myUser.is_admin) {
                showScreen('admin-screen');
            } else {
                showScreen('user-screen');
                document.getElementById('my-ticket').textContent = myUser.ticket;
            }
            if (msg.queue) renderApp(msg.queue, msg.current);
        } 
        else if (msg.type === 'update') {
            queueData = msg.queue || [];
            renderApp(msg.queue, msg.current);
        } 
        else if (msg.type === 'session_expired') {
            localStorage.removeItem('tqueue_user_id');
            showScreen('login-screen');
        } 
        else if (msg.type === 'show_screen') {
            showScreen(msg.screen === 'admin' ? 'admin-screen' : 'user-screen');
        }
    };

    ws.onclose = () => setTimeout(connect, 1000);
}

function fullReset() {
    myUser = null;
    wasServed = false;
    isAdminMode = false;
    localStorage.removeItem('tqueue_user_id');
    clearInputFields();
    
    // Скрываем всё
    document.querySelectorAll('.screen').forEach(s => s.classList.add('hidden'));
    
    // Показываем экран входа УЧАСТНИКА
    document.getElementById('user-login-screen').classList.remove('hidden');
}

function showScreen(id) {
    document.querySelectorAll('.screen').forEach(s => s.classList.add('hidden'));
    document.getElementById(id).classList.remove('hidden');
}

// UI HANDLERS
function joinQueue(type) {
    let name, pass;

    if (type === 'admin') {
        // Берем данные из формы админа
        name = document.getElementById('admin-username').value;
        pass = document.getElementById('admin-password').value;
        if (!name) return alert("Введите имя (например: Стол 1)!");
        if (!pass) return alert("Введите пароль!");
    } else {
        // Берем данные из формы юзера
        name = document.getElementById('username').value;
        pass = ""; // У юзера нет пароля
        if (!name) return alert("Введите ваше имя!");
    }

    // Проверка соединения
    if (!ws || ws.readyState !== WebSocket.OPEN) return alert("Нет соединения с сервером!");

    ws.send(JSON.stringify({
        type: 'join',
        payload: JSON.stringify({ name: name, password: pass })
    }));
}

function clearInputFields() {
    if(document.getElementById('username')) document.getElementById('username').value = '';
    if(document.getElementById('admin-username')) document.getElementById('admin-username').value = '';
    if(document.getElementById('admin-password')) document.getElementById('admin-password').value = '';
}

function switchToAdmin() {
    isAdminMode = true;
    // Скрываем экран юзера
    document.getElementById('user-login-screen').classList.add('hidden');
    // Показываем экран админа
    document.getElementById('admin-login-screen').classList.remove('hidden');
    
    // Очищаем поля админа при входе
    document.getElementById('admin-password').value = '';
}

function switchToUser() {
    isAdminMode = false;
    // Скрываем экран админа
    document.getElementById('admin-login-screen').classList.add('hidden');
    // Показываем экран юзера
    document.getElementById('user-login-screen').classList.remove('hidden');
}

function togglePause() {
    if (!myUser) return;
    const me = queueData.find(u => u.id === myUser.id);
    const action = (me && me.status === 'frozen') ? 'resume' : 'pause';
    ws.send(JSON.stringify({ type: 'action', payload: JSON.stringify({ action: action, user_id: myUser.id }) }));
}

function leaveQueue() {
    if (!confirm("Точно отказаться от талона? Вернуть его будет нельзя.")) return;
    
    // 1. Отправляем сигнал на сервер (чтобы пометить в БД как left)
    if (myUser) {
        ws.send(JSON.stringify({
            type: 'action',
            payload: JSON.stringify({ action: 'leave', user_id: myUser.id })
        }));
    }

    // 2. Немедленно убиваем сессию в браузере
    localStorage.removeItem('tqueue_user_id');
    
    // 3. Сбрасываем интерфейс
    fullReset();
}

function callNext() {
    ws.send(JSON.stringify({ type: 'action', payload: JSON.stringify({ action: 'next', user_id: '' }) }));
}

function resetQueue() {
    if (!confirm("Сбросить всё?")) return;
    ws.send(JSON.stringify({ type: 'action', payload: JSON.stringify({ action: 'reset', user_id: '' }) }));
}

function renderApp(queue, current) {
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

    // Если я ЮЗЕР
    if (myUser && !myUser.is_admin) {
        const amICurrent = current && current.id === myUser.id;
        const amInQueue = queue.find(u => u.id === myUser.id);

        if (wasServed && !amICurrent && !amInQueue) {
            alert("✅ Обслуживание завершено! Спасибо.");
            fullReset();
            return;
        }

        if (amICurrent) {
            wasServed = true;
            curTicket.textContent = "ВЫ!";
            curName.textContent = "Проходите к стойке";
            document.getElementById('people-before').textContent = "0 чел.";
            document.getElementById('est-time').textContent = "0 мин";
            document.getElementById('status-badge').textContent = "ВАС ВЫЗВАЛИ";
            if(navigator.vibrate) navigator.vibrate([300, 100, 300]);
        } 
        else if (amInQueue) {
            wasServed = false;
            const myIdx = queue.findIndex(u => u.id === myUser.id);
            const me = queue[myIdx];

            document.getElementById('people-before').textContent = myIdx + " чел.";
            
            // --- ИЗМЕНЕНИЕ ЗДЕСЬ: Умножаем на 10 минут ---
            document.getElementById('est-time').textContent = "~" + ((myIdx + 1) * 10) + " мин";
            // ---------------------------------------------

            const badge = document.getElementById('status-badge');
            const btnPause = document.getElementById('btn-pause');
            
            if (me.status === 'frozen') {
                badge.textContent = "ПАУЗА"; badge.className = "badge frozen"; btnPause.textContent = "▶️ Вернуться";
            } else {
                badge.textContent = "В ОЧЕРЕДИ"; badge.className = "badge waiting"; btnPause.textContent = "⏸ Отойти";
            }
        }
    }

    // Если я АДМИН (код без изменений)
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
                <div class="t-info"><span class="t-number">${u.ticket}</span><span class="t-name">${u.name}</span></div>
                <div class="t-status">${u.status === 'frozen' ? '🧊' : '⏳'}${u.tg_chat_id ? '📱' : ''}</div>`;
            list.appendChild(li);
        });
    }
}

// START
document.addEventListener('DOMContentLoaded', function() {
    clearInputFields();
    isAdminMode = false;
    connect();
});