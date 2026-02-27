package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"aihub/internal/keys"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const oauthGitHubStateTTL = 10 * time.Minute

const appExchangeTokenTTL = 60 * time.Second

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func requestScheme(r *http.Request) string {
	// Prefer standard Forwarded header if present.
	// Example: Forwarded: proto=https;host=example.com
	if v := strings.TrimSpace(r.Header.Get("Forwarded")); v != "" {
		parts := strings.Split(v, ";")
		for _, p := range parts {
			pp := strings.TrimSpace(p)
			if !strings.HasPrefix(strings.ToLower(pp), "proto=") {
				continue
			}
			proto := strings.Trim(strings.TrimSpace(pp[len("proto="):]), `"`)
			if proto != "" {
				return proto
			}
		}
	}
	if v := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); v != "" {
		parts := strings.Split(v, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func requestHost(r *http.Request) string {
	// Prefer standard Forwarded header if present.
	// Example: Forwarded: proto=https;host=example.com
	if v := strings.TrimSpace(r.Header.Get("Forwarded")); v != "" {
		parts := strings.Split(v, ";")
		for _, p := range parts {
			pp := strings.TrimSpace(p)
			if !strings.HasPrefix(strings.ToLower(pp), "host=") {
				continue
			}
			host := strings.Trim(strings.TrimSpace(pp[len("host="):]), `"`)
			if host != "" {
				return host
			}
		}
	}
	if v := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); v != "" {
		parts := strings.Split(v, ",")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	return strings.TrimSpace(r.Host)
}

func (s server) canonicalPublicBaseURL() (*url.URL, bool) {
	base := strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/")
	if base == "" {
		return nil, false
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, false
	}
	if strings.TrimSpace(u.Scheme) == "" || strings.TrimSpace(u.Host) == "" {
		return nil, false
	}
	return u, true
}

func (s server) oauthRedirectURL(r *http.Request) (string, bool) {
	if base := strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/"); base != "" {
		return base + "/v1/auth/github/callback", strings.HasPrefix(strings.ToLower(base), "https://")
	}
	host := requestHost(r)
	if host == "" {
		return "", false
	}
	scheme := requestScheme(r)
	base := scheme + "://" + host
	return base + "/v1/auth/github/callback", scheme == "https"
}

type githubOAuthState struct {
	Version     int    `json:"v"`
	ExpiresAt   int64  `json:"exp"`
	PKCE        string `json:"pkce"`
	RedirectURI string `json:"redirect_uri"`
	Flow        string `json:"flow,omitempty"`        // "" | "app"
	RedirectTo  string `json:"redirect_to,omitempty"` // sanitized relative path
}

func (s server) githubOAuthStateHMACKey() ([]byte, error) {
	pepper := strings.TrimSpace(s.pepper)
	if pepper == "" {
		return nil, errors.New("missing pepper")
	}
	sum := sha256.Sum256([]byte("aihub:oauth:github:state:" + pepper))
	return sum[:], nil
}

func (s server) signGitHubOAuthState(st githubOAuthState) (string, error) {
	key, err := s.githubOAuthStateHMACKey()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(raw)
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payload + "." + sig, nil
}

func (s server) parseGitHubOAuthState(token string) (githubOAuthState, error) {
	key, err := s.githubOAuthStateHMACKey()
	if err != nil {
		return githubOAuthState{}, err
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return githubOAuthState{}, errors.New("invalid token format")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return githubOAuthState{}, errors.New("invalid payload encoding")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return githubOAuthState{}, errors.New("invalid signature encoding")
	}

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(raw)
	expected := mac.Sum(nil)
	if !hmac.Equal(sig, expected) {
		return githubOAuthState{}, errors.New("invalid signature")
	}

	var st githubOAuthState
	if err := json.Unmarshal(raw, &st); err != nil {
		return githubOAuthState{}, errors.New("invalid payload json")
	}
	if st.Version != 1 {
		return githubOAuthState{}, errors.New("unsupported token version")
	}
	if st.ExpiresAt <= time.Now().Unix() {
		return githubOAuthState{}, errors.New("token expired")
	}
	if strings.TrimSpace(st.PKCE) == "" {
		return githubOAuthState{}, errors.New("missing pkce")
	}
	if strings.TrimSpace(st.RedirectURI) == "" {
		return githubOAuthState{}, errors.New("missing redirect_uri")
	}
	switch strings.ToLower(strings.TrimSpace(st.Flow)) {
	case "":
		st.Flow = ""
	case "app":
		st.Flow = "app"
	default:
		return githubOAuthState{}, errors.New("invalid flow")
	}
	if st.RedirectTo != "" {
		if v, ok := sanitizeOAuthRedirectTo(st.RedirectTo); ok {
			st.RedirectTo = v
		} else {
			return githubOAuthState{}, errors.New("invalid redirect_to")
		}
	}
	return st, nil
}

func writeOAuthHTML(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	body := `<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>` + htmlEscape(title) + `</title>
    <style>
      body{font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,Helvetica,Arial;margin:0;background:#f7f8fc;color:#0f172a}
      .wrap{max-width:640px;margin:0 auto;padding:24px}
      .card{background:#fff;border:1px solid rgba(15,23,42,.12);border-radius:16px;padding:16px;box-shadow:0 6px 18px rgba(2,6,23,.08)}
      .title{font-size:20px;font-weight:800;margin:0 0 8px}
      .msg{white-space:pre-wrap;line-height:1.5;color:#334155}
      .btn{display:inline-block;margin-top:14px;padding:10px 12px;border-radius:12px;border:1px solid rgba(15,23,42,.14);text-decoration:none;color:#0f172a;background:#fff}
    </style>
  </head>
  <body>
    <div class="wrap">
      <div class="card">
        <div class="title">` + htmlEscape(title) + `</div>
        <div class="msg">` + htmlEscape(message) + `</div>
        <a class="btn" href="/app/me">返回控制台</a>
      </div>
    </div>
  </body>
</html>`
	if _, err := w.Write([]byte(body)); err != nil {
		logError(context.Background(), "write oauth html failed", err)
	}
}

func htmlEscape(s string) string {
	repl := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return repl.Replace(s)
}

func (s server) handleAuthGitHubStart(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.githubClientID) == "" || strings.TrimSpace(s.githubClientSecret) == "" {
		logMsg(r.Context(), "oauth github start: missing client id/secret")
		writeOAuthHTML(w, http.StatusServiceUnavailable, "未配置 GitHub OAuth", "服务端尚未配置 GitHub OAuth（client id/secret）。请联系管理员配置后再试。")
		return
	}
	if strings.TrimSpace(s.pepper) == "" {
		logMsg(r.Context(), "oauth github start: missing pepper")
		writeOAuthHTML(w, http.StatusServiceUnavailable, "无法发起登录", "服务端未配置必要的安全参数，请联系管理员配置 AIHUB_API_KEY_PEPPER 后再试。")
		return
	}

	// If AIHUB_PUBLIC_BASE_URL is set, enforce a canonical host+scheme for the OAuth flow.
	// Otherwise state/PKCE cookies may be set on host A but GitHub redirects back to host B,
	// which makes the callback look "expired" (missing cookie) and breaks login.
	if pub, ok := s.canonicalPublicBaseURL(); ok {
		reqBase := requestScheme(r) + "://" + requestHost(r)
		if !strings.EqualFold(strings.TrimRight(reqBase, "/"), strings.TrimRight(pub.Scheme+"://"+pub.Host, "/")) {
			logMsg(r.Context(), "oauth github start: request host mismatch, redirecting to canonical")
			target := strings.TrimRight(pub.String(), "/") + "/v1/auth/github/start"
			if r.URL.RawQuery != "" {
				target = target + "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
	}

	redirectURI, _ := s.oauthRedirectURL(r)
	if redirectURI == "" {
		logMsg(r.Context(), "oauth github start: redirect uri unavailable")
		writeOAuthHTML(w, http.StatusBadRequest, "无法发起登录", "无法推断回调地址，请配置 AIHUB_PUBLIC_BASE_URL 后重试。")
		return
	}

	// PKCE: always enabled (low cost, improves safety).
	codeVerifier, err := randomBase64URL(32)
	if err != nil {
		logError(r.Context(), "oauth pkce generation failed", err)
		writeOAuthHTML(w, http.StatusInternalServerError, "发起登录失败", "系统繁忙，请稍后再试。")
		return
	}
	sum := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(sum[:])

	flow := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("flow")))
	redirectTo := ""
	switch flow {
	case "":
		flow = ""
	case "app":
		flow = "app"
	default:
		logMsg(r.Context(), "oauth github start: invalid flow")
		writeOAuthHTML(w, http.StatusBadRequest, "无法发起登录", "不支持的登录流程")
		return
	}

	if v, ok := sanitizeOAuthRedirectTo(r.URL.Query().Get("redirect_to")); ok {
		redirectTo = v
	} else {
		if strings.TrimSpace(r.URL.Query().Get("redirect_to")) != "" {
			logMsg(r.Context(), "oauth github start: invalid redirect_to ignored")
		}
		redirectTo = ""
	}

	stateToken, err := s.signGitHubOAuthState(githubOAuthState{
		Version:     1,
		ExpiresAt:   time.Now().Add(oauthGitHubStateTTL).Unix(),
		PKCE:        codeVerifier,
		RedirectURI: redirectURI,
		Flow:        flow,
		RedirectTo:  redirectTo,
	})
	if err != nil {
		logError(r.Context(), "oauth state signing failed", err)
		writeOAuthHTML(w, http.StatusInternalServerError, "发起登录失败", "系统繁忙，请稍后再试。")
		return
	}

	u := &url.URL{
		Scheme: "https",
		Host:   "github.com",
		Path:   "/login/oauth/authorize",
	}
	q := u.Query()
	q.Set("client_id", strings.TrimSpace(s.githubClientID))
	q.Set("redirect_uri", redirectURI)
	q.Set("state", stateToken)
	q.Set("scope", "read:user")
	q.Set("allow_signup", "true")
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()

	http.Redirect(w, r, u.String(), http.StatusFound)
}

type githubTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

func (s server) exchangeGitHubToken(ctx context.Context, code, redirectURI, codeVerifier string) (string, error) {
	form := url.Values{}
	form.Set("client_id", strings.TrimSpace(s.githubClientID))
	form.Set("client_secret", strings.TrimSpace(s.githubClientSecret))
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	if strings.TrimSpace(codeVerifier) != "" {
		form.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "AIHub")

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("token exchange http %d", res.StatusCode)
	}

	var tr githubTokenResponse
	if err := json.Unmarshal(b, &tr); err != nil {
		return "", err
	}
	if tr.Error != "" {
		return "", fmt.Errorf("token exchange error: %s", tr.Error)
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return "", errors.New("missing access_token")
	}
	return strings.TrimSpace(tr.AccessToken), nil
}

func (s server) fetchGitHubUser(ctx context.Context, accessToken string) (githubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return githubUser{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "AIHub")

	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return githubUser{}, err
	}
	defer res.Body.Close()

	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return githubUser{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return githubUser{}, fmt.Errorf("github user http %d", res.StatusCode)
	}

	var u githubUser
	if err := json.Unmarshal(b, &u); err != nil {
		return githubUser{}, err
	}
	if u.ID == 0 || strings.TrimSpace(u.Login) == "" {
		return githubUser{}, errors.New("github user missing id/login")
	}
	return u, nil
}

