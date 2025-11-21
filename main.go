package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url" // Нужно для отправки сообщений в телеграм
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// --- КОНФИГ ---
const telegramToken = "8293823191:AAGqs7cDTFQfuvWoo6ulPTKoe1lsElgNSq0" // <--- ПРОВЕРЬ ТОКЕН!
const adminPassword = "admin"

// --- Структуры ---

type User struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Ticket   string `json:"ticket"`
	Status   string `json:"status"` // "waiting", "frozen", "served"
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

// Структура для запроса от Python-бота
type LinkRequest struct {
	Ticket string `json:"ticket"`
	ChatID int64  `json:"chat_id"`
}

// --- Глобальное состояние ---

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
	// Раздача статики
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// WebSocket
	http.HandleFunc("/ws", handleConnections)

	// API для Python бота
	http.HandleFunc("/api/link_telegram", handleLinkTelegram)

	// Фоновая рассылка сообщений сокетов
	go handleMessages()

	fmt.Println("🚀 Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// --- Обработка запроса от Python ---
func handleLinkTelegram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LinkRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Bad JSON", http.StatusBadRequest)
		return
	}

	queueMutex.Lock()
	defer queueMutex.Unlock()

	found := false
	for _, u := range queue {
		if u.Ticket == req.Ticket {
			u.TgChatID = req.ChatID
			found = true
			fmt.Printf("🔗 Linked ticket %s to ChatID %d\n", u.Ticket, u.TgChatID)
			break
		}
	}

	if found {
		broadcastQueueState() // Обновляем админку (появится значок телефона)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		http.Error(w, "Ticket not found", http.StatusNotFound)
	}
}

// --- WebSocket Logic ---

func handleConnections(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close()

	clients[ws] = true
	sendQueueState(ws)

	for {
		var msg Message
		err := ws.ReadJSON(&msg)
		if err != nil {
			delete(clients, ws)
			break
		}

		if msg.Type == "join" {
			handleJoin(msg.Payload, ws)
		} else if msg.Type == "action" {
			handleAction(msg.Payload)
		}
	}
}

func handleJoin(payloadStr string, ws *websocket.Conn) {
	var payload JoinPayload
	json.Unmarshal([]byte(payloadStr), &payload)

	queueMutex.Lock()
	defer queueMutex.Unlock()

	newUser := &User{
		ID:       fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:     payload.Name,
		JoinedAt: time.Now().Unix(),
		Status:   "waiting",
	}

	if payload.Password == adminPassword {
		newUser.IsAdmin = true
		newUser.Name = "Администратор"
		newUser.Ticket = "ADMIN"
	} else {
		ticketCounter++
		newUser.Ticket = fmt.Sprintf("A-%d", ticketCounter)
		newUser.IsAdmin = false
		queue = append(queue, newUser)
	}

	ws.WriteJSON(map[string]interface{}{
		"type": "registered",
		"user": newUser,
	})

	broadcastQueueState()
}

func handleAction(payloadStr string) {
	var payload ActionPayload
	json.Unmarshal([]byte(payloadStr), &payload)

	queueMutex.Lock()
	defer queueMutex.Unlock()

	switch payload.Action {
	case "pause":
		for _, u := range queue {
			if u.ID == payload.UserID {
				u.Status = "frozen"
			}
		}
	case "resume":
		for _, u := range queue {
			if u.ID == payload.UserID {
				u.Status = "waiting"
			}
		}
	case "leave":
		newQueue := []*User{}
		for _, u := range queue {
			if u.ID != payload.UserID {
				newQueue = append(newQueue, u)
			}
		}
		queue = newQueue
	case "next":
		if len(queue) > 0 {
			foundIdx := -1
			for i, u := range queue {
				if u.Status == "waiting" {
					foundIdx = i
					break
				}
			}

			if foundIdx != -1 {
				currentServing = queue[foundIdx]
				queue = append(queue[:foundIdx], queue[foundIdx+1:]...)

				// Отправляем уведомление в Telegram
				if currentServing.TgChatID != 0 {
					go sendTgMessage(currentServing.TgChatID, "🔥 ВАША ОЧЕРЕДЬ! ПОДХОДИТЕ К СТОЙКЕ!")
				}
			}
		}
	case "reset":
		queue = []*User{}
		currentServing = nil
		ticketCounter = 100
	}
	broadcastQueueState()
}

func sendTgMessage(chatID int64, text string) {
	// Go отправляет сообщение сам, когда админ жмет кнопку
	msg := url.QueryEscape(text)
	reqUrl := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage?chat_id=%d&text=%s", telegramToken, chatID, msg)
	http.Get(reqUrl)
}

func handleMessages() {
	for msg := range broadcast {
		for client := range clients {
			client.WriteMessage(websocket.TextMessage, msg)
		}
	}
}

func broadcastQueueState() {
	state := map[string]interface{}{
		"type":    "update",
		"queue":   queue,
		"current": currentServing,
	}
	jsonMsg, _ := json.Marshal(state)
	broadcast <- jsonMsg
}

func sendQueueState(ws *websocket.Conn) {
	state := map[string]interface{}{"type": "update", "queue": queue, "current": currentServing}
	ws.WriteJSON(state)
}