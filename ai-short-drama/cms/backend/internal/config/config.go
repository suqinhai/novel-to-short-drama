package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host           string
	Port           string
	DatabaseURL    string
	AllowedOrigins []string
	N8NHealthURL   string
	N8NProjectURL  string
	N8NStage2URL   string
	N8NStage3URL   string
	N8NStage4URL   string
	N8NStage5URL   string
	MediaHealthURL string
	MediaPublicURL string
	ProbeTimeout   time.Duration
	WebhookTimeout time.Duration
}

func Load() (Config, error) {
	loadEnvironmentFiles()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		host := env("POSTGRES_HOST", "127.0.0.1")
		port := env("POSTGRES_PORT", "5432")
		user := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
		password := os.Getenv("POSTGRES_PASSWORD")
		database := env("DRAMA_DB", "short_drama")
		if user == "" || password == "" {
			return Config{}, fmt.Errorf("database configuration is incomplete: set DATABASE_URL or POSTGRES_USER/POSTGRES_PASSWORD")
		}
		databaseURL = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", urlEncode(user), urlEncode(password), host, port, database)
	}

	timeoutSeconds, err := strconv.Atoi(env("CMS_PROBE_TIMEOUT_SECONDS", "3"))
	if err != nil || timeoutSeconds <= 0 {
		timeoutSeconds = 3
	}
	webhookTimeoutSeconds, err := strconv.Atoi(env("CMS_N8N_WEBHOOK_TIMEOUT_SECONDS", "900"))
	if err != nil || webhookTimeoutSeconds <= 0 {
		webhookTimeoutSeconds = 900
	}

	return Config{
		Host:           env("CMS_HOST", "127.0.0.1"),
		Port:           env("CMS_PORT", "8080"),
		DatabaseURL:    databaseURL,
		AllowedOrigins: splitCSV(env("CMS_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")),
		N8NHealthURL:   env("CMS_N8N_HEALTH_URL", "http://127.0.0.1:5678/healthz"),
		N8NProjectURL:  env("CMS_N8N_PROJECT_WEBHOOK_URL", "http://127.0.0.1:5678/webhook/ai-short-drama/projects"),
		N8NStage2URL:   env("CMS_N8N_STAGE2_WEBHOOK_URL", "http://127.0.0.1:5678/webhook/ai-short-drama/stage2"),
		N8NStage3URL:   env("CMS_N8N_STAGE3_WEBHOOK_URL", "http://127.0.0.1:5678/webhook/ai-short-drama/stage3"),
		N8NStage4URL:   env("CMS_N8N_STAGE4_WEBHOOK_URL", "http://127.0.0.1:5678/webhook/ai-short-drama/stage4"),
		N8NStage5URL:   env("CMS_N8N_STAGE5_WEBHOOK_URL", "http://127.0.0.1:5678/webhook/ai-short-drama/stage5"),
		MediaHealthURL: env("CMS_MEDIA_HEALTH_URL", "http://127.0.0.1:8088/healthz"),
		MediaPublicURL: strings.TrimRight(env("MEDIA_PUBLIC_BASE_URL", "http://127.0.0.1:8088"), "/"),
		ProbeTimeout:   time.Duration(timeoutSeconds) * time.Second,
		WebhookTimeout: time.Duration(webhookTimeoutSeconds) * time.Second,
	}, nil
}

func loadEnvironmentFiles() {
	candidates := []string{}
	if explicit := strings.TrimSpace(os.Getenv("CMS_ENV_FILE")); explicit != "" {
		candidates = append(candidates, explicit)
	}
	candidates = append(candidates, ".env", filepath.Join("..", "..", ".env"), filepath.Join("..", "..", ".env.example"))

	for _, path := range candidates {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.Trim(strings.TrimSpace(value), "\"'")
			if key != "" {
				if _, exists := os.LookupEnv(key); !exists {
					_ = os.Setenv(key, value)
				}
			}
		}
		_ = file.Close()
		return
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func urlEncode(value string) string {
	replacer := strings.NewReplacer("%", "%25", ":", "%3A", "/", "%2F", "@", "%40", "?", "%3F", "#", "%23")
	return replacer.Replace(value)
}
