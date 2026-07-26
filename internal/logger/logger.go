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

	b, err := json.Marshal(entry)
	if err != nil {
		log.Printf("logger: failed to marshal log entry: %v (level=%q, msg=%q)", err, entry.Level, entry.Msg)
		// Marshal again without Extra, which is the only field that can carry an
		// unmarshalable value. Building the fallback by hand would let a quote or
		// newline in Msg inject arbitrary JSON into the log stream.
		fallback, ferr := json.Marshal(LogMessage{
			Level: entry.Level,
			Time:  entry.Time,
			Msg:   "[marshal error] " + entry.Msg,
		})
		if ferr != nil {
			return
		}
		// Emit to the persistent log too, so the structured record survives for
		// anything parsing the log stream rather than only reaching live SSE
		// subscribers.
		log.Println(string(fallback))
		BroadcastLog(string(fallback))
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

func Warn(msg string) {
	LogWithLevel("warn", msg, nil)
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
