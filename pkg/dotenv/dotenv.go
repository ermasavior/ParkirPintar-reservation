package dotenv

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnv(args ...string) {
	loc := ".env"
	if len(args) > 1 {
		loc = args[0]
	}

	_, err := os.Stat(loc)
	if err != nil {
		slog.Error("failed locating file", slog.Any("error", err))
		return
	}

	err = godotenv.Load()
	if err != nil {
		slog.Error("error loading .env file", slog.Any("error", err))
		return
	}
}

func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
