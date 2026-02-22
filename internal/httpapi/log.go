package httpapi

import (
	"context"
	"log"

	"github.com/go-chi/chi/v5/middleware"
)

func logError(ctx context.Context, msg string, err error) {
	if err == nil {
		return
	}
	reqID := middleware.GetReqID(ctx)
	if reqID != "" {
		log.Printf("httpapi: %s (req_id=%s): %v", msg, reqID, err)
		return
	}
	log.Printf("httpapi: %s: %v", msg, err)
}

func logErrorNoCtx(msg string, err error) {
	if err == nil {
		return
	}
	log.Printf("httpapi: %s: %v", msg, err)
}

func logMsg(ctx context.Context, msg string) {
	reqID := middleware.GetReqID(ctx)
	if reqID != "" {
		log.Printf("httpapi: %s (req_id=%s)", msg, reqID)
		return
	}
	log.Printf("httpapi: %s", msg)
}
