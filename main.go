package main

import (
	"net"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const telegramToken = "8293823191:AAGqs7cDTFQfuvWoo6ulPTKoe1lsElgNSq0"
const adminPassword = "admin"

type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Ticket    string `json:"ticket"`
	Status    string `json:"status"`
	JoinedAt  int64  `json:"joined_at"` 
	IsAdmin   bool   `json:"is_admin"`
	TgChatID  int64  `json:"tg_chat_id"`
	IPAddress string `json:"ip_address"`
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

type LinkRequest struct {
	Ticket string `json:"ticket"`
	ChatID int64  `json:"chat_id"`
}

type BotActionRequest struct {
	ChatID int64  `json:"chat_id"`
	Action string `json:"action"`
}

var (
	clients   = make(map[*websocket.Conn]bool)
	broadcast = make(chan []byte)
	upgrader  = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	queue     []*User
	queueMutex sync.Mutex
	currentServing *User
)

func main() {
	initDB()

	queueMutex.Lock()
	queue = loadQueueFromDB()
	currentServing = loadCurrentServing()
	queueMutex.Unlock()

	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)
	http.HandleFunc("/ws", handleConnections)
	http.HandleFunc("/api/link_telegram", handleLinkTelegram)
	http.HandleFunc("/api/bot_action", handleBotAction)

	go handleMessages()

	fmt.Println("🚀 Server started on :8080 (with Database)")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleBotAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		return
	}

	var req BotActionRequest
	json.NewDecoder(r.Body).Decode(&req)

	queueMutex.Lock()
	defer queueMutex.Unlock()

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

	if req.Action == "pause" {
		if targetUser.Status == "frozen" {
			targetUser.Status = "waiting"
			updateUserStatus(targetUser.ID, "waiting")
		} else {
			targetUser.Status = "frozen"
			updateUserStatus(targetUser.ID, "frozen")
		}
	} else if req.Action == "leave" {
		newQueue := []*User{}
		for _, u := range queue {
			if u.ID != targetUser.ID {
				newQueue = append(newQueue, u)
			}
		}
		queue = newQueue
		deleteUser(targetUser.ID)
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
			updateUserTelegram(u.ID, req.ChatID)
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

func handleRestoreByIP(ip string, ws *websocket.Conn) {
	queueMutex.Lock()
	defer queueMutex.Unlock()

	user, err := findActiveUserByIP(ip)

	if err == nil && user != nil {
		if user.Status == "served" {
			ws.WriteJSON(map[string]interface{}{
				"type":    "error",
				"payload": "Вы уже прошли обслуживание. Спасибо!",
			})
		} else {
			ws.WriteJSON(map[string]interface{}{
				"type": "registered",
				"user": user,
			})
			sendQueueState(ws)
		}
	} else {
		ws.WriteJSON(map[string]interface{}{
			"type":    "error",
			"payload": "Талон не найден для этого устройства.",
		})
	}
}

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
		if err := ws.ReadJSON(&msg); err != nil {
			delete(clients, ws)
			break
		}

		if msg.Type == "restore_by_ip" {
    		host, _, err := net.SplitHostPort(ws.RemoteAddr().String())
    		if err != nil {
        		host = ws.RemoteAddr().String()
    		}
    		handleRestoreByIP(host, ws) 
		}
		if msg.Type == "join" {
			handleJoin(msg.Payload, ws)
		}
		if msg.Type == "action" {
			handleAction(msg.Payload)
		}
		if msg.Type == "restore_session" {
			handleWSRestoreSession(msg.Payload, ws)
		}
	}
}

func handleWSRestoreSession(payloadStr string, ws *websocket.Conn) {
	var payload struct {
		UserID string `json:"user_id"`
	}
	json.Unmarshal([]byte(payloadStr), &payload)

	queueMutex.Lock()
	defer queueMutex.Unlock()

	var user *User
	for _, u := range queue {
		if u.ID == payload.UserID {
			user = u
			break
		}
	}

	if user != nil {
		ws.WriteJSON(map[string]interface{}{
			"type":    "session_restored",
			"user":    user,
			"queue":   queue,
			"current": currentServing,
		})

		if user.IsAdmin {
			ws.WriteJSON(map[string]interface{}{
				"type":   "show_screen",
				"screen": "admin",
			})
		} else {
			ws.WriteJSON(map[string]interface{}{
				"type":   "show_screen",
				"screen": "user",
			})
		}
	} else {
		ws.WriteJSON(map[string]interface{}{
			"type": "session_expired",
		})
	}
}

