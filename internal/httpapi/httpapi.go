package httpapi

import "github.com/jackc/pgx/v5/pgxpool"

type Deps struct {
	DB                     *pgxpool.Pool
	Pepper                 string
	AdminToken             string
	PublicBaseURL          string
	GitHubOAuthClientID    string
	GitHubOAuthClientSecret string
	SkillsGatewayWhitelist []string

	PublishMinCompletedWorkItems int
	MatchingParticipantCount     int
	WorkItemLeaseSeconds         int
}
