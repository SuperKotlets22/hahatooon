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
    const wsUrl = protocol + window.location.host + '/ws';
    
    console.log("Подключение к:", wsUrl);

    ws = new WebSocket(wsUrl);

    ws.onopen = () => {
        console.log('✅ WebSocket подключен');
        const savedID = localStorage.getItem('tqueue_user_id');
        if (savedID) {
            ws.send(JSON.stringify({
                type: 'restore_session',
                payload: JSON.stringify({ user_id: savedID })
            }));
        }
    };

    ws.onerror = (error) => {
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
                setTimeout(initGame, 100);
            } else {
                showScreen('user-screen');
                document.getElementById('my-ticket').textContent = myUser.ticket;
                setTimeout(initGame, 100);
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
    console.log("♻️ Выполняем полный сброс...");
    
    // 1. Сбрасываем переменные
    myUser = null;
    wasServed = false;
    isAdminMode = false;
    
    // 2. Чистим память
    localStorage.removeItem('tqueue_user_id');
    clearInputFields();
    
    // 3. Скрываем ВСЕ экраны
    document.querySelectorAll('.screen').forEach(s => s.classList.add('hidden'));
    
    // 4. Явно показываем начальный экран УЧАСТНИКА
    const startScreen = document.getElementById('user-login-screen');
    if (startScreen) {
        startScreen.classList.remove('hidden');
    } else {
        console.error("❌ Ошибка: Не найден экран user-login-screen");
    }
}

function showScreen(id) {
    document.querySelectorAll('.screen').forEach(s => s.classList.add('hidden'));
    document.getElementById(id).classList.remove('hidden');
}

// UI HANDLERS
function joinQueue(type) {
    let name, pass;

    if (type === 'admin') {
        name = document.getElementById('admin-username').value;
        pass = document.getElementById('admin-password').value;
        if (!name) return alert("Введите имя (например: Стол 1)!");
        if (!pass) return alert("Введите пароль!");
    } else {
        name = document.getElementById('username').value;
        pass = ""; 
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
    
    if (myUser) {
        ws.send(JSON.stringify({
            type: 'action',
            payload: JSON.stringify({ action: 'leave', user_id: myUser.id })
        }));
    }

    localStorage.removeItem('tqueue_user_id');
    
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
            
            document.getElementById('est-time').textContent = "~" + ((myIdx + 1) * 10) + " мин";

            const badge = document.getElementById('status-badge');
            const btnPause = document.getElementById('btn-pause');
            
            if (me.status === 'frozen') {
                badge.textContent = "ПАУЗА"; badge.className = "badge frozen"; btnPause.textContent = "▶️ Вернуться";
            } else {
                badge.textContent = "В ОЧЕРЕДИ"; badge.className = "badge waiting"; btnPause.textContent = "⏸ Отойти";
            }
        }
    }

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
//ЛОГИКА ИГРЫ 2048
const boardSize = 4;
let board = [];
let score = 0;

// Инициализация игры 
function initGame() {
    board = Array(boardSize * boardSize).fill(0);
    score = 0;
    updateScore();
    addRandomTile();
    addRandomTile();
    renderBoard();
    setupInput();
}

function restartGame() {
    initGame();
}

function renderBoard() {
    const container = document.getElementById('game-board');
    container.innerHTML = '';
    board.forEach(val => {
        const tile = document.createElement('div');
        tile.className = 'tile';
        if (val > 0) {
            tile.textContent = val;
            tile.setAttribute('data-val', val);
        }
        container.appendChild(tile);
    });
}

function addRandomTile() {
    const emptyIndices = board.map((val, idx) => val === 0 ? idx : null).filter(val => val !== null);
    if (emptyIndices.length === 0) return;
    const randomIdx = emptyIndices[Math.floor(Math.random() * emptyIndices.length)];
    board[randomIdx] = Math.random() < 0.9 ? 2 : 4;
}

function updateScore() {
    const sEl = document.getElementById('score');
    if(sEl) sEl.textContent = score;
}

// Логика движения
function slide(row) {
    let arr = row.filter(val => val);
    let missing = boardSize - arr.length;
    let zeros = Array(missing).fill(0);
    return arr.concat(zeros);
}

function combine(row) {
    for (let i = 0; i < boardSize - 1; i++) {
        if (row[i] !== 0 && row[i] === row[i + 1]) {
            row[i] *= 2;
            row[i + 1] = 0;
            score += row[i];
        }
    }
    return row;
}

function move(direction) {
    let oldBoard = [...board];
    let moved = false;

    if (direction === 'Left' || direction === 'Right') {
        for (let i = 0; i < boardSize * boardSize; i += boardSize) {
            let row = board.slice(i, i + boardSize);
            if (direction === 'Right') row.reverse();
            row = slide(combine(slide(row)));
            if (direction === 'Right') row.reverse();
            for (let j = 0; j < boardSize; j++) board[i + j] = row[j];
        }
    } else { // Up or Down
        for (let i = 0; i < boardSize; i++) {
            let col = [board[i], board[i + boardSize], board[i + boardSize * 2], board[i + boardSize * 3]];
            if (direction === 'Down') col.reverse();
            col = slide(combine(slide(col)));
            if (direction === 'Down') col.reverse();
            board[i] = col[0]; board[i + boardSize] = col[1];
            board[i + boardSize * 2] = col[2]; board[i + boardSize * 3] = col[3];
        }
    }

    if (JSON.stringify(oldBoard) !== JSON.stringify(board)) {
        addRandomTile();
        renderBoard();
        updateScore();
    }
}

function setupInput() {
    // Клавиатура
    document.onkeydown = (e) => {
        if (e.key === 'ArrowLeft') move('Left');
        else if (e.key === 'ArrowRight') move('Right');
        else if (e.key === 'ArrowUp') move('Up');
        else if (e.key === 'ArrowDown') move('Down');
    };

    // Свайпы для телефонов
    let touchStartX = 0;
    let touchStartY = 0;
    const gameBoard = document.getElementById('game-board');

    gameBoard.addEventListener('touchstart', (e) => {
        touchStartX = e.changedTouches[0].screenX;
        touchStartY = e.changedTouches[0].screenY;
    }, {passive: false});

    gameBoard.addEventListener('touchend', (e) => {
        let touchEndX = e.changedTouches[0].screenX;
        let touchEndY = e.changedTouches[0].screenY;
        handleSwipe(touchStartX, touchStartY, touchEndX, touchEndY);
    }, {passive: false});
}

function handleSwipe(startX, startY, endX, endY) {
    let dx = endX - startX;
    let dy = endY - startY;
    
    if (Math.abs(dx) > Math.abs(dy)) {
        if (Math.abs(dx) > 30) move(dx > 0 ? 'Right' : 'Left');
    } else {
        if (Math.abs(dy) > 30) move(dy > 0 ? 'Down' : 'Up');
    }
}