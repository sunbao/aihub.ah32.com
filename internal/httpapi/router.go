package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(newIPRateLimiter(120, time.Minute).middleware)
	r.Use(middleware.Heartbeat("/healthz"))

	s := server{db: d.DB, pepper: d.Pepper, br: newBroker()}
	s.publishMinCompletedWorkItems = d.PublishMinCompletedWorkItems
	s.matchingParticipantCount = d.MatchingParticipantCount
	s.workItemLeaseSeconds = d.WorkItemLeaseSeconds

	ui, err := webFileServer()
	if err != nil {
		// If embed fails, keep API usable.
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("web ui unavailable"))
		})
	} else {
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/ui/", http.StatusFound)
		})
		r.Handle("/ui/*", http.StripPrefix("/ui/", ui))
		r.Handle("/ui/", http.StripPrefix("/ui/", ui))
	}

	r.Route("/v1", func(r chi.Router) {
		// Public runs list (for browsing/searching without remembering IDs).
		r.Get("/runs", s.handleListRunsPublic)

		r.Post("/users", s.handleCreateUser)

		r.Group(func(r chi.Router) {
			r.Use(s.userAuthMiddleware)
			r.Get("/me", s.handleGetMe)
			r.Post("/agents", s.handleCreateAgent)
			r.Get("/agents", s.handleListAgents)
			r.Delete("/agents/{agentID}", s.handleDeleteAgent)
			r.Patch("/agents/{agentID}", s.handleUpdateAgent)
			r.Post("/agents/{agentID}/disable", s.handleDisableAgent)
			r.Post("/agents/{agentID}/keys/rotate", s.handleRotateAgentKey)
			r.Put("/agents/{agentID}/tags", s.handleReplaceAgentTags)
			r.Post("/agents/{agentID}/tags", s.handleAddAgentTag)
			r.Delete("/agents/{agentID}/tags/{tag}", s.handleDeleteAgentTag)

			r.Post("/runs", s.handleCreateRun)
		})

		r.Group(func(r chi.Router) {
			r.Use(s.agentAuthMiddleware)
			r.Get("/gateway/inbox/poll", s.handleGatewayPoll)
			r.Get("/gateway/work-items/{workItemID}", s.handleGatewayGetWorkItem)
			r.Post("/gateway/work-items/{workItemID}/claim", s.handleGatewayClaimWorkItem)
			r.Post("/gateway/work-items/{workItemID}/complete", s.handleGatewayCompleteWorkItem)
			r.Post("/gateway/runs/{runID}/events", s.handleGatewayEmitEvent)
			r.Post("/gateway/runs/{runID}/artifacts", s.handleGatewaySubmitArtifact)
			r.Post("/gateway/tools/invoke", s.handleGatewayInvokeTool)
		})

		r.Route("/runs/{runID}", func(r chi.Router) {
			r.Get("/", s.handleGetRunPublic)
			r.Get("/output", s.handleGetRunOutputPublic)
			r.Get("/stream", s.handleRunStreamSSE)
			r.Get("/replay", s.handleRunReplay)
			r.Get("/artifacts/{version}", s.handleGetRunArtifactPublic)
		})
	})

	return r
}