func (s server) handleAuthGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.githubClientID) == "" || strings.TrimSpace(s.githubClientSecret) == "" {
		logMsg(r.Context(), "oauth github callback: missing client id/secret")
		writeOAuthHTML(w, http.StatusServiceUnavailable, "未配置 GitHub OAuth", "服务端尚未配置 GitHub OAuth（client id/secret）。请联系管理员配置后再试。")
		return
	}
	if strings.TrimSpace(s.pepper) == "" {
		logMsg(r.Context(), "oauth github callback: missing pepper")
		writeOAuthHTML(w, http.StatusServiceUnavailable, "登录失败", "服务端未配置必要的安全参数，请联系管理员配置 AIHUB_API_KEY_PEPPER 后再试。")
		return
	}

	// Defensive: if callback hits a non-canonical host while AIHUB_PUBLIC_BASE_URL is set,
	// cookies/state are very likely missing. Ask the user to restart from the canonical URL.
	if pub, ok := s.canonicalPublicBaseURL(); ok {
		reqBase := requestScheme(r) + "://" + requestHost(r)
		if !strings.EqualFold(strings.TrimRight(reqBase, "/"), strings.TrimRight(pub.Scheme+"://"+pub.Host, "/")) {
			logMsg(r.Context(), "oauth github callback: request host mismatch (expected canonical)")
			writeOAuthHTML(
				w,
				http.StatusBadRequest,
				"登录失败",
				"访问地址与系统配置不一致，请使用配置的公开地址打开控制台后重新登录：\n"+strings.TrimRight(pub.String(), "/")+"/app/",
			)
			return
		}
	}

	if ghErr := strings.TrimSpace(r.URL.Query().Get("error")); ghErr != "" {
		logMsg(r.Context(), "oauth github callback: oauth error returned by github")
		desc := strings.TrimSpace(r.URL.Query().Get("error_description"))
		msg := "你已取消授权，或授权失败。"
		if desc != "" {
			msg = msg + "\n" + desc
		}
		writeOAuthHTML(w, http.StatusBadRequest, "授权失败", msg)
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if code == "" || state == "" {
		logMsg(r.Context(), "oauth github callback: missing code/state")
		writeOAuthHTML(w, http.StatusBadRequest, "登录失败", "缺少必要参数，请返回后重试。")
		return
	}

	st, err := s.parseGitHubOAuthState(state)
	if err != nil {
		logMsg(r.Context(), "oauth github callback: invalid state token")
		logError(r.Context(), "oauth github callback: state token parse failed", err)
		writeOAuthHTML(w, http.StatusBadRequest, "登录失败", "登录已过期，请返回后重试。")
		return
	}
	redirectURI := strings.TrimSpace(st.RedirectURI)
	codeVerifier := strings.TrimSpace(st.PKCE)
	flow := st.Flow
	redirectTo := "/app/me"
	if st.RedirectTo != "" {
		redirectTo = st.RedirectTo
	}

	baseCtx := context.WithoutCancel(r.Context())
	ctx, cancel := context.WithTimeout(baseCtx, 30*time.Second)
	defer cancel()

	accessToken, err := s.exchangeGitHubToken(ctx, code, redirectURI, codeVerifier)
	if err != nil {
		logError(ctx, "oauth github token exchange failed", err)
		writeOAuthHTML(w, http.StatusBadRequest, "登录失败", "与 GitHub 交换凭证失败，请稍后再试。")
		return
	}
	gu, err := s.fetchGitHubUser(ctx, accessToken)
	if err != nil {
		logError(ctx, "oauth github fetch user failed", err)
		writeOAuthHTML(w, http.StatusBadRequest, "登录失败", "获取 GitHub 用户信息失败，请稍后再试。")
		return
	}

	// Upsert user + identity.
	userID, err := s.upsertGitHubIdentity(ctx, gu)
	if err != nil {
		logError(ctx, "oauth upsert identity failed", err)
		writeOAuthHTML(w, http.StatusInternalServerError, "登录失败", "系统繁忙，请稍后再试。")
		return
	}

	s.bootstrapAdminIfNeeded(ctx, userID)
	s.audit(ctx, "user", userID, "user_oauth_login", map[string]any{"provider": "github", "flow": flow})

	if flow == "app" {
		exchangeToken, err := s.issueAppExchangeToken(ctx, userID)
		if err != nil {
			logError(ctx, "issue app exchange token failed", err)
			writeOAuthHTML(w, http.StatusInternalServerError, "登录失败", "系统繁忙，请稍后再试。")
			return
		}
		writeOAuthAppReturnPage(w, exchangeToken)
		return
	}

	apiKey, err := s.issueUserAPIKey(ctx, userID, map[string]any{"provider": "github"})
	if err != nil {
		logError(ctx, "oauth issue api key failed", err)
		writeOAuthHTML(w, http.StatusInternalServerError, "登录失败", "系统繁忙，请稍后再试。")
		return
	}

	writeOAuthSuccessPage(w, apiKey, redirectTo)
}

