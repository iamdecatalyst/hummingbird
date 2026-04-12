package config

import (
	"os"
	"strconv"
)

type Config struct {
	// Mode
	MultiTenant bool

	// Cricket Protocol — required for all scoring and signal detection
	// Sign up at https://cricket.vylth.com
	CricketURL string // e.g. https://api-cricket.vylth.com
	CricketKey string // CRICKET_API_KEY from your Cricket dashboard

	// Signet (single-tenant only — optional in multi-tenant)
	SignetAPIKey    string
	SignetAPISecret string
	SignetBaseURL   string

	// Solana RPC
	SolanaRPC string

	// Telegram bot (single-tenant only)
	TelegramToken     string
	TelegramChatID    string
	TelegramChannelID string // public broadcast channel

	// Server
	Port string

	// Risk
	MaxConcurrentPositions int
	MaxDailyLossPercent    float64

	// Multi-tenant
	DatabaseURL   string // postgres://...
	EncryptionKey string // 64 hex chars = 32 bytes for AES-256
	JWTSecret     string
}

func Load() *Config {
	return &Config{
		MultiTenant: getBool("MULTI_TENANT", false),

		CricketURL: getEnv("CRICKET_API_URL", "https://api-cricket.vylth.com"),
		CricketKey: getEnv("CRICKET_API_KEY", ""),

		SignetAPIKey:    getEnv("SIGNET_API_KEY", ""),
		SignetAPISecret: getEnv("SIGNET_API_SECRET", ""),
		SignetBaseURL:   getEnv("SIGNET_BASE_URL", "https://api.signet.vylth.com/v1"),

		SolanaRPC: getEnv("RPC_HTTP", "https://api.mainnet-beta.solana.com"),

		TelegramToken:     getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:    getEnv("TELEGRAM_CHAT_ID", ""),
		TelegramChannelID: getEnv("TELEGRAM_CHANNEL_ID", ""),

		Port: getEnv("ORCHESTRATOR_PORT", "8002"),

		MaxConcurrentPositions: getInt("MAX_CONCURRENT_POSITIONS", 5),
		MaxDailyLossPercent:    getFloat("MAX_DAILY_LOSS_PERCENT", 0.30),

		DatabaseURL:   getEnv("DATABASE_URL", ""),
		EncryptionKey: getEnv("ENCRYPTION_KEY", ""),
		JWTSecret:     getEnv("JWT_SECRET", "change-me-in-production"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
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