func handleJoin(payloadStr string, ws *websocket.Conn) {
	var payload JoinPayload
	json.Unmarshal([]byte(payloadStr), &payload)

	host, _, err := net.SplitHostPort(ws.RemoteAddr().String())
	if err != nil {

    	host = ws.RemoteAddr().String()
	}
	ip := host

	queueMutex.Lock()
	defer queueMutex.Unlock()

	if payload.Password != adminPassword {
		existingUser, err := findActiveUserByIP(ip)
		if err == nil && existingUser != nil && existingUser.Status != "served" {
			ws.WriteJSON(map[string]interface{}{
				"type":    "error",
				"payload": fmt.Sprintf("У вас уже есть активный талон: %s", existingUser.Ticket),
			})
			ws.WriteJSON(map[string]interface{}{"type": "registered", "user": existingUser})
			return
		}
	}

	newUser := &User{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Name:      payload.Name,
		JoinedAt:  time.Now().Unix(), 
		Status:    "waiting",
		IPAddress: ip,
	}

	if payload.Password == adminPassword {
		newUser.IsAdmin = true
		newUser.Name = "Администратор"
		newUser.Ticket = "ADMIN"
	} else {
		newTicketNum := getNextTicketNumber()
		newUser.Ticket = fmt.Sprintf("A-%d", newTicketNum)
		newUser.IsAdmin = false
		queue = append(queue, newUser)
	}

	saveUser(newUser)

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
		for _, u := range queue {
			if u.ID == payload.UserID {
				if u.Status == "frozen" {
					u.Status = "waiting"
				} else {
					u.Status = "frozen"
				}
				updateUserStatus(u.ID, u.Status)
			}
		}
	case "resume":
		for _, u := range queue {
			if u.ID == payload.UserID {
				u.Status = "waiting"
				updateUserStatus(u.ID, "waiting")
			}
		}
	case "leave":
		newQueue := []*User{}
		for _, u := range queue {
			if u.ID != payload.UserID {
				newQueue = append(newQueue, u)
			} else {
				updateUserStatus(u.ID, "left")
			}
		}
		queue = newQueue

	case "next":
		if currentServing != nil {
			updateUserStatus(currentServing.ID, "served")
		}

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

				saveCurrentServing(currentServing)

				if currentServing.TgChatID != 0 {
					go sendTgMessage(currentServing.TgChatID, "🔥 ВАША ОЧЕРЕДЬ! ПОДХОДИТЕ К СТОЙКЕ!")
				}
				notifyNextInLine()
			} else {
				currentServing = nil
				saveCurrentServing(nil)
			}
		} else {
			currentServing = nil
			saveCurrentServing(nil)
		}

	case "reset":
		queue = []*User{}
		currentServing = nil
		db.Exec("DELETE FROM users")
		db.Exec("UPDATE queue_state SET current_user_id = NULL, ticket_counter = 100 WHERE key = 'main'")
	}

	broadcastQueueState()
}

func notifyNextInLine() {
	for _, u := range queue {
		if u.Status == "waiting" {
			if u.TgChatID != 0 {
				go sendTgMessage(u.TgChatID, fmt.Sprintf("⚠️ Приготовьтесь! Вы следующий. Ваш талон: %s", u.Ticket))
			}
			break
		}
	}
}

func sendTgMessage(chatID int64, text string) {
	msg := url.QueryEscape(text)
	http.Get(fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage?chat_id=%d&text=%s", telegramToken, chatID, msg))
}

func handleMessages() {
	for msg := range broadcast {
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				client.Close()
				delete(clients, client)
			}
		}
	}
}

func broadcastQueueState() {
	jsonMsg, _ := json.Marshal(map[string]interface{}{
		"type":    "update",
		"queue":   queue,
		"current": currentServing,
	})
	broadcast <- jsonMsg
}

func sendQueueState(ws *websocket.Conn) {
	ws.WriteJSON(map[string]interface{}{
		"type":    "update",
		"queue":   queue,
		"current": currentServing,
	})
}

func getClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}
