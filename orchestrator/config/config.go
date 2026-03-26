package config

import (
	"os"
	"strconv"
)

type Config struct {
	// Signet
	SignetAPIKey    string
	SignetAPISecret string
	SignetBaseURL   string

	// Solana RPC (for price + dev wallet polling)
	SolanaRPC string

	// Telegram bot
	TelegramToken  string
	TelegramChatID string

	// Server
	Port string

	// Risk
	MaxConcurrentPositions int
	MaxDailyLossPercent    float64 // e.g. 0.30 = 30%
}

func Load() *Config {
	return &Config{
		SignetAPIKey:    mustEnv("SIGNET_API_KEY"),
		SignetAPISecret: mustEnv("SIGNET_API_SECRET"),
		SignetBaseURL:   getEnv("SIGNET_BASE_URL", "https://api.signet.vylth.com/v1"),

		SolanaRPC: getEnv("RPC_HTTP", "https://api.mainnet-beta.solana.com"),

		TelegramToken:  getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID: getEnv("TELEGRAM_CHAT_ID", ""),

		Port: getEnv("ORCHESTRATOR_PORT", "8002"),

		MaxConcurrentPositions: getInt("MAX_CONCURRENT_POSITIONS", 5),
		MaxDailyLossPercent:    getFloat("MAX_DAILY_LOSS_PERCENT", 0.30),
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("required env var not set: " + key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
