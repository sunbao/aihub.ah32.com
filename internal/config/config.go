package config

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL                  string
	HTTPAddr                     string
	APIKeyPepper                 string
	AdminToken                   string
	PublishMinCompletedWorkItems int
	SkillsGatewayWhitelist       []string

	MatchingParticipantCount int
	WorkItemLeaseSeconds     int
	WorkerTickSeconds        int
}

func Load() (Config, error) {
	// Optional: load local .env for development. Missing file is fine.
	_ = godotenv.Load()

	minCompleted := getenvIntDefault("AIHUB_PUBLISH_MIN_COMPLETED_WORK_ITEMS", 1)
	if minCompleted < 1 {
		minCompleted = 1
	}

	participantCount := getenvIntDefault("AIHUB_MATCHING_PARTICIPANT_COUNT", 3)
	if participantCount < 1 {
		participantCount = 1
	}

	leaseSeconds := getenvIntDefault("AIHUB_WORK_ITEM_LEASE_SECONDS", 300)
	if leaseSeconds < 30 {
		leaseSeconds = 30
	}

	workerTick := getenvIntDefault("AIHUB_WORKER_TICK_SECONDS", 5)
	if workerTick < 1 {
		workerTick = 1
	}

	cfg := Config{
		DatabaseURL:                  os.Getenv("AIHUB_DATABASE_URL"),
		HTTPAddr:                     getenvDefault("AIHUB_HTTP_ADDR", ":8080"),
		APIKeyPepper:                 os.Getenv("AIHUB_API_KEY_PEPPER"),
		AdminToken:                   os.Getenv("AIHUB_ADMIN_TOKEN"),
		PublishMinCompletedWorkItems: minCompleted,
		SkillsGatewayWhitelist:       getenvCSV("AIHUB_SKILLS_GATEWAY_WHITELIST"),
		MatchingParticipantCount:     participantCount,
		WorkItemLeaseSeconds:         leaseSeconds,
		WorkerTickSeconds:            workerTick,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("AIHUB_DATABASE_URL is required")
	}
	if cfg.APIKeyPepper == "" {
		return Config{}, errors.New("AIHUB_API_KEY_PEPPER is required")
	}
	return cfg, nil
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvIntDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getenvCSV(key string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}

	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}
