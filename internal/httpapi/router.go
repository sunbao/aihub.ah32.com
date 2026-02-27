package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(serverErrorLoggerMiddleware)
	r.Use(corsMiddleware)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(newIPRateLimiter(120, time.Minute).middleware)
	r.Use(middleware.Heartbeat("/healthz"))

	s := server{
		db:                     d.DB,
		pepper:                 d.Pepper,
		adminToken:             d.AdminToken,
		publicBaseURL:          d.PublicBaseURL,
		githubClientID:         d.GitHubOAuthClientID,
		githubClientSecret:     d.GitHubOAuthClientSecret,
		skillsGatewayWhitelist: d.SkillsGatewayWhitelist,
		br:                     newBroker(),

		platformKeysEncryptionKey: d.PlatformKeysEncryptionKey,
		platformCertIssuer:        d.PlatformCertIssuer,
		platformCertTTLSeconds:    d.PlatformCertTTLSeconds,
		promptViewMaxChars:        d.PromptViewMaxChars,

		ossProvider:           d.OSSProvider,
		ossEndpoint:           d.OSSEndpoint,
		ossRegion:             d.OSSRegion,
		ossBucket:             d.OSSBucket,
		ossBasePrefix:         d.OSSBasePrefix,
		ossAccessKeyID:        d.OSSAccessKeyID,
		ossAccessKeySecret:    d.OSSAccessKeySecret,
		ossSTSRoleARN:         d.OSSSTSRoleARN,
		ossSTSDurationSeconds: d.OSSSTSDurationSeconds,
		ossLocalDir:           d.OSSLocalDir,
		ossEventsIngestToken:  d.OSSEventsIngestToken,
	}
	s.publishMinCompletedWorkItems = d.PublishMinCompletedWorkItems
	s.matchingParticipantCount = d.MatchingParticipantCount
	s.workItemLeaseSeconds = d.WorkItemLeaseSeconds

	// Start background scheduler for scheduled work items
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			s.schedulePendingWorkItems(ctx)
			cancel()
		}
	}()

	appUI, err := appFileServer()
	if err != nil {
		logErrorNoCtx("init app ui failed", err)

		// If embed fails, keep API usable.
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("app ui unavailable (frontend assets not embedded)\n")); err != nil {
				logError(r.Context(), "write app ui unavailable message failed", err)
			}
		})
		r.Get("/app", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			if _, err := w.Write([]byte("app ui unavailable (frontend assets not embedded)\n")); err != nil {
				logError(r.Context(), "write app ui unavailable message failed", err)
			}
		})
	} else {
		r.Get("/app", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/app/", http.StatusFound)
		})
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/app/", http.StatusFound)
		})
		r.Handle("/app/*", http.StripPrefix("/app/", appUI))
		r.Handle("/app/", http.StripPrefix("/app/", appUI))
	}

	r.Route("/v1", func(r chi.Router) {
		// Public runs list (for browsing/searching without remembering IDs).
		r.Get("/runs", s.handleListRunsPublic)
		// Public activity feed (latest key nodes).
		r.Get("/activity", s.handleListActivityPublic)

		// Public "cosmology" read APIs (OSS-backed).
		r.Get("/agents/{agentID}/dimensions", s.handleGetAgentDimensions)
		r.Get("/agents/{agentID}/daily-thought", s.handleGetAgentDailyThought)
		r.Get("/agents/{agentID}/highlights", s.handleGetAgentHighlights)
		r.Get("/curations", s.handleListCurations)

		// Public platform signing keyset (for agent-side verification).
		r.Get("/platform/signing-keys", s.handleListPlatformSigningKeys)

		// Public agent discovery (from platform projection).
		r.Get("/agents/discover", s.handleDiscoverAgents)
		r.Get("/agents/discover/{agentID}", s.handleDiscoverAgentDetail)

		// OAuth (GitHub).
		r.Get("/auth/github/start", s.handleAuthGitHubStart)
		r.Get("/auth/github/callback", s.handleAuthGitHubCallback)
		r.Post("/auth/app/exchange", s.handleAuthAppExchange)

		r.Group(func(r chi.Router) {
			r.Use(s.userAuthMiddleware)
			r.Get("/me", s.handleGetMe)
			r.Post("/agents", s.handleCreateAgent)
			r.Get("/agents", s.handleListAgents)
			r.Get("/agents/{agentID}", s.handleGetAgent)
			r.Delete("/agents/{agentID}", s.handleDeleteAgent)
			r.Patch("/agents/{agentID}", s.handleUpdateAgent)
			r.Post("/agents/{agentID}/disable", s.handleDisableAgent)
			r.Post("/agents/{agentID}/keys/rotate", s.handleRotateAgentKey)
			r.Put("/agents/{agentID}/tags", s.handleReplaceAgentTags)
			r.Post("/agents/{agentID}/tags", s.handleAddAgentTag)
			r.Delete("/agents/{agentID}/tags/{tag}", s.handleDeleteAgentTag)

			// Agent Home 32: owner-initiated sync/admission.
			r.Post("/agents/{agentID}/sync-to-oss", s.handleSyncAgentToOSS)
			r.Post("/agents/{agentID}/admission/start", s.handleAdmissionStart)
			r.Get("/agents/{agentID}/prompt-bundle", s.handleGetAgentPromptBundle)

			// Cosmology owner APIs.
			r.Get("/agents/{agentID}/timeline", s.handleOwnerGetTimeline)
			r.Post("/agents/{agentID}/swap-tests", s.handleOwnerCreateSwapTest)
			r.Get("/agents/{agentID}/swap-tests/{swapTestID}", s.handleOwnerGetSwapTest)
			r.Get("/agents/{agentID}/weekly-reports", s.handleOwnerGetWeeklyReport)
			r.Put("/agents/{agentID}/daily-thought", s.handleOwnerUpsertDailyThought)

			r.Post("/curations", s.handleCreateCuration)

			// Agent Card catalogs for wizard authoring (curated; cacheable via catalog_version).
			r.Get("/agent-card/catalogs", s.handleGetAgentCardCatalogs)

			// Persona templates (custom submission; requires admin approval before use).
			r.Get("/persona-templates", s.handleListApprovedPersonaTemplates)
			r.Post("/persona-templates", s.handleSubmitPersonaTemplate)

			r.Post("/runs", s.handleCreateRun)
		})

		r.Group(func(r chi.Router) {
			r.Use(s.agentAuthMiddleware)
			// Agent Home 32: admission + OSS access.
			r.Get("/agents/{agentID}/admission/challenge", s.handleAdmissionChallenge)
			r.Post("/agents/{agentID}/admission/complete", s.handleAdmissionComplete)

			r.Post("/oss/credentials", s.handleIssueOSSCredentials)
			r.Get("/oss/events/poll", s.handlePollOSSEvents)
			r.Post("/oss/events/ack", s.handleAckOSSEvents)

			r.Get("/gateway/inbox/poll", s.handleGatewayPoll)
			r.Get("/gateway/tasks", s.handleGatewayTasks)
			r.Get("/gateway/work-items/{workItemID}", s.handleGatewayGetWorkItem)
			r.Get("/gateway/work-items/{workItemID}/skills", s.handleGatewayWorkItemSkills)
			r.Post("/gateway/work-items/{workItemID}/claim", s.handleGatewayClaimWorkItem)
			r.Post("/gateway/work-items/{workItemID}/complete", s.handleGatewayCompleteWorkItem)
			r.Post("/gateway/runs/{runID}/events", s.handleGatewayEmitEvent)
			r.Post("/gateway/runs/{runID}/artifacts", s.handleGatewaySubmitArtifact)
			r.Post("/gateway/tools/invoke", s.handleGatewayInvokeTool)
		})

		// OSS event ingest webhook (optional; guarded by shared token).
		r.Group(func(r chi.Router) {
			r.Use(s.ossIngestAuthMiddleware)
			r.Post("/oss/events/ingest", s.handleIngestOSSEvents)
		})

		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Get("/", s.handleGetRunPublic)
			r.Get("/output", s.handleGetRunOutputPublic)
			r.Get("/stream", s.handleRunStreamSSE)
			r.Get("/replay", s.handleRunReplay)
			r.Get("/artifacts/{version}", s.handleGetRunArtifactPublic)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(s.adminAuthMiddleware)
			r.Post("/users/issue-key", s.handleAdminIssueUserKey)
			r.Get("/moderation/queue", s.handleAdminModerationQueue)
			r.Get("/moderation/{targetType}/{id}", s.handleAdminModerationGet)
			r.Post("/moderation/{targetType}/{id}/approve", s.handleAdminModerationApprove)
			r.Post("/moderation/{targetType}/{id}/reject", s.handleAdminModerationReject)
			r.Post("/moderation/{targetType}/{id}/unreject", s.handleAdminModerationUnreject)

			// Platform signing keys.
			r.Get("/platform/signing-keys", s.handleAdminListPlatformSigningKeys)
			r.Post("/platform/signing-keys/rotate", s.handleAdminRotatePlatformSigningKey)
			r.Post("/platform/signing-keys/{keyID}/revoke", s.handleAdminRevokePlatformSigningKey)

			// Persona template review.
			r.Get("/persona-templates", s.handleAdminListPersonaTemplates)
			r.Post("/persona-templates/{templateID}/approve", s.handleAdminApprovePersonaTemplate)
			r.Post("/persona-templates/{templateID}/reject", s.handleAdminRejectPersonaTemplate)

			// Curation review (OSS-backed).
			r.Post("/curations/{curationID}/approve", s.handleAdminApproveCuration)
			r.Post("/curations/{curationID}/reject", s.handleAdminRejectCuration)

			// OSS control plane (platform-owned objects in OSS).
			r.Post("/oss/circles", s.handleAdminCreateCircle)
			r.Post("/oss/circles/{circleID}/process-joins", s.handleAdminProcessCircleJoins)
			r.Post("/oss/tasks", s.handleAdminCreateTaskManifest)
			r.Post("/oss/topics", s.handleAdminCreateTopicManifest)
			r.Post("/oss/topics/{topicID}/state", s.handleAdminUpdateTopicState)
		})
	})

	return r
}
