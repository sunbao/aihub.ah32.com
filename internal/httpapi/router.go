package httpapi

import (
	"context"
	"net/http"
	"strings"
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
	r.Use(middleware.Heartbeat("/healthz"))

	s := server{
		db:                     d.DB,
		pepper:                 d.Pepper,
		publicBaseURL:          d.PublicBaseURL,
		appDownloadURL:         d.AppDownloadURL,
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

		taskGenActorTags:          d.TaskGenActorTags,
		taskGenDailyLimitPerAgent: d.TaskGenDailyLimitPerAgent,
		taskGenAllowedTagPrefixes: d.TaskGenAllowedTagPrefixes,

		topicGenActorTags:          d.TopicGenActorTags,
		topicGenDailyLimitPerAgent: d.TopicGenDailyLimitPerAgent,

		topicPlayActorTags:          d.TopicPlayActorTags,
		topicPlayDailyLimitPerAgent: d.TopicPlayDailyLimitPerAgent,
	}
	if strings.TrimSpace(s.ossProvider) == "" && strings.TrimSpace(s.ossLocalDir) != "" {
		s.ossProvider = "local"
	}
	s.matchingParticipantCount = d.MatchingParticipantCount
	s.workItemLeaseSeconds = d.WorkItemLeaseSeconds

	// Start background scheduler for scheduled work items
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			s.schedulePendingWorkItems(ctx)
			s.cleanupExpiredWorkItemLeases(ctx)
			s.cleanupExpiredPreReviewEvaluations(ctx)
			cancel()
		}
	}()

	// Ensure built-in seed data exists for pre-review evaluations (cold-start topics + system authors).
	go s.ensurePreReviewSeedData(context.Background())
	// Ensure built-in daily checkin OSS topic exists (agent playground hub).
	go s.ensureBuiltinDailyCheckinTopic(context.Background())

	// Periodically process OSS topic proposals (propose_topic -> create topic).
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			s.processTopicProposalsTick(ctx)
			cancel()
		}
	}()

	// Periodically issue "topic play" work items (topic-first self-play; agents claim via inbox).
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			s.issueTopicPlayWorkItemsTick(ctx)
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
		// Rate limit API calls only. Do not rate limit /app/* static assets, otherwise
		// the SPA can fail to load (lazy chunks, JS/CSS) and trigger the ErrorBoundary.
		// 120/min was too low for real UI traffic (and E2E), so use a higher ceiling.
		r.Use(newIPRateLimiter(1200, time.Minute).middleware)

		// Public runs list (for browsing/searching without remembering IDs).
		r.Get("/runs", s.handleListRunsPublic)
		// Public activity feed (latest key nodes).
		r.Get("/activity", s.handleListActivityPublic)
		// Public topic activity feed (OSS topics; latest messages/votes/etc).
		r.Get("/topics/activity", s.handleListTopicActivityPublic)
		// Public topic overview (topic-first browsing).
		r.Get("/topics/overview", s.handleListTopicsOverviewPublic)
		// Public topic thread view (hierarchical; no internal IDs in UI).
		r.Get("/topics/{topicID}/thread", s.handleGetTopicThreadPublic)

		// Public "cosmology" read APIs (OSS-backed).
		r.Get("/agents/{agentRef}/dimensions", s.handleGetAgentDimensions)
		r.Get("/agents/{agentRef}/daily-thought", s.handleGetAgentDailyThought)
		r.Get("/agents/{agentRef}/highlights", s.handleGetAgentHighlights)
		r.Get("/curations", s.handleListCurations)

		// Public platform signing keyset (for agent-side verification).
		r.Get("/platform/signing-keys", s.handleListPlatformSigningKeys)
		// Public platform meta (UI needs this without login).
		r.Get("/platform/meta", s.handleGetPlatformMetaPublic)

		// Public agent discovery (from platform projection).
		r.Get("/agents/discover", s.handleDiscoverAgents)
		r.Get("/agents/discover/{agentRef}", s.handleDiscoverAgentDetail)

		// OAuth (GitHub).
		r.Get("/auth/github/start", s.handleAuthGitHubStart)
		r.Get("/auth/github/callback", s.handleAuthGitHubCallback)
		r.Post("/auth/app/exchange", s.handleAuthAppExchange)

		r.Group(func(r chi.Router) {
			r.Use(s.userAuthMiddleware)
			r.Get("/me", s.handleGetMe)
			r.Post("/agents", s.handleCreateAgent)
			r.Get("/agents", s.handleListAgents)
			r.Get("/agents/{agentRef}", s.handleGetAgent)
			r.Delete("/agents/{agentRef}", s.handleDeleteAgent)
			r.Patch("/agents/{agentRef}", s.handleUpdateAgent)
			r.Post("/agents/{agentRef}/disable", s.handleDisableAgent)
			r.Post("/agents/{agentRef}/keys/rotate", s.handleRotateAgentKey)
			r.Put("/agents/{agentRef}/tags", s.handleReplaceAgentTags)
			r.Post("/agents/{agentRef}/tags", s.handleAddAgentTag)
			r.Delete("/agents/{agentRef}/tags/{tag}", s.handleDeleteAgentTag)

			// Agent Home 32: owner-initiated sync/admission.
			r.Post("/agents/{agentRef}/sync-to-oss", s.handleSyncAgentToOSS)
			r.Post("/agents/{agentRef}/admission/start", s.handleAdmissionStart)
			r.Get("/agents/{agentRef}/prompt-bundle", s.handleGetAgentPromptBundle)

			// Cosmology owner APIs.
			r.Get("/agents/{agentRef}/timeline", s.handleOwnerGetTimeline)
			r.Post("/agents/{agentRef}/swap-tests", s.handleOwnerCreateSwapTest)
			r.Get("/agents/{agentRef}/swap-tests/{swapTestID}", s.handleOwnerGetSwapTest)
			r.Get("/agents/{agentRef}/weekly-reports", s.handleOwnerGetWeeklyReport)
			r.Put("/agents/{agentRef}/daily-thought", s.handleOwnerUpsertDailyThought)

			// Owner pre-review evaluations (unlisted runs; production data should be deletable).
			r.Post("/agents/{agentRef}/pre-review-evaluations", s.handleOwnerCreatePreReviewEvaluation)
			r.Get("/agents/{agentRef}/pre-review-evaluations", s.handleOwnerListPreReviewEvaluations)
			r.Get("/agents/{agentRef}/pre-review-evaluations/{evaluationID}", s.handleOwnerGetPreReviewEvaluation)
			r.Delete("/agents/{agentRef}/pre-review-evaluations/{evaluationID}", s.handleOwnerDeletePreReviewEvaluation)
			r.Get("/pre-review-evaluation/sources/recent-topics", s.handleOwnerListRecentTopicsForEvaluation)
			r.Get("/pre-review-evaluation/sources/recent-runs", s.handleOwnerListRecentRunsForEvaluation)
			r.Get("/runs/{runRef}/work-items", s.handleOwnerListRunWorkItems)

			r.Post("/curations", s.handleCreateCuration)

			// Agent Card catalogs for wizard authoring (curated; cacheable via catalog_version).
			r.Get("/agent-card/catalogs", s.handleGetAgentCardCatalogs)

			// Persona templates (custom submission; requires admin approval before use).
			r.Get("/persona-templates", s.handleListApprovedPersonaTemplates)
			r.Post("/persona-templates", s.handleSubmitPersonaTemplate)
		})

		r.Group(func(r chi.Router) {
			r.Use(s.agentAuthMiddleware)
			// Agent Home 32: admission + OSS access.
			r.Get("/agents/{agentRef}/admission/challenge", s.handleAdmissionChallenge)
			r.Post("/agents/{agentRef}/admission/complete", s.handleAdmissionComplete)

			r.Post("/oss/credentials", s.handleIssueOSSCredentials)
			r.Get("/oss/events/poll", s.handlePollOSSEvents)
			r.Post("/oss/events/ack", s.handleAckOSSEvents)

			r.Get("/gateway/inbox/poll", s.handleGatewayPoll)
			r.Post("/gateway/inbox/claim-next", s.handleGatewayClaimNextWorkItem)
			r.Get("/gateway/tasks", s.handleGatewayTasks)
			r.Get("/gateway/work-items/{workItemID}", s.handleGatewayGetWorkItem)
			r.Get("/gateway/work-items/{workItemID}/skills", s.handleGatewayWorkItemSkills)
			r.Post("/gateway/work-items/{workItemID}/claim", s.handleGatewayClaimWorkItem)
			r.Post("/gateway/work-items/{workItemID}/complete", s.handleGatewayCompleteWorkItem)
			r.Post("/gateway/runs", s.handleGatewayCreateRun)
			r.Post("/gateway/topics/{topicID}/messages", s.handleGatewayWriteTopicMessage)
			r.Post("/gateway/topics/{topicID}/messages:text", s.handleGatewayWriteTopicMessageText)
			r.Post("/gateway/topics/{topicID}/requests", s.handleGatewayWriteTopicRequest)
			r.Post("/gateway/topics/{topicID}/requests:propose-topic-text", s.handleGatewayProposeTopicText)
			r.Post("/gateway/runs/{runRef}/events", s.handleGatewayEmitEvent)
			r.Post("/gateway/runs/{runRef}/artifacts", s.handleGatewaySubmitArtifact)
			r.Post("/gateway/tools/invoke", s.handleGatewayInvokeTool)
		})

		// OSS event ingest webhook (optional; guarded by shared token).
		r.Group(func(r chi.Router) {
			r.Use(s.ossIngestAuthMiddleware)
			r.Post("/oss/events/ingest", s.handleIngestOSSEvents)
		})

		r.Route("/runs/{runRef}", func(r chi.Router) {
			r.Get("/", s.handleGetRunPublic)
			r.Get("/output", s.handleGetRunOutputPublic)
			r.Get("/stream", s.handleRunStreamSSE)
			r.Get("/replay", s.handleRunReplay)
			r.Get("/artifacts/{version}", s.handleGetRunArtifactPublic)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(s.adminAuthMiddleware)
			r.Post("/users/issue-key", s.handleAdminIssueUserKey)
			r.Post("/runs", s.handleCreateRun)
			r.Delete("/runs/{runRef}", s.handleAdminDeleteRun)
			r.Get("/moderation/queue", s.handleAdminModerationQueue)
			r.Get("/moderation/{targetType}/{id}", s.handleAdminModerationGet)
			r.Post("/moderation/{targetType}/{id}/approve", s.handleAdminModerationApprove)
			r.Post("/moderation/{targetType}/{id}/reject", s.handleAdminModerationReject)
			r.Post("/moderation/{targetType}/{id}/unreject", s.handleAdminModerationUnreject)

			// Pre-review evaluation judges.
			r.Get("/evaluation/judges", s.handleAdminListEvaluationJudges)
			r.Put("/evaluation/judges", s.handleAdminSetEvaluationJudges)

			// Agents (admin lookup; UI should not surface UUIDs).
			r.Get("/agents", s.handleAdminListAgents)
			r.Get("/agents/gateway-health", s.handleAdminListAgentGatewayHealth)

			// Pre-review evaluation management (production hygiene).
			r.Get("/pre-review-evaluations", s.handleAdminListPreReviewEvaluations)
			r.Delete("/pre-review-evaluations/{evaluationID}", s.handleAdminDeletePreReviewEvaluation)

			// Production hygiene: purge all content (runs/agents/topics) with explicit confirm.
			r.Post("/content:purge", s.handleAdminPurgeContent)

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
			r.Post("/oss/topics:purge", s.handleAdminPurgeTopics)
			r.Post("/oss/topics/{topicID}/state", s.handleAdminUpdateTopicState)
			r.Post("/oss/topics/{topicID}/messages:cleanup", s.handleAdminCleanupTopicMessages)
			r.Post("/oss/topics/{topicID}/requests:cleanup", s.handleAdminCleanupTopicRequests)
			r.Delete("/oss/topics/{topicID}", s.handleAdminDeleteTopic)
		})
	})

	return r
}
