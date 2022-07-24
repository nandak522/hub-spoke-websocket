package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var port = 8080
var workTime = 20

type Message struct {
	topic string
	data  string
}

type Subscriber struct {
	Id    string
	Queue chan Message
}

type PubSubGroup struct {
	publisherQueue chan Message
	subscribers    []Subscriber
}

type NotificationHub struct {
	sync.RWMutex
	registry map[string]PubSubGroup // topic vs publisher and subscriber(s) combination
}

func (hub *NotificationHub) GetTopics() []string {
	fmt.Println("Hub lock is getting acquired...")
	hub.RLock()
	fmt.Println("Hub lock acquired...")
	defer hub.RUnlock()
	topics := []string{}
	for topic := range hub.registry {
		topics = append(topics, topic)
	}
	return topics
}

func (hub *NotificationHub) SetPubSubGroupForTopic(topic string, pubSubGroup PubSubGroup) {
	fmt.Println("Hub lock is getting acquired...")
	hub.Lock()
	defer hub.Unlock()
	fmt.Println("Hub lock acquired...")
	hub.registry[topic] = pubSubGroup
}

func (hub *NotificationHub) ConcludePubSubGroupForTopic(topic string) {
	fmt.Println("Hub lock is getting acquired...")
	hub.Lock()
	defer hub.Unlock()
	fmt.Println("Hub lock acquired...")
	delete(hub.registry, topic)
}

func (hub *NotificationHub) GetPubSubGroupForTopic(topic string) (PubSubGroup, bool) {
	fmt.Println("Hub lock is getting acquired...")
	hub.RLock()
	defer hub.RUnlock()
	fmt.Println("Hub lock acquired...")
	if pubSubGroup, ok := hub.registry[topic]; ok {
		return pubSubGroup, true
	}
	return PubSubGroup{}, false
}

func (hub *NotificationHub) ClosePublisherForTopic(topic string) {
	fmt.Println("Hub lock is getting acquired...")
	hub.RLock()
	defer hub.RUnlock()
	fmt.Println("Hub lock acquired...")
	if pubSubGroup, ok := hub.registry[topic]; ok {
		fmt.Println("Closing the publisherQueue: ", pubSubGroup.publisherQueue)
		close(pubSubGroup.publisherQueue)
		fmt.Println("Closed the publisherQueue: ", pubSubGroup.publisherQueue)
		for _, subscriber := range pubSubGroup.subscribers {
			fmt.Println("Closing the subscriber: ", subscriber.Id)
			close(subscriber.Queue)
			fmt.Println("Closed the subscriber: ", subscriber.Id)
		}
	} else {
		fmt.Println(fmt.Sprintf("Topic '%s' not found in registry", topic))
	}
}

func homepageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "homepage.html")
}
func listenerHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "listener.html")
}

func sendMessage(hub *NotificationHub, topic, data string) {
	pubSubGroup, isTopicPresent := hub.GetPubSubGroupForTopic(topic)
	if !isTopicPresent {
		fmt.Println(fmt.Sprintf("Topic: %s not found in registry. Creating...", topic))
		pubSubGroup = PubSubGroup{
			publisherQueue: make(chan Message),
			subscribers:    []Subscriber{},
		}
		hub.SetPubSubGroupForTopic(topic, pubSubGroup)
	}
	fmt.Println(fmt.Sprintf("Sending data: '%s' to topic: '%s' to publisherQueue: %v", data, topic, pubSubGroup.publisherQueue))
	pubSubGroup.publisherQueue <- Message{
		topic: topic,
		data:  data,
	}
	fmt.Println(fmt.Sprintf("Data sent: '%s' to topic: '%s'", data, topic))
}

func (hub *NotificationHub) RegisterSubscriber(topic string, subscriberId string, wsConn *websocket.Conn) (Subscriber, error) {
	pubSubGroup, isTopicPresent := hub.GetPubSubGroupForTopic(topic)
	if !isTopicPresent {
		return Subscriber{}, errors.New(fmt.Sprintf("\t\t\t\t\t\t\t\t\tTopic: %s not found in registry.", topic))
	}
	fmt.Println(fmt.Sprintf("\t\t\t\t\t\t\t\t\tAll existing subscribers against the topic: %s are: %v", topic, pubSubGroup.subscribers))
	subscriber := Subscriber{
		Id:    subscriberId,
		Queue: make(chan Message),
	}
	pubSubGroup.subscribers = append(pubSubGroup.subscribers, subscriber)
	hub.SetPubSubGroupForTopic(topic, pubSubGroup)
	return subscriber, nil
}

