package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Hub struct {
	clients   map[string]*Client
	broadcast chan []byte
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var port = 8080
var workTime = 30

func homepageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "homepage.html")
}
func listenerHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "listener.html")
}

type Client struct {
	wsConn *websocket.Conn
	id     string
}

func triggerWork(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error in upgrading connection. Reason:", err.Error())
		return
	}
	defer conn.Close()

	// some work
	for i := 1; i <= workTime; i += 1 {
		msg := []byte(fmt.Sprintf("Some work happening here... %d", i))
		time.Sleep(1 * time.Second)
		conn.WriteMessage(1, msg)
		hub.broadcast <- msg
	}
}

func generateClientIdHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(uuid.New().String()))
}

func listenForMessages(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error in upgrading connection. Reason:", err.Error())
		return
	}
	defer conn.Close()

	clientId := strings.Trim(r.URL.Query().Get("clientId"), " ")
	fmt.Println("clientId:", clientId)
	if clientId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var client Client
	if _, ok := hub.clients[clientId]; ok {
		client = *(hub.clients[clientId])
		fmt.Println("Client already present. Clients set now:", hub.clients)
	} else {
		client = Client{
			wsConn: conn,
			id:     clientId,
		}

		hub.clients[clientId] = &client
		fmt.Println("Client registered freshly. Clients set now:", hub.clients)
	}

	_, latestMsg, err := conn.ReadMessage()
	if err != nil {
		fmt.Println("Error in retrieving the msg. Error:", err.Error())
		return
	}
	client.wsConn.WriteMessage(1, latestMsg)
}

func broadcastAllIncomingMessages(hub *Hub) {
	for {
		msg := <-hub.broadcast
		fmt.Println("Msg received from broadcast:", string(msg))
		fmt.Println("Msg will be broadcasted to clients:", hub.clients)
		for _, client := range hub.clients {
			client.wsConn.WriteMessage(1, msg)
		}
	}
}

func main() {
	hub := Hub{
		broadcast: make(chan []byte),
		clients:   make(map[string]*Client),
	}
	http.HandleFunc("/", homepageHandler)
	http.HandleFunc("/generate-client-id", generateClientIdHandler)
	http.HandleFunc("/triggerWork", func(w http.ResponseWriter, r *http.Request) {
		triggerWork(&hub, w, r)
	})
	http.HandleFunc("/listener", listenerHandler)
	http.HandleFunc("/listen", func(w http.ResponseWriter, r *http.Request) {
		listenForMessages(&hub, w, r)
	})
	go broadcastAllIncomingMessages(&hub)

	fmt.Println("http server ready to serve...")
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		panic(err.Error())
	}
}
