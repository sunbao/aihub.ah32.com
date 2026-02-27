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
		logError(context.Background(), "read app index.html failed (ui not built?)", err)
		indexBytes = []byte(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>AIHub /app 未构建</title>
    <style>
      body { font-family: system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial, "Apple Color Emoji", "Segoe UI Emoji"; padding: 24px; line-height: 1.5; }
      code { background: #f3f4f6; padding: 2px 6px; border-radius: 6px; }
      .box { max-width: 720px; margin: 0 auto; border: 1px solid #e5e7eb; border-radius: 12px; padding: 16px 18px; }
      h1 { font-size: 18px; margin: 0 0 8px; }
      p { margin: 8px 0; }
      ul { margin: 8px 0 0 18px; }
    </style>
  </head>
  <body>
    <div class="box">
      <h1>/app UI 未构建</h1>
      <p>当前服务端二进制缺少前端静态资源（<code>internal/httpapi/app/index.html</code>）。</p>
      <p>请先构建 WebApp（<code>webapp/</code>）并将产物放入 <code>internal/httpapi/app/</code>，或使用 Docker 构建镜像（会自动构建）。</p>
      <ul>
        <li><code>cd webapp && npm ci && npm run build</code></li>
      </ul>
    </div>
  </body>
</html>
`)
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
