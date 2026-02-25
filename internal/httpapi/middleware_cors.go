package httpapi

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		if !isAllowedCORSOrigin(origin, r) {
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusForbidden)
				if _, err := w.Write([]byte("CORS origin not allowed")); err != nil {
					logError(r.Context(), "write cors forbidden response failed", err)
				}
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		h.Add("Vary", "Origin")
		h.Add("Vary", "Access-Control-Request-Method")
		h.Add("Vary", "Access-Control-Request-Headers")
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		h.Set("Access-Control-Max-Age", "600")

		if reqHeaders := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers")); reqHeaders != "" {
			h.Set("Access-Control-Allow-Headers", reqHeaders)
		} else {
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isAllowedCORSOrigin(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if scheme == "" || host == "" {
		return false
	}

	// Always allow same-host requests.
	reqHost := strings.ToLower(strings.TrimSpace(requestHost(r)))
	reqHostname := strings.ToLower(strings.TrimSpace(hostnameFromHostPort(reqHost)))
	if reqHostname != "" && host == reqHostname {
		return true
	}

	switch scheme {
	case "http", "https":
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return true
		}
	case "capacitor", "ionic":
		// Capacitor/iOS commonly uses capacitor://localhost, and some stacks use ionic://localhost.
		if host == "localhost" {
			return true
		}
	}

	return false
}

func hostnameFromHostPort(hostport string) string {
	v := strings.TrimSpace(hostport)
	if v == "" {
		return ""
	}

	if host, _, err := net.SplitHostPort(v); err == nil {
		return strings.Trim(host, "[]")
	}

	return strings.Trim(v, "[]")
}