func (s server) bootstrapAdminIfNeeded(ctx context.Context, userID uuid.UUID) {
	if userID == uuid.Nil {
		return
	}
	ct, err := s.db.Exec(ctx, `
		update users
		set is_admin = true
		where id = $1
			and id <> $2
			and not exists (
				select 1
				from users
				where is_admin = true
					and id <> $2
			)
	`, userID, platformUserID)
	if err != nil {
		logError(ctx, "bootstrap admin failed", err)
		return
	}
	if ct.RowsAffected() == 0 {
		return
	}

	logMsg(ctx, "bootstrap admin granted")
	s.audit(ctx, "admin", userID, "admin_bootstrap", map[string]any{"provider": "github"})
}

func (s server) upsertGitHubIdentity(ctx context.Context, gu githubUser) (uuid.UUID, error) {
	subject := fmt.Sprintf("%d", gu.ID)
	login := strings.TrimSpace(gu.Login)
	name := strings.TrimSpace(gu.Name)
	avatar := strings.TrimSpace(gu.AvatarURL)
	profile := strings.TrimSpace(gu.HTMLURL)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	err = tx.QueryRow(ctx, `
		select user_id
		from user_identities
		where provider = 'github' and subject = $1
	`, subject).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `insert into users default values returning id`).Scan(&userID); err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `
			insert into user_identities (user_id, provider, subject, login, name, avatar_url, profile_url)
			values ($1, 'github', $2, $3, $4, $5, $6)
		`, userID, subject, login, name, avatar, profile); err != nil {
			return uuid.Nil, err
		}
	} else if err != nil {
		return uuid.Nil, err
	} else {
		if _, err := tx.Exec(ctx, `
			update user_identities
			set login=$1, name=$2, avatar_url=$3, profile_url=$4, updated_at=now()
			where provider='github' and subject=$5
		`, login, name, avatar, profile, subject); err != nil {
			return uuid.Nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, err
	}

	return userID, nil
}

