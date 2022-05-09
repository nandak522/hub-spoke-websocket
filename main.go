package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	clients   []*Client
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

func listenForMessages(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error in upgrading connection. Reason:", err.Error())
		return
	}
	defer conn.Close()

	hub.clients = append(hub.clients, &Client{
		wsConn: conn,
	})
	fmt.Println("Client registered. Clients set now:", hub.clients)
}

func broadcastAllIncomingMessages(hub *Hub) {
	for {
		msg := <-hub.broadcast
		fmt.Println("Msg received from broadcast:", msg)
		fmt.Println("Msg will be broadcasted to clients:", hub.clients)
		for _, client := range hub.clients {
			client.wsConn.WriteMessage(1, msg)
		}
	}
}

func main() {
	hub := Hub{
		broadcast: make(chan []byte),
	}
	http.HandleFunc("/", homepageHandler)
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
