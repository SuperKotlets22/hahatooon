package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const telegramToken = "8293823191:AAGqs7cDTFQfuvWoo6ulPTKoe1lsElgNSq0" // <--- ПРОВЕРЬ ТОКЕН!!!
const adminPassword = "admin"

// --- Структуры ---
type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Ticket   string `json:"ticket"`
	Status   string `json:"status"`
	JoinedAt int64  `json:"joined_at"`
	IsAdmin  bool   `json:"is_admin"`
	TgChatID int64  `json:"tg_chat_id"`
}

type Message struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type JoinPayload struct {
	Name     string `json:"name"`
	Password string `json:"password,omitempty"`
}

type ActionPayload struct {
	Action string `json:"action"`
	UserID string `json:"user_id"`
}

// Для запроса от Python (привязка)
type LinkRequest struct {
	Ticket string `json:"ticket"`
	ChatID int64  `json:"chat_id"`
}

// Для запроса от Python (кнопки)
type BotActionRequest struct {
	ChatID int64  `json:"chat_id"`
	Action string `json:"action"` // "pause", "leave"
}

var (
	clients   = make(map[*websocket.Conn]bool)
	broadcast = make(chan []byte)
	upgrader  = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	queue      []*User
	queueMutex sync.Mutex
	currentServing *User
	ticketCounter  = 100
)

func main() {
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", handleConnections)
	http.HandleFunc("/api/link_telegram", handleLinkTelegram)
	http.HandleFunc("/api/bot_action", handleBotAction) // <-- НОВЫЙ РОУТ

	go handleMessages()

	fmt.Println("🚀 Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// --- Обработка кнопок от бота ---
func handleBotAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { return }
	var req BotActionRequest
	json.NewDecoder(r.Body).Decode(&req)

	queueMutex.Lock()
	defer queueMutex.Unlock()

	// Находим юзера по ChatID
	var targetUser *User
	for _, u := range queue {
		if u.TgChatID == req.ChatID {
			targetUser = u
			break
		}
	}

	if targetUser == nil {
		http.Error(w, "User not found", 404)
		return
	}

	// Выполняем действие
	if req.Action == "pause" {
		if targetUser.Status == "frozen" {
			targetUser.Status = "waiting" // Toggle (если был заморожен -> разморозить)
		} else {
			targetUser.Status = "frozen"
		}
	} else if req.Action == "leave" {
		newQueue := []*User{}
		for _, u := range queue {
			if u.ID != targetUser.ID { newQueue = append(newQueue, u) }
		}
		queue = newQueue
	}

	broadcastQueueState()
	w.Write([]byte("OK"))
}

func handleLinkTelegram(w http.ResponseWriter, r *http.Request) {
	var req LinkRequest
	json.NewDecoder(r.Body).Decode(&req)
	queueMutex.Lock()
	defer queueMutex.Unlock()
	found := false
	for _, u := range queue {
		if u.Ticket == req.Ticket {
			u.TgChatID = req.ChatID
			found = true
			break
		}
	}
	if found {
		broadcastQueueState()
		w.Write([]byte("OK"))
	} else {
		http.Error(w, "Not found", 404)
	}
}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	defer ws.Close()

	clients[ws] = true
	// Сразу состояние не шлем, ждем или join или reconnect от клиента

	for {
		var msg Message
		err := ws.ReadJSON(&msg)
		if err != nil {
			delete(clients, ws)
			break
		}

		if msg.Type == "join" {
			handleJoin(msg.Payload, ws)
		} else if msg.Type == "reconnect" {
			// НОВОЕ: Обработка переподключения
			handleReconnect(msg.Payload, ws)
		} else if msg.Type == "action" {
			handleAction(msg.Payload)
		}
	}
}