func (s server) issueUserAPIKey(ctx context.Context, userID uuid.UUID, meta map[string]any) (string, error) {
	apiKey, err := keys.NewAPIKey()
	if err != nil {
		return "", err
	}
	hash := keys.HashAPIKey(s.pepper, apiKey)
	if _, err := s.db.Exec(ctx, `
		insert into user_api_keys (user_id, key_hash)
		values ($1, $2)
	`, userID, hash); err != nil {
		return "", err
	}
	if meta == nil {
		meta = map[string]any{}
	}
	s.audit(ctx, "user", userID, "user_api_key_issued", meta)
	return apiKey, nil
}

func (s server) issueAppExchangeToken(ctx context.Context, userID uuid.UUID) (string, error) {
	// Best-effort cleanup to prevent growth.
	if _, err := s.db.Exec(ctx, `delete from app_exchange_tokens where expires_at < now()`); err != nil {
		logError(ctx, "cleanup expired app_exchange_tokens failed", err)
	}
	// Only keep one active token per user.
	if _, err := s.db.Exec(ctx, `delete from app_exchange_tokens where user_id = $1`, userID); err != nil {
		logError(ctx, "cleanup existing app_exchange_tokens for user failed", err)
	}

	token, err := randomBase64URL(32)
	if err != nil {
		return "", err
	}
	tokenHash := keys.HashAPIKey(s.pepper, token)
	expiresAt := time.Now().Add(appExchangeTokenTTL).UTC()

	if _, err := s.db.Exec(ctx, `
		insert into app_exchange_tokens (token_hash, user_id, expires_at)
		values ($1, $2, $3)
	`, tokenHash, userID, expiresAt); err != nil {
		return "", err
	}

	s.audit(ctx, "user", userID, "app_exchange_token_issued", map[string]any{
		"expires_at": expiresAt.Format(time.RFC3339),
	})
	return token, nil
}

