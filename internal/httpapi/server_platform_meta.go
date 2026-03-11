package httpapi

import (
	"net/http"
	"strings"
)

type platformMetaPublicDTO struct {
	AppDownloadURL string `json:"app_download_url,omitempty"`
}

func (s server) handleGetPlatformMetaPublic(w http.ResponseWriter, r *http.Request) {
	out := platformMetaPublicDTO{
		AppDownloadURL: strings.TrimSpace(s.appDownloadURL),
	}
	writeJSON(w, http.StatusOK, out)
}
