package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	LogClients = make(map[chan string]bool)
	Mu         sync.Mutex
)

type LogMessage struct {
	Level string      `json:"level"`
	Time  string      `json:"time"`
	Msg   string      `json:"msg"`
	Extra interface{} `json:"extra,omitempty"`
}

func BroadcastLog(msg string) {
	Mu.Lock()
	defer Mu.Unlock()
	for c := range LogClients {
		select {
		case c <- msg:
		default:
		}
	}
}

func LogWithLevel(level, msg string, extra interface{}) {
	entry := LogMessage{
		Level: level,
		Time:  time.Now().Format(time.RFC3339),
		Msg:   msg,
		Extra: extra,
	}

	b, err := json.Marshal(entry)
	if err != nil {
		log.Printf("logger: failed to marshal log entry: %v (level=%s, msg=%s)", err, entry.Level, entry.Msg)
		// Emit a safe fallback JSON string
		fallback := fmt.Sprintf(`{"level":"%s","msg":"[marshal error] %s","time":"%s"}`, entry.Level, entry.Msg, entry.Time)
		BroadcastLog(fallback)
		return
	}
	log.Println(string(b))
	BroadcastLog(string(b))
}

func Info(msg string) {
	LogWithLevel("info", msg, nil)
}

func Error(msg string) {
	LogWithLevel("error", msg, nil)
}

// Subscribe creates a buffered log channel, registers it, and returns it.
func Subscribe() chan string {
	c := make(chan string, 10)
	Mu.Lock()
	LogClients[c] = true
	Mu.Unlock()
	return c
}

// Unsubscribe removes the channel from LogClients and closes it.
func Unsubscribe(c chan string) {
	Mu.Lock()
	delete(LogClients, c)
	Mu.Unlock()
	close(c)
}