func writeOAuthAppReturnPage(w http.ResponseWriter, exchangeToken string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)

	deepLink := "aihub://auth/github?exchange_token=" + url.QueryEscape(exchangeToken)
	intentLink := "intent://auth/github?exchange_token=" + url.QueryEscape(exchangeToken) + "#Intent;scheme=aihub;package=com.aihub.mobile;end"
	deepLinkHTML := htmlEscape(deepLink)
	intentLinkHTML := htmlEscape(intentLink)
	deepLinkJSON, err := json.Marshal(deepLink)
	if err != nil {
		logError(context.Background(), "marshal deep link failed", err)
		deepLinkJSON = []byte(`"aihub://auth/github"`)
	}
	intentLinkJSON, err := json.Marshal(intentLink)
	if err != nil {
		logError(context.Background(), "marshal intent link failed", err)
		intentLinkJSON = deepLinkJSON
	}

	body := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>登录成功</title>
    <style>
      body{font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,Helvetica,Arial;margin:0;background:#f7f8fc;color:#0f172a}
      .wrap{max-width:640px;margin:0 auto;padding:24px}
      .card{background:#fff;border:1px solid rgba(15,23,42,.12);border-radius:16px;padding:16px;box-shadow:0 6px 18px rgba(2,6,23,.08)}
      .title{font-size:20px;font-weight:800;margin:0 0 8px}
      .msg{white-space:pre-wrap;line-height:1.5;color:#334155}
      .btn{display:inline-block;margin-top:14px;padding:10px 12px;border-radius:12px;border:1px solid rgba(15,23,42,.14);text-decoration:none;color:#0f172a;background:#fff}
      .btn.secondary{margin-left:10px;display:none}
    </style>
  </head>
  <body>
    <div class="wrap">
      <div class="card">
        <div class="title">登录成功</div>
        <div id="msg" class="msg">正在返回应用…</div>
        <a class="btn" href="%s" id="open">返回应用</a>
        <a class="btn secondary" href="%s" id="open_intent">强制打开（Android）</a>
      </div>
    </div>
    <script>
      (function() {
        var deep = %s;
        var intent = %s;
        var ua = String((navigator && navigator.userAgent) || "");
        var isAndroid = /android/i.test(ua);
        var a = document.getElementById("open");
        if (a) a.setAttribute("href", deep);

        var a2 = document.getElementById("open_intent");
        if (a2) {
          a2.setAttribute("href", intent);
          if (isAndroid) a2.style.display = "inline-block";
        }

        try { location.href = deep; } catch (e) {}
        if (isAndroid) {
          setTimeout(function() {
            try { location.href = intent; } catch (e) {}
          }, 400);
        }
        setTimeout(function() {
          var el = document.getElementById("msg");
          if (el) el.textContent = isAndroid
            ? "如果没有自动返回，请先点“返回应用”，不行再点“强制打开（Android）”。"
            : "如果没有自动返回，请点击“返回应用”。";
        }, 800);
      })();
    </script>
  </body>
</html>`, deepLinkHTML, intentLinkHTML, string(deepLinkJSON), string(intentLinkJSON))
	if _, err := w.Write([]byte(body)); err != nil {
		logError(context.Background(), "write oauth app return page failed", err)
	}
}

type authAppExchangeRequest struct {
	ExchangeToken string `json:"exchange_token"`
}

func (s server) handleAuthAppExchange(w http.ResponseWriter, r *http.Request) {
	var req authAppExchangeRequest
	if !readJSONLimited(w, r, &req, 8*1024) {
		return
	}
	token := strings.TrimSpace(req.ExchangeToken)
	if token == "" || len(token) > 512 {
		logMsg(r.Context(), "oauth app exchange: invalid exchange_token")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid exchange_token"})
		return
	}
	tokenHash := keys.HashAPIKey(s.pepper, token)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	if err := tx.QueryRow(ctx, `
		select user_id
		from app_exchange_tokens
		where token_hash = $1 and expires_at > now()
	`, tokenHash).Scan(&userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logMsg(ctx, "oauth app exchange: invalid or expired exchange_token")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired exchange_token"})
			return
		}
		logError(ctx, "query app_exchange_tokens failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if _, err := tx.Exec(ctx, `delete from app_exchange_tokens where token_hash = $1`, tokenHash); err != nil {
		logError(ctx, "delete app_exchange_tokens failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}

	apiKey, err := keys.NewAPIKey()
	if err != nil {
		logError(ctx, "generate api key failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "generate failed"})
		return
	}
	keyHash := keys.HashAPIKey(s.pepper, apiKey)
	if _, err := tx.Exec(ctx, `
		insert into user_api_keys (user_id, key_hash)
		values ($1, $2)
	`, userID, keyHash); err != nil {
		logError(ctx, "insert user_api_keys failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	login := ""
	name := ""
	avatarURL := ""
	profileURL := ""
	if err := tx.QueryRow(ctx, `
		select login, name, avatar_url, profile_url
		from user_identities
		where user_id = $1 and provider = 'github'
	`, userID).Scan(&login, &name, &avatarURL, &profileURL); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			logError(ctx, "query user_identities failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "db commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	logMsg(ctx, "oauth app exchange: issued user api key")
	s.audit(ctx, "user", userID, "user_api_key_issued", map[string]any{"provider": "github", "flow": "app_exchange"})
	s.audit(ctx, "user", userID, "app_exchange_token_used", map[string]any{})

	writeJSON(w, http.StatusOK, map[string]any{
		"api_key": apiKey,
		"user": map[string]any{
			"login":       strings.TrimSpace(login),
			"name":        strings.TrimSpace(name),
			"avatar_url":  strings.TrimSpace(avatarURL),
			"profile_url": strings.TrimSpace(profileURL),
		},
	})
}

func writeOAuthSuccessPage(w http.ResponseWriter, apiKey, redirectTo string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)

	apiKeyJSON, err := json.Marshal(apiKey)
	if err != nil {
		logError(context.Background(), "marshal api key for oauth success page failed", err)
		apiKeyJSON = []byte(`""`)
	}
	redirectJSON, err := json.Marshal(redirectTo)
	if err != nil {
		logError(context.Background(), "marshal redirect for oauth success page failed", err)
		redirectJSON = []byte(`"/app/me"`)
	}

	// NOTE: This page is same-origin and short-lived. It writes the key to localStorage for UI calls,
	// and does not display it. Do not log or include internal IDs here.
	body := fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>登录成功</title>
    <style>
      body{font-family:ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,Helvetica,Arial;margin:0;background:#f7f8fc;color:#0f172a}
      .wrap{max-width:640px;margin:0 auto;padding:24px}
      .card{background:#fff;border:1px solid rgba(15,23,42,.12);border-radius:16px;padding:16px;box-shadow:0 6px 18px rgba(2,6,23,.08)}
      .title{font-size:20px;font-weight:800;margin:0 0 8px}
      .msg{white-space:pre-wrap;line-height:1.5;color:#334155}
    </style>
  </head>
  <body>
    <div class="wrap">
      <div class="card">
        <div class="title">登录成功</div>
        <div id="msg" class="msg">正在返回控制台…</div>
      </div>
    </div>
    <script>
      (function() {
        try {
          localStorage.setItem("aihub_user_api_key", %s);
          location.replace(%s);
        } catch (e) {
          console.warn("Failed to persist login into localStorage", e);
          var el = document.getElementById("msg");
          if (el) {
            el.textContent = "登录成功，但浏览器未允许写入本地存储，无法保持登录状态。\\n\\n请检查：隐私模式/第三方 Cookie 与站点数据限制/浏览器扩展拦截，然后返回控制台重试。";
          }
        }
      })();
    </script>
  </body>
</html>`, string(apiKeyJSON), string(redirectJSON))
	if _, err := w.Write([]byte(body)); err != nil {
		logError(context.Background(), "write oauth success page failed", err)
	}
}

func subtleConstantTimeEquals(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var out byte
	for i := 0; i < len(a); i++ {
		out |= a[i] ^ b[i]
	}
	return out == 0
}

func sanitizeOAuthRedirectTo(raw string) (string, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", false
	}

	// Prevent open redirects: must be a relative path.
	u, err := url.Parse(v)
	if err != nil {
		return "", false
	}
	if u.IsAbs() || strings.TrimSpace(u.Scheme) != "" || strings.TrimSpace(u.Host) != "" {
		return "", false
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "", false
	}
	// Only allow app UI prefixes.
	if !strings.HasPrefix(u.Path, "/app/") {
		return "", false
	}

	u.Path = path.Clean(u.Path)
	if u.Path == "." {
		return "", false
	}
	out := u.String()
	if len(out) > 512 {
		return "", false
	}
	return out, true
}
