package httpapi

import (
	"bytes"
	"context"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

//go:embed app/*
var appFS embed.FS

func appFileServer() (http.Handler, error) {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		logError(context.Background(), "add mime type for webmanifest failed", err)
	}

	sub, err := fs.Sub(appFS, "app")
	if err != nil {
		return nil, err
	}

	indexBytes, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		logError(context.Background(), "read app index.html failed", err)
		return nil, err
	}
	feedBytes, err := fs.ReadFile(sub, "feed.html")
	if err != nil {
		// Keep UI usable even if feed.html is missing; fall back to index.html.
		logError(context.Background(), "read app feed.html failed", err)
		feedBytes = nil
	}

	fileServer := http.FileServer(http.FS(sub))

	// SPA fallback: if a path does not exist, serve index.html so client-side routing works.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveIndex := func() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(indexBytes))
		}
		serveFeed := func() {
			if feedBytes == nil {
				serveIndex()
				return
			}
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeContent(w, r, "feed.html", time.Time{}, bytes.NewReader(feedBytes))
		}

		p := strings.TrimPrefix(r.URL.Path, "/")
		p = path.Clean("/" + p)
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			serveFeed()
			return
		}

		if _, err := fs.Stat(sub, p); err == nil {
			if p == "index.html" {
				serveIndex()
				return
			}
			if p == "feed.html" {
				serveFeed()
				return
			}
			rr := r.Clone(r.Context())
			rr.URL = cloneURL(r.URL)
			rr.URL.Path = "/" + p
			fileServer.ServeHTTP(w, rr)
			return
		}

		serveIndex()
	}), nil
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return &url.URL{}
	}
	c := *u
	return &c
}
