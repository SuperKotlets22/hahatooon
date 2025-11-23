package main

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite3", "./tqueue.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			ticket TEXT UNIQUE,
			status TEXT DEFAULT 'waiting',
			is_admin BOOLEAN DEFAULT FALSE,
			tg_chat_id INTEGER DEFAULT 0,
			joined_at DATETIME,
			ip_address TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_ip ON users(ip_address)`,

		`CREATE TABLE IF NOT EXISTS queue_state (
			key TEXT PRIMARY KEY,
			current_user_id TEXT,
			ticket_counter INTEGER DEFAULT 100,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS sessions (
			user_id TEXT PRIMARY KEY,
			last_seen DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, query := range queries {
		_, err = db.Exec(query)
		if err != nil {
			log.Fatal("Failed to create table:", err)
		}
	}

	db.Exec("INSERT OR IGNORE INTO queue_state (key, ticket_counter) VALUES ('main', 100)")
	log.Println("✅ Database initialized successfully")
}

func saveUser(user *User) error {
	joinedAtStr := time.Unix(user.JoinedAt, 0).Format(time.RFC3339)
	_, err := db.Exec(
		"INSERT OR REPLACE INTO users (id, name, ticket, status, is_admin, tg_chat_id, joined_at, ip_address) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		user.ID, user.Name, user.Ticket, user.Status, user.IsAdmin, user.TgChatID, joinedAtStr, user.IPAddress,
	)
	return err
}

func deleteUser(userID string) error {
	_, err := db.Exec("DELETE FROM users WHERE id = ?", userID)
	return err
}

func findActiveUserByIP(ip string) (*User, error) {
	var u User
	var joinedAt string

	err := db.QueryRow(`
		SELECT id, name, ticket, status, is_admin, tg_chat_id, joined_at, ip_address 
		FROM users 
		WHERE ip_address = ? AND status IN ('waiting', 'frozen', 'served')
		ORDER BY created_at DESC LIMIT 1
	`, ip).Scan(&u.ID, &u.Name, &u.Ticket, &u.Status, &u.IsAdmin, &u.TgChatID, &joinedAt, &u.IPAddress)

	if err != nil {
		return nil, err
	}

	t, _ := time.Parse(time.RFC3339, joinedAt)
	u.JoinedAt = t.Unix() 
	return &u, nil
}

func getNextTicketNumber() int {
	var counter int
	db.QueryRow("SELECT ticket_counter FROM queue_state WHERE key = 'main'").Scan(&counter)
	counter++
	db.Exec("UPDATE queue_state SET ticket_counter = ? WHERE key = 'main'", counter)
	return counter
}

func loadQueueFromDB() []*User {
	rows, err := db.Query(`
		SELECT id, name, ticket, status, is_admin, tg_chat_id, joined_at, ip_address 
		FROM users 
		WHERE status IN ('waiting', 'frozen') AND is_admin = FALSE
		ORDER BY joined_at
	`)
	if err != nil {
		log.Println("Error loading queue:", err)
		return []*User{}
	}
	defer rows.Close()

	var loadedQueue []*User
	for rows.Next() {
		var u User
		var joinedAt string
		rows.Scan(&u.ID, &u.Name, &u.Ticket, &u.Status, &u.IsAdmin, &u.TgChatID, &joinedAt, &u.IPAddress)

		t, _ := time.Parse(time.RFC3339, joinedAt)
		u.JoinedAt = t.Unix()
		
		loadedQueue = append(loadedQueue, &u)
	}
	return loadedQueue
}

func loadCurrentServing() *User {
	var userID string
	err := db.QueryRow("SELECT current_user_id FROM queue_state WHERE key = 'main'").Scan(&userID)
	if err != nil || userID == "" {
		return nil
	}

	var u User
	var joinedAt string
	err = db.QueryRow("SELECT id, name, ticket, status, is_admin, tg_chat_id, joined_at, ip_address FROM users WHERE id = ?", userID).Scan(&u.ID, &u.Name, &u.Ticket, &u.Status, &u.IsAdmin, &u.TgChatID, &joinedAt, &u.IPAddress)
	if err != nil {
		return nil
	}

	t, _ := time.Parse(time.RFC3339, joinedAt)
	u.JoinedAt = t.Unix()
	
	return &u
}

func saveCurrentServing(user *User) {
	if user == nil {
		db.Exec("UPDATE queue_state SET current_user_id = NULL WHERE key = 'main'")
	} else {
		db.Exec("UPDATE queue_state SET current_user_id = ? WHERE key = 'main'", user.ID)
	}
}

func updateUserStatus(userID, status string) {
	db.Exec("UPDATE users SET status = ? WHERE id = ?", status, userID)
}

func updateUserTelegram(userID string, tgChatID int64) {
	db.Exec("UPDATE users SET tg_chat_id = ? WHERE id = ?", tgChatID, userID)
}
