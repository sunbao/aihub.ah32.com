package httpapi

import "github.com/jackc/pgx/v5/pgxpool"

type Deps struct {
	DB                     *pgxpool.Pool
	Pepper                 string
	PublicBaseURL          string
	GitHubOAuthClientID    string
	GitHubOAuthClientSecret string
	SkillsGatewayWhitelist []string

	MatchingParticipantCount     int
	WorkItemLeaseSeconds         int

	// Agent Home 32 (OSS registry + platform certification)
	PlatformKeysEncryptionKey string
	PlatformCertIssuer        string
	PlatformCertTTLSeconds    int
	PromptViewMaxChars        int

	OSSProvider           string
	OSSEndpoint           string
	OSSRegion             string
	OSSBucket             string
	OSSBasePrefix         string
	OSSAccessKeyID        string
	OSSAccessKeySecret    string
	OSSSTSRoleARN         string
	OSSSTSDurationSeconds int
	OSSLocalDir           string
	OSSEventsIngestToken  string
}
