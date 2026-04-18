package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"uplarr/internal/api"
	"uplarr/internal/logger"
	"uplarr/internal/models"
	"uplarr/internal/queue"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		i, err := strconv.Atoi(value)
		if err != nil {
			return fallback
		}
		return i
	}
	return fallback
}

var (
	httpListenAndServe = http.ListenAndServe
	apiSetupApp        = api.SetupApp
	osExit             = os.Exit
)

func Run() error {
	config := models.Config{
		LocalDir:     getEnv("LOCAL_DIR", "./test_data"),
		ConfigDir:    getEnv("CONFIG_DIR", "./config"),
		WebPort:      getEnv("WEB_PORT", "8080"),
		AuthPassword: getEnv("AUTH_PASSWORD", ""),
	}

	qm := queue.NewQueueManager(config.LocalDir, config.ConfigDir)

	mux, err := apiSetupApp(config, qm)
	if err != nil {
		return fmt.Errorf("setup failed: %v", err)
	}

	logger.Info(fmt.Sprintf("Server starting on port: %s (binding to 0.0.0.0)", config.WebPort))
	return httpListenAndServe("0.0.0.0:"+config.WebPort, mux)
}

func main() {
	if err := Run(); err != nil {
		logger.Error(fmt.Sprintf("Application failed: %v", err))
		osExit(1)
	}
}
