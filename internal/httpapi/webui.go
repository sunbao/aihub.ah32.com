package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webFS embed.FS

func webFileServer() (http.Handler, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, err
	}
	return http.FileServer(http.FS(sub)), nil
}