// НОВАЯ ФУНКЦИЯ
func handleReconnect(userID string, ws *websocket.Conn) {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	var foundUser *User

	// 1. Ищем в очереди ожидания
	for _, u := range queue {
		if u.ID == userID {
			foundUser = u
			break
		}
	}

	// 2. Ищем в текущем обслуживаемом (если он есть)
	if foundUser == nil && currentServing != nil && currentServing.ID == userID {
		foundUser = currentServing
	}

	// 3. Ищем админа (у админа ID фиксированный или мы его не храним в очереди, 
    // но для упрощения, если юзер знает пароль, он просто логинится заново. 
    // Здесь мы восстанавливаем только обычных юзеров, либо если админ был в списке)
	
	if foundUser != nil {
		// Пользователь найден - восстанавливаем сессию
		ws.WriteJSON(map[string]interface{}{
			"type": "registered",
			"user": foundUser,
		})
		// И сразу шлем актуальное состояние очереди
		sendQueueState(ws)
	} else {
		// Пользователь не найден (например, сервер перезагрузили)
		// Шлем команду на очистку LocalStorage
		ws.WriteJSON(map[string]interface{}{
			"type": "error",
			"payload": "session_expired",
		})
	}
}

func handleJoin(payloadStr string, ws *websocket.Conn) {
	var payload JoinPayload
	json.Unmarshal([]byte(payloadStr), &payload)
	queueMutex.Lock()
	defer queueMutex.Unlock()
	newUser := &User{
		ID: fmt.Sprintf("%d", time.Now().UnixNano()), Name: payload.Name, JoinedAt: time.Now().Unix(), Status: "waiting",
	}
	if payload.Password == adminPassword {
		newUser.IsAdmin = true; newUser.Name = "Admin"; newUser.Ticket = "ADMIN"
	} else {
		ticketCounter++; newUser.Ticket = fmt.Sprintf("A-%d", ticketCounter); newUser.IsAdmin = false; queue = append(queue, newUser)
	}
	ws.WriteJSON(map[string]interface{}{"type": "registered", "user": newUser})
	broadcastQueueState()
}

func handleAction(payloadStr string) {
	var payload ActionPayload
	json.Unmarshal([]byte(payloadStr), &payload)
	queueMutex.Lock()
	defer queueMutex.Unlock()

	switch payload.Action {
	case "pause":
		for _, u := range queue { if u.ID == payload.UserID { u.Status = "frozen" } }
	case "resume":
		for _, u := range queue { if u.ID == payload.UserID { u.Status = "waiting" } }
	case "leave":
		newQueue := []*User{}
		for _, u := range queue { if u.ID != payload.UserID { newQueue = append(newQueue, u) } }
		queue = newQueue
	case "next":
		if len(queue) > 0 {
			// Ищем первого активного
			foundIdx := -1
			for i, u := range queue {
				if u.Status == "waiting" { foundIdx = i; break }
			}
			if foundIdx != -1 {
				currentServing = queue[foundIdx]
				queue = append(queue[:foundIdx], queue[foundIdx+1:]...)
				
				// 1. Уведомляем того, кого вызвали
				if currentServing.TgChatID != 0 {
					go sendTgMessage(currentServing.TgChatID, "🔥 ВАША ОЧЕРЕДЬ! ПОДХОДИТЕ К СТОЙКЕ!")
				}

				// 2. Уведомляем следующего (кто теперь стал первым), чтобы готовился
				notifyNextInLine()
			}
		}
	case "reset":
		queue = []*User{}; currentServing = nil; ticketCounter = 100
	}
	broadcastQueueState()
}

func notifyNextInLine() {
	// Ищем кто теперь стоит первым в очереди
	for _, u := range queue {
		if u.Status == "waiting" {
			if u.TgChatID != 0 {
				go sendTgMessage(u.TgChatID, fmt.Sprintf("⚠️ Приготовьтесь! Вы следующий. Ваш талон: %s", u.Ticket))
			}
			break // Нашли первого - выходим
		}
	}
}

func sendTgMessage(chatID int64, text string) {
	msg := url.QueryEscape(text)
	http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage?chat_id=%d&text=%s", telegramToken, chatID, msg))
}

func handleMessages() {
	for msg := range broadcast { for client := range clients { client.WriteMessage(websocket.TextMessage, msg) } }
}
func broadcastQueueState() {
	jsonMsg, _ := json.Marshal(map[string]interface{}{"type": "update", "queue": queue, "current": currentServing})
	broadcast <- jsonMsg
}
func sendQueueState(ws *websocket.Conn) {
	ws.WriteJSON(map[string]interface{}{"type": "update", "queue": queue, "current": currentServing})
}