package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sync"
	"time"
)

type Message struct {
	Client  string `json:"client"`
	Message string `json:"message"`
}

type ClientStats struct {
	Total    int
	LastMsg  string
	LastTime time.Time
	Speed    float64
}

var (
	stats     = make(map[string]*ClientStats)
	statsLock sync.Mutex
	clients   = make(map[chan string]bool)
)

func main() {
	http.HandleFunc("/message", messageHandler)
	http.HandleFunc("/dashboard", dashboardHandler)
	http.HandleFunc("/events", sseHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	log.Println("Сервер запущен на порту 8080")
	go broadcastLoop()
	http.ListenAndServe(":8080", nil)
}

func messageHandler(w http.ResponseWriter, r *http.Request) {
	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	statsLock.Lock()
	defer statsLock.Unlock()

	stat, exists := stats[msg.Client]
	if !exists {
		stat = &ClientStats{}
		stats[msg.Client] = stat
	}

	stat.Total++
	stat.LastMsg = msg.Message
	stat.LastTime = time.Now()
}

func dashboardHandler(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, "template error", 500)
		return
	}
	tmpl.Execute(w, nil)
}

func sseHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string)
	clients[ch] = true

	defer func() {
		delete(clients, ch)
		close(ch)
	}()

	for msg := range ch {
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	}
}

func broadcastLoop() {
	for {
		time.Sleep(1 * time.Second)

		statsLock.Lock()
		snapshot := make(map[string]ClientStats)
		for k, v := range stats {
			speed := float64(v.Total) / time.Since(v.LastTime).Seconds()
			v.Speed = speed
			snapshot[k] = *v
		}
		statsLock.Unlock()

		jsonData, _ := json.Marshal(snapshot)

		for ch := range clients {
			select {
			case ch <- string(jsonData):
			default:
			}
		}
	}
}
