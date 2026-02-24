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

	fileServer := http.FileServer(http.FS(sub))

	// SPA fallback: if a path does not exist, serve index.html so client-side routing works.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveIndex := func() {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(indexBytes))
		}

		p := strings.TrimPrefix(r.URL.Path, "/")
		p = path.Clean("/" + p)
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			serveIndex()
			return
		}

		if _, err := fs.Stat(sub, p); err == nil {
			if p == "index.html" {
				serveIndex()
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
