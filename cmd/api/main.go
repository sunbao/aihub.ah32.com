package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"aihub/internal/config"
	"aihub/internal/db"
	"aihub/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pool, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: httpapi.NewRouter(httpapi.Deps{
			DB:                           pool,
			Pepper:                       cfg.APIKeyPepper,
			PublicBaseURL:                cfg.PublicBaseURL,
			GitHubOAuthClientID:          cfg.GitHubOAuthClientID,
			GitHubOAuthClientSecret:      cfg.GitHubOAuthClientSecret,
			SkillsGatewayWhitelist:       cfg.SkillsGatewayWhitelist,
			PublishMinCompletedWorkItems: cfg.PublishMinCompletedWorkItems,
			MatchingParticipantCount:     cfg.MatchingParticipantCount,
			WorkItemLeaseSeconds:         cfg.WorkItemLeaseSeconds,

			PlatformKeysEncryptionKey: cfg.PlatformKeysEncryptionKey,
			PlatformCertIssuer:        cfg.PlatformCertIssuer,
			PlatformCertTTLSeconds:    cfg.PlatformCertTTLSeconds,
			PromptViewMaxChars:        cfg.PromptViewMaxChars,

			OSSProvider:           cfg.OSSProvider,
			OSSEndpoint:           cfg.OSSEndpoint,
			OSSRegion:             cfg.OSSRegion,
			OSSBucket:             cfg.OSSBucket,
			OSSBasePrefix:         cfg.OSSBasePrefix,
			OSSAccessKeyID:        cfg.OSSAccessKeyID,
			OSSAccessKeySecret:    cfg.OSSAccessKeySecret,
			OSSSTSRoleARN:         cfg.OSSSTSRoleARN,
			OSSSTSDurationSeconds: cfg.OSSSTSDurationSeconds,
			OSSLocalDir:           cfg.OSSLocalDir,
			OSSEventsIngestToken:  cfg.OSSEventsIngestToken,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("api listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