func triggerWork(notificationHub *NotificationHub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error in upgrading connection. Reason:", err.Error())
		return
	}
	defer conn.Close()

	topic := r.URL.Query().Get("topic")
	// some work
	for i := 1; i <= workTime; i += 1 {
		msg := fmt.Sprintf("[Topic %s] Some work happening here... %d", topic, i)
		sendMessage(notificationHub, topic, msg)
		time.Sleep(1 * time.Second)
	}
	fmt.Println(fmt.Sprintf("[Topic %s] Work finished", topic))
	notificationHub.ClosePublisherForTopic(topic)
}

func generateClientIdHandler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(uuid.New().String()))
}

func listenForMessages(notificationHub *NotificationHub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Println("Error in upgrading connection. Reason:", err.Error())
		return
	}
	defer conn.Close()

	clientId := strings.Trim(r.URL.Query().Get("clientId"), " ")
	topic := strings.Trim(r.URL.Query().Get("topic"), " ")
	fmt.Println("clientId:", clientId)
	if clientId == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	subscriber, _ := notificationHub.RegisterSubscriber(topic, clientId, conn)
	for {
		select {
		case latestMsg, moreMsgs := <-subscriber.Queue:
			if !moreMsgs {
				fmt.Println(fmt.Sprintf("\t\t\t\t\t\t\t\t\t[Topic %s] %s Subscriber has nothing more to receive.", topic, subscriber.Id))
				conn.WriteMessage(1, []byte("Done"))
				return
			}
			fmt.Println(fmt.Sprintf("\t\t\t\t\t\t\t\t\t[Topic %s] %s Subscriber received: %s", topic, subscriber.Id, latestMsg.data))
			conn.WriteMessage(1, []byte(latestMsg.data))
		default:
			fmt.Println(fmt.Sprintf("\t\t\t\t\t\t\t\t\t%s Subscriber listening for new msgs. Nothing yet!", subscriber.Id))
			// time.Sleep(100 * time.Millisecond)
			time.Sleep(1 * time.Second)
		}
	}
}

func (hub *NotificationHub) RunConnector(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Println("Connector running...")
	for {
		for _, topic := range hub.GetTopics() {
			pubSubGroup, isTopicPresent := hub.GetPubSubGroupForTopic(topic)
			fmt.Println(fmt.Sprintf("topic: %v, isTopicPresent: %v pubSubGroup: %v", topic, isTopicPresent, pubSubGroup))
			if !isTopicPresent {
				break
			}
			select {
			case msg, moreMsgs := <-pubSubGroup.publisherQueue:
				if !moreMsgs {
					fmt.Println(fmt.Sprintf("[Topic %s] Publisher is done", topic))
					hub.ConcludePubSubGroupForTopic(topic)
					break
				}
				fmt.Println(fmt.Sprintf("\t\t\t\t\t\t\t\t\tConnector received msg: '%s' from publisher: %s", msg.data, topic))
				if len(pubSubGroup.subscribers) == 0 {
					fmt.Println(fmt.Sprintf("\t\t\t\t\t\t\t\t\tConnector didn't find any subscribers for %s Topic. Hence ignoring the received msg", topic))
					break
				}
				for i := 0; i < len(pubSubGroup.subscribers); i += 1 {
					fmt.Println(fmt.Sprintf("\t\t\t\t\t\t\t\t\tConnector sent msg: '%s' to subscriber: %s", msg.data, pubSubGroup.subscribers[i].Id))
					pubSubGroup.subscribers[i].Queue <- msg
				}
			default:
				fmt.Println(fmt.Sprintf("\t\t\t\t\t\t\t\t\tConnector listening for new msgs. Nothing yet!"))
				time.Sleep(1 * time.Second)
			}
		}
		fmt.Println("Revisiting all topics once more to check any pending msgs...")
		time.Sleep(1 * time.Second)
	}
}

func main() {
	hub := NotificationHub{
		registry: map[string]PubSubGroup{},
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

	var connectionWG sync.WaitGroup
	go hub.RunConnector(&connectionWG)
	connectionWG.Add(1)

	fmt.Println("http server ready to serve...")
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	if err != nil {
		panic(err.Error())
	}
	connectionWG.Wait()
}
