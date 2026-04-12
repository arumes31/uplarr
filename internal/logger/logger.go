package logger

import (
	"encoding/json"
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

	b, _ := json.Marshal(entry)
	log.Println(string(b))
	BroadcastLog(string(b))
}

func Info(msg string) {
	LogWithLevel("info", msg, nil)
}

func Error(msg string) {
	LogWithLevel("error", msg, nil)
}
