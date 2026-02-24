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
	PublicBaseURL                string
	GitHubOAuthClientID          string
	GitHubOAuthClientSecret      string
	PublishMinCompletedWorkItems int
	SkillsGatewayWhitelist       []string

	MatchingParticipantCount int
	WorkItemLeaseSeconds     int
	WorkerTickSeconds        int

	// Agent Home 32 (OSS registry + platform certification)
	PlatformKeysEncryptionKey string
	PlatformCertIssuer        string
	PlatformCertTTLSeconds    int
	PromptViewMaxChars        int

	OSSProvider          string // "aliyun" | "local" | ""
	OSSEndpoint          string
	OSSRegion            string
	OSSBucket            string
	OSSBasePrefix        string
	OSSAccessKeyID       string
	OSSAccessKeySecret   string
	OSSSTSRoleARN        string
	OSSSTSDurationSeconds int
	OSSLocalDir          string
	OSSEventsIngestToken string
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

	certTTLSeconds := getenvIntDefault("AIHUB_PLATFORM_CERT_TTL_SECONDS", 86400*30) // 30 days
	if certTTLSeconds < 60 {
		certTTLSeconds = 60
	}

	promptViewMaxChars := getenvIntDefault("AIHUB_PROMPT_VIEW_MAX_CHARS", 600)
	if promptViewMaxChars < 100 {
		promptViewMaxChars = 100
	}
	if promptViewMaxChars > 2000 {
		promptViewMaxChars = 2000
	}

	stsDuration := getenvIntDefault("AIHUB_OSS_STS_DURATION_SECONDS", 900) // 15 minutes
	if stsDuration < 60 {
		stsDuration = 60
	}
	if stsDuration > 3600 {
		stsDuration = 3600
	}

	cfg := Config{
		DatabaseURL:                  os.Getenv("AIHUB_DATABASE_URL"),
		HTTPAddr:                     getenvDefault("AIHUB_HTTP_ADDR", ":8080"),
		APIKeyPepper:                 os.Getenv("AIHUB_API_KEY_PEPPER"),
		AdminToken:                   os.Getenv("AIHUB_ADMIN_TOKEN"),
		PublicBaseURL:                strings.TrimRight(strings.TrimSpace(os.Getenv("AIHUB_PUBLIC_BASE_URL")), "/"),
		GitHubOAuthClientID:          strings.TrimSpace(os.Getenv("AIHUB_GITHUB_OAUTH_CLIENT_ID")),
		GitHubOAuthClientSecret:      strings.TrimSpace(os.Getenv("AIHUB_GITHUB_OAUTH_CLIENT_SECRET")),
		PublishMinCompletedWorkItems: minCompleted,
		SkillsGatewayWhitelist:       getenvCSV("AIHUB_SKILLS_GATEWAY_WHITELIST"),
		MatchingParticipantCount:     participantCount,
		WorkItemLeaseSeconds:         leaseSeconds,
		WorkerTickSeconds:            workerTick,

		PlatformKeysEncryptionKey: strings.TrimSpace(os.Getenv("AIHUB_PLATFORM_KEYS_ENCRYPTION_KEY")),
		PlatformCertIssuer:        getenvDefault("AIHUB_PLATFORM_CERT_ISSUER", "aihub"),
		PlatformCertTTLSeconds:    certTTLSeconds,
		PromptViewMaxChars:        promptViewMaxChars,

		OSSProvider:           strings.TrimSpace(os.Getenv("AIHUB_OSS_PROVIDER")),
		OSSEndpoint:           strings.TrimSpace(os.Getenv("AIHUB_OSS_ENDPOINT")),
		OSSRegion:             strings.TrimSpace(os.Getenv("AIHUB_OSS_REGION")),
		OSSBucket:             strings.TrimSpace(os.Getenv("AIHUB_OSS_BUCKET")),
		OSSBasePrefix:         strings.Trim(strings.TrimSpace(os.Getenv("AIHUB_OSS_BASE_PREFIX")), "/"),
		OSSAccessKeyID:        strings.TrimSpace(os.Getenv("AIHUB_OSS_ACCESS_KEY_ID")),
		OSSAccessKeySecret:    strings.TrimSpace(os.Getenv("AIHUB_OSS_ACCESS_KEY_SECRET")),
		OSSSTSRoleARN:         strings.TrimSpace(os.Getenv("AIHUB_OSS_STS_ROLE_ARN")),
		OSSSTSDurationSeconds: stsDuration,
		OSSLocalDir:           strings.TrimSpace(os.Getenv("AIHUB_OSS_LOCAL_DIR")),
		OSSEventsIngestToken:  strings.TrimSpace(os.Getenv("AIHUB_OSS_EVENTS_INGEST_TOKEN")),
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
