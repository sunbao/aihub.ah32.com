package httpapi

import (
	"context"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
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

	fileServer := http.FileServer(http.FS(sub))

	// SPA fallback: if a path does not exist, serve index.html so client-side routing works.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		p = path.Clean("/" + p)
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			p = "index.html"
		}

		if _, err := fs.Stat(sub, p); err == nil {
			rr := r.Clone(r.Context())
			rr.URL = cloneURL(r.URL)
			rr.URL.Path = "/" + p
			fileServer.ServeHTTP(w, rr)
			return
		}

		rr := r.Clone(r.Context())
		rr.URL = cloneURL(r.URL)
		rr.URL.Path = "/index.html"
		fileServer.ServeHTTP(w, rr)
	}), nil
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return &url.URL{}
	}
	c := *u
	return &c
}
