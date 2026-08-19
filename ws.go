package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Controller struct {
	mu    sync.Mutex
	conns map[*websocket.Conn]bool
}

type UpEvent struct {
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}

func (con *Controller) Broadcast(event string, payload interface{}) {
	con.mu.Lock()
	defer con.mu.Unlock()

	data, err := json.Marshal(UpEvent{Event: event, Payload: payload})
	if err != nil {
		fmt.Println("bad payload", err)
		return
	}

	for conn := range con.conns {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(con.conns, conn)
		}
	}
}

func (con *Controller) AddConn(conn *websocket.Conn) {
	con.mu.Lock()
	defer con.mu.Unlock()

	con.conns[conn] = true
}

func (con *Controller) DelConn(conn *websocket.Conn) {
	con.mu.Lock()
	defer con.mu.Unlock()

	delete(con.conns, conn)
	conn.Close()
}

func handleWS(con *Controller) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")

		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		if _, err := ValJWT(token); err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade error: %v", err)
			return
		}
		con.AddConn(conn)
		fmt.Println("websocket client connected")

		defer con.DelConn(conn)

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}

	}
}
