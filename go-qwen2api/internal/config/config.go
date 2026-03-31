package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DataSaveMode        string
	APIKeys             []string
	AdminKey            string
	ListenAddress       string
	ListenPort          int
	AutoRefresh         bool
	AutoRefreshInterval int
	LogLevel            string
	EnableFileLog       bool
	LogDir              string
	MaxLogFileSize      int
	MaxLogFiles         int
	QwenChatProxyURL    string
	QwenCliProxyURL     string
	ProxyURL            string
	MaxCliRequests      int
}

var C *Config

func Load() {
	apiKeyEnv := os.Getenv("API_KEY")
	var apiKeys []string
	var adminKey string
	if apiKeyEnv != "" {
		for _, k := range strings.Split(apiKeyEnv, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				apiKeys = append(apiKeys, k)
			}
		}
		if len(apiKeys) > 0 {
			adminKey = apiKeys[0]
		}
	}
	C = &Config{
		DataSaveMode:        envOr("DATA_SAVE_MODE", "none"),
		APIKeys:             apiKeys,
		AdminKey:            adminKey,
		ListenAddress:       os.Getenv("LISTEN_ADDRESS"),
		ListenPort:          envInt("SERVICE_PORT", 3000),
		AutoRefresh:         true,
		AutoRefreshInterval: envInt("AUTO_REFRESH_INTERVAL", 21600),
		LogLevel:            envOr("LOG_LEVEL", "INFO"),
		EnableFileLog:       envBool("ENABLE_FILE_LOG"),
		LogDir:              envOr("LOG_DIR", "./logs"),
		MaxLogFileSize:      envInt("MAX_LOG_FILE_SIZE", 10),
		MaxLogFiles:         envInt("MAX_LOG_FILES", 5),
		QwenChatProxyURL:    envOr("QWEN_CHAT_PROXY_URL", "https://chat.qwen.ai"),
		QwenCliProxyURL:     envOr("QWEN_CLI_PROXY_URL", "https://portal.qwen.ai"),
		ProxyURL:            os.Getenv("PROXY_URL"),
		MaxCliRequests:      envInt("MAX_CLI_REQUESTS", 2000),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	return strings.EqualFold(os.Getenv(key), "true")
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}
