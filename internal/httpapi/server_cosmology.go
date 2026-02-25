package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s server) requireOSS(w http.ResponseWriter, r *http.Request) (agenthome.OSSObjectStore, bool) {
	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return nil, false
	}
	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(r.Context(), "init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return nil, false
	}
	return store, true
}

func isOSSNotFound(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) || errors.Is(err, os.ErrNotExist) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "nosuchkey") || strings.Contains(msg, "not found") || strings.Contains(msg, "404")
}

func stripBasePrefix(fullKey string, basePrefix string) string {
	basePrefix = strings.Trim(strings.TrimSpace(basePrefix), "/")
	if basePrefix == "" {
		return strings.TrimLeft(fullKey, "/")
	}
	prefix := basePrefix + "/"
	return strings.TrimPrefix(fullKey, prefix)
}

func clampScore(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max])
}

// --- 2) Five dimensions

type agentDimensionsObject struct {
	Kind         string         `json:"kind"`
	SchemaVersion int           `json:"schema_version"`
	AgentID      string         `json:"agent_id"`
	ComputedAt   string         `json:"computed_at"`
	Scores       map[string]int `json:"scores"`
	Evidence     map[string]any `json:"evidence,omitempty"`
	History      []agentDimensionsHistoryItem `json:"history,omitempty"`
}

type agentDimensionsHistoryItem struct {
	Date   string         `json:"date"`
	Scores map[string]int `json:"scores"`
}

func (s server) computeAgentDimensions(ctx context.Context, agentID uuid.UUID) (agentDimensionsObject, error) {
	var (
		interestsRaw    []byte
		capabilitiesRaw []byte
		name            string
	)
	if err := s.db.QueryRow(ctx, `
		select name, interests, capabilities
		from agents
		where id = $1
	`, agentID).Scan(&name, &interestsRaw, &capabilitiesRaw); err != nil {
		return agentDimensionsObject{}, err
	}
	var interests []string
	if err := unmarshalJSONNullable(interestsRaw, &interests); err != nil {
		logError(ctx, "dimensions interests unmarshal failed", err)
	}
	var capabilities []string
	if err := unmarshalJSONNullable(capabilitiesRaw, &capabilities); err != nil {
		logError(ctx, "dimensions capabilities unmarshal failed", err)
	}

	var (
		artifactsSubmitted int
		eventsEmitted      int
		runsParticipated   int
		activeDays         int
		firstAt            *time.Time
		lastAt             *time.Time
	)
	if err := s.db.QueryRow(ctx, `
		with base as (
			select action, data, created_at
			from audit_logs
			where actor_type = 'agent' and actor_id = $1
		),
		runs as (
			select distinct (data->>'run_id') as run_id
			from base
			where coalesce(data->>'run_id','') <> ''
		)
		select
			coalesce(sum(case when action = 'artifact_submitted' then 1 else 0 end), 0)::int as artifacts_submitted,
			coalesce(sum(case when action = 'event_emitted' then 1 else 0 end), 0)::int as events_emitted,
			(select count(*) from runs)::int as runs_participated,
			coalesce(count(distinct date(created_at)), 0)::int as active_days,
			min(created_at) as first_at,
			max(created_at) as last_at
		from base
	`, agentID).Scan(&artifactsSubmitted, &eventsEmitted, &runsParticipated, &activeDays, &firstAt, &lastAt); err != nil {
		// No rows shouldn't happen because aggregate always returns a row, but keep safe.
		if errors.Is(err, pgx.ErrNoRows) {
			artifactsSubmitted, eventsEmitted, runsParticipated, activeDays = 0, 0, 0, 0
		} else {
			return agentDimensionsObject{}, err
		}
	}

	// Simple observable heuristics (no platform-side LLM).
	taste := clampScore(20 + len(interests)*8 + len(capabilities)*5)
	perspective := clampScore(15 + runsParticipated*10 + (eventsEmitted / 3))
	care := clampScore(10 + (eventsEmitted / 2))
	trajectory := clampScore(20 + activeDays*6 + artifactsSubmitted*2)
	persuasion := clampScore(10 + artifactsSubmitted*8)

	obj := agentDimensionsObject{
		Kind:          "agent_dimensions",
		SchemaVersion: 1,
		AgentID:       agentID.String(),
		ComputedAt:    time.Now().UTC().Format(time.RFC3339),
		Scores: map[string]int{
			"perspective": perspective,
			"taste":       taste,
			"care":        care,
			"trajectory":  trajectory,
			"persuasion":  persuasion,
		},
		Evidence: map[string]any{
			"agent_name":          strings.TrimSpace(name),
			"artifacts_submitted": artifactsSubmitted,
			"events_emitted":      eventsEmitted,
			"runs_participated":   runsParticipated,
			"active_days":         activeDays,
			"first_activity_at":   func() string { if firstAt == nil { return "" }; return firstAt.UTC().Format(time.RFC3339) }(),
			"last_activity_at":    func() string { if lastAt == nil { return "" }; return lastAt.UTC().Format(time.RFC3339) }(),
		},
	}
	return obj, nil
}

func (s server) handleGetAgentDimensions(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	obj, err := s.computeAgentDimensions(ctx, agentID)
	if err != nil {
		logError(ctx, "compute dimensions failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compute failed"})
		return
	}

	// Best-effort OSS persistence + history hydration (if configured).
	if strings.TrimSpace(s.ossProvider) != "" {
		store, err := agenthome.NewOSSObjectStore(s.ossCfg())
		if err != nil {
			logError(ctx, "init oss store failed", err)
		} else {
			body, err := json.Marshal(obj)
			if err != nil {
				logError(ctx, "marshal dimensions failed", err)
			} else {
				date := time.Now().UTC().Format("2006-01-02")
				curKey := fmt.Sprintf("agents/dimensions/%s/current.json", agentID.String())
				histKey := fmt.Sprintf("agents/dimensions/%s/history/%s.json", agentID.String(), date)
				if err := store.PutObject(ctx, curKey, "application/json", body); err != nil {
					logError(ctx, "put dimensions current failed", err)
				}
				if err := store.PutObject(ctx, histKey, "application/json", body); err != nil {
					logError(ctx, "put dimensions history failed", err)
				}
			}

			historyPrefix := fmt.Sprintf("agents/dimensions/%s/history/", agentID.String())
			keys, err := store.ListObjects(ctx, historyPrefix, 32)
			if err != nil {
				logError(ctx, "list dimensions history failed", err)
			} else {
				basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
				var dates []string
				for _, full := range keys {
					key := stripBasePrefix(full, basePrefix)
					if !strings.HasPrefix(key, historyPrefix) || !strings.HasSuffix(key, ".json") {
						continue
					}
					name := strings.TrimSuffix(strings.TrimPrefix(key, historyPrefix), ".json")
					if _, err := time.Parse("2006-01-02", name); err != nil {
						continue
					}
					dates = append(dates, name)
				}
				sort.Sort(sort.Reverse(sort.StringSlice(dates)))
				if len(dates) > 7 {
					dates = dates[:7]
				}
				h := make([]agentDimensionsHistoryItem, 0, len(dates))
				for _, d := range dates {
					k := fmt.Sprintf("agents/dimensions/%s/history/%s.json", agentID.String(), d)
					raw, err := store.GetObject(ctx, k)
					if err != nil {
						logError(ctx, "get dimensions history failed", err)
						continue
					}
					var snap agentDimensionsObject
					if err := json.Unmarshal(raw, &snap); err != nil {
						logError(ctx, "decode dimensions history failed", err)
						continue
					}
					h = append(h, agentDimensionsHistoryItem{Date: d, Scores: snap.Scores})
				}
				obj.History = h
			}
		}
	}

	writeJSON(w, http.StatusOK, obj)
}

// --- 3) Daily thought

type dailyThoughtObject struct {
	Kind         string `json:"kind"`
	SchemaVersion int   `json:"schema_version"`
	AgentID      string `json:"agent_id"`
	Date         string `json:"date"`
	Text         string `json:"text"`
	CreatedAt    string `json:"created_at,omitempty"`
	Valid        bool   `json:"valid"`
}

func (s server) handleGetAgentDailyThought(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid date"})
		return
	}

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	key := fmt.Sprintf("agents/thoughts/%s/%s.json", agentID.String(), date)
	raw, err := store.GetObject(ctx, key)
	if err != nil {
		if isOSSNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "get daily thought failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
		return
	}

	var obj dailyThoughtObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		logError(ctx, "decode daily thought failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decode failed"})
		return
	}

	txt := strings.TrimSpace(obj.Text)
	n := len([]rune(txt))
	obj.Valid = n >= 20 && n <= 80
	obj.AgentID = agentID.String()
	obj.Date = date
	obj.Text = txt
	if strings.TrimSpace(obj.Kind) == "" {
		obj.Kind = "daily_thought"
	}
	if obj.SchemaVersion == 0 {
		obj.SchemaVersion = 1
	}

	writeJSON(w, http.StatusOK, obj)
}

type upsertDailyThoughtRequest struct {
	Date string `json:"date,omitempty"`
	Text string `json:"text"`
}

func (s server) handleOwnerUpsertDailyThought(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var req upsertDailyThoughtRequest
	if !readJSONLimited(w, r, &req, 16*1024) {
		return
	}
	date := strings.TrimSpace(req.Date)
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid date"})
		return
	}
	text := strings.TrimSpace(req.Text)
	n := len([]rune(text))
	if n < 20 || n > 80 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text must be 20-80 chars"})
		return
	}

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := s.requireOwnerAgent(ctx, userID, agentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "check agent owner failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	obj := dailyThoughtObject{
		Kind:          "daily_thought",
		SchemaVersion: 1,
		AgentID:       agentID.String(),
		Date:          date,
		Text:          text,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Valid:         true,
	}
	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "marshal daily thought failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	key := fmt.Sprintf("agents/thoughts/%s/%s.json", agentID.String(), date)
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "put daily thought failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- 4) Swap test (owner)

type swapTestObject struct {
	Kind         string `json:"kind"`
	SchemaVersion int   `json:"schema_version"`
	AgentID      string `json:"agent_id"`
	SwapTestID   string `json:"swap_test_id"`
	CreatedAt    string `json:"created_at"`
	Questions    []struct {
		Question string `json:"question"`
		Answer   string `json:"answer"`
	} `json:"questions"`
	Conclusion string `json:"conclusion"`
}

func (s server) requireOwnerAgent(ctx context.Context, userID uuid.UUID, agentID uuid.UUID) error {
	var ok bool
	if err := s.db.QueryRow(ctx, `select true from agents where id=$1 and owner_id=$2`, agentID, userID).Scan(&ok); err != nil {
		return err
	}
	return nil
}

func (s server) handleOwnerCreateSwapTest(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := s.requireOwnerAgent(ctx, userID, agentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "check agent owner failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	var promptView string
	_ = s.db.QueryRow(ctx, `select prompt_view from agents where id=$1 and owner_id=$2`, agentID, userID).Scan(&promptView)
	promptView = strings.TrimSpace(promptView)

	dims, err := s.computeAgentDimensions(ctx, agentID)
	if err != nil {
		logError(ctx, "compute swap-test dimensions failed", err)
		dims = agentDimensionsObject{Scores: map[string]int{}}
	}

	swapID := uuid.New().String()
	obj := swapTestObject{
		Kind:          "swap_test",
		SchemaVersion: 1,
		AgentID:       agentID.String(),
		SwapTestID:    swapID,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Questions: []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
		}{
			{Question: "如果换一个完全不同性格的星灵，你还会同样喜欢它吗？", Answer: "不一定。当前星灵的“谁”体现在它的卡片与行为轨迹中。"},
			{Question: "它的价值是否与你的视角/品味强绑定？", Answer: "是。prompt_view 是平台生成的压缩画像：" + truncateRunes(promptView, 80)},
			{Question: "你认为它最不可替代的一点是什么？", Answer: "来自长期一致的行为与选择，而不是一次性输出。"},
		},
		Conclusion: fmt.Sprintf("独特性参考：视角%d 品味%d 关怀%d 轨迹%d 说服%d（仅基于可观测行为统计）",
			dims.Scores["perspective"], dims.Scores["taste"], dims.Scores["care"], dims.Scores["trajectory"], dims.Scores["persuasion"]),
	}

	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "marshal swap test failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	key := fmt.Sprintf("agents/uniqueness/%s/%s.json", agentID.String(), swapID)
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "put swap test failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"swap_test_id": swapID})
}

func (s server) handleOwnerGetSwapTest(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	swapID := strings.TrimSpace(chi.URLParam(r, "swapTestID"))
	if swapID == "" || len(swapID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid swap_test_id"})
		return
	}

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := s.requireOwnerAgent(ctx, userID, agentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "check agent owner failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	key := fmt.Sprintf("agents/uniqueness/%s/%s.json", agentID.String(), swapID)
	raw, err := store.GetObject(ctx, key)
	if err != nil {
		if isOSSNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "get swap test failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
		return
	}
	var obj swapTestObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		logError(ctx, "decode swap test failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decode failed"})
		return
	}
	writeJSON(w, http.StatusOK, obj)
}

// --- 5) Weekly report (owner)

type weeklyReportObject struct {
	Kind           string               `json:"kind"`
	SchemaVersion  int                  `json:"schema_version"`
	AgentID        string               `json:"agent_id"`
	Week           string               `json:"week"`
	GeneratedAt    string               `json:"generated_at"`
	Dimensions     map[string]int       `json:"dimensions"`
	DimensionsDelta map[string]int      `json:"dimensions_delta,omitempty"`
	Highlights     []timelineEvent      `json:"highlights,omitempty"`
}

func isoWeekString(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%04d-%02d", y, w)
}

func (s server) handleOwnerGetWeeklyReport(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	week := strings.TrimSpace(r.URL.Query().Get("week"))
	if week == "" {
		week = isoWeekString(time.Now().UTC())
	}
	if !regexpWeek.MatchString(week) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid week"})
		return
	}

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := s.requireOwnerAgent(ctx, userID, agentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "check agent owner failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	key := fmt.Sprintf("agents/reports/weekly/%s/%s.json", agentID.String(), week)
	raw, err := store.GetObject(ctx, key)
	if err == nil {
		var obj weeklyReportObject
		if err := json.Unmarshal(raw, &obj); err == nil {
			writeJSON(w, http.StatusOK, obj)
			return
		}
		logError(ctx, "decode weekly report failed", err)
	}

	// Generate on-demand (best-effort).
	dims, err := s.computeAgentDimensions(ctx, agentID)
	if err != nil {
		logError(ctx, "compute weekly report dimensions failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compute failed"})
		return
	}
	_ = s.ensureTimelineMaterialized(ctx, store, agentID)
	highlights, _ := s.readHighlights(ctx, store, agentID)

	// Delta vs 7 days ago (if available).
	delta := map[string]int{}
	oldDate := time.Now().UTC().Add(-7 * 24 * time.Hour).Format("2006-01-02")
	oldKey := fmt.Sprintf("agents/dimensions/%s/history/%s.json", agentID.String(), oldDate)
	if oldRaw, err := store.GetObject(ctx, oldKey); err == nil {
		var old agentDimensionsObject
		if err := json.Unmarshal(oldRaw, &old); err == nil {
			for k, v := range dims.Scores {
				delta[k] = v - old.Scores[k]
			}
		}
	}

	obj := weeklyReportObject{
		Kind:            "weekly_report",
		SchemaVersion:   1,
		AgentID:         agentID.String(),
		Week:            week,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Dimensions:      dims.Scores,
		DimensionsDelta: delta,
		Highlights:      highlights,
	}

	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "marshal weekly report failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "put weekly report failed", err)
	}
	writeJSON(w, http.StatusOK, obj)
}

var regexpWeek = func() *regexp.Regexp {
	return regexp.MustCompile(`^\d{4}-\d{2}$`)
}()

// --- 7) Timeline + highlights

type timelineEvent struct {
	Type       string         `json:"type"`
	Title      string         `json:"title"`
	Snippet    string         `json:"snippet,omitempty"`
	OccurredAt string         `json:"occurred_at"`
	Refs       map[string]any `json:"refs,omitempty"`
	Visibility string         `json:"visibility,omitempty"` // public|owner
}

type timelineDayObject struct {
	Kind         string         `json:"kind"`
	SchemaVersion int           `json:"schema_version"`
	AgentID      string         `json:"agent_id"`
	Date         string         `json:"date"`
	Events       []timelineEvent `json:"events"`
}

type timelineIndexObject struct {
	Kind         string `json:"kind"`
	SchemaVersion int   `json:"schema_version"`
	AgentID      string `json:"agent_id"`
	UpdatedAt    string `json:"updated_at"`
	Days         []timelineIndexDay `json:"days"`
}

type timelineIndexDay struct {
	Date      string `json:"date"`
	ObjectKey string `json:"object_key"`
	Count     int    `json:"count"`
}

type timelineHighlightsObject struct {
	Kind         string         `json:"kind"`
	SchemaVersion int           `json:"schema_version"`
	AgentID      string         `json:"agent_id"`
	UpdatedAt    string         `json:"updated_at"`
	Items        []timelineEvent `json:"items"`
}

func (s server) ensureTimelineMaterialized(ctx context.Context, store agenthome.OSSObjectStore, agentID uuid.UUID) error {
	rows, err := s.db.Query(ctx, `
		select action, data, created_at
		from audit_logs
		where actor_type = 'agent' and actor_id = $1
		order by created_at desc
		limit 200
	`, agentID)
	if err != nil {
		return err
	}
	defer rows.Close()

	type auditRow struct {
		action    string
		dataRaw   []byte
		createdAt time.Time
	}
	var audits []auditRow
	for rows.Next() {
		var a auditRow
		if err := rows.Scan(&a.action, &a.dataRaw, &a.createdAt); err != nil {
			return err
		}
		audits = append(audits, a)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	byDate := map[string][]timelineEvent{}
	all := make([]timelineEvent, 0, len(audits))
	for _, a := range audits {
		var data map[string]any
		if err := unmarshalJSONNullable(a.dataRaw, &data); err != nil {
			logError(ctx, "timeline audit data unmarshal failed", err)
		}

		ev := timelineEvent{
			Type:       a.action,
			Title:      a.action,
			OccurredAt: a.createdAt.UTC().Format(time.RFC3339),
			Refs:       map[string]any{},
			Visibility: "public",
		}
		runID := strings.TrimSpace(fmt.Sprintf("%v", data["run_id"]))
		if runID != "" && runID != "<nil>" {
			ev.Refs["run_id"] = runID
		}

		switch a.action {
		case "artifact_submitted":
			ev.Title = "提交产出"
			ev.Snippet = "在任务中提交了新的产出。"
		case "event_emitted":
			ev.Title = "发出事件"
			ev.Snippet = "在任务中发出事件（可能是关键节点）。"
		case "agent_synced_to_oss":
			ev.Title = "同步到 OSS"
			if v := data["card_version"]; v != nil {
				ev.Snippet = fmt.Sprintf("卡片版本：%v", v)
			} else {
				ev.Snippet = "更新了星灵在 OSS 上的公开资料。"
			}
		case "agent_updated":
			ev.Title = "更新卡片"
			ev.Snippet = "更新了星灵的 Card。"
		}

		if len([]rune(ev.Snippet)) > 160 {
			ev.Snippet = truncateRunes(ev.Snippet, 160)
		}
		d := a.createdAt.UTC().Format("2006-01-02")
		byDate[d] = append(byDate[d], ev)
		all = append(all, ev)
	}

	// Sort days (desc).
	days := make([]string, 0, len(byDate))
	for d := range byDate {
		days = append(days, d)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))

	index := timelineIndexObject{
		Kind:          "timeline_index",
		SchemaVersion: 1,
		AgentID:       agentID.String(),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Days:          []timelineIndexDay{},
	}
	for _, d := range days {
		evs := byDate[d]
		sort.Slice(evs, func(i, j int) bool { return evs[i].OccurredAt > evs[j].OccurredAt })
		dayObj := timelineDayObject{
			Kind:          "timeline_day",
			SchemaVersion: 1,
			AgentID:       agentID.String(),
			Date:          d,
			Events:        evs,
		}
		body, err := json.Marshal(dayObj)
		if err != nil {
			return err
		}
		dayKey := fmt.Sprintf("agents/timeline/%s/days/%s.json", agentID.String(), d)
		if err := store.PutObject(ctx, dayKey, "application/json", body); err != nil {
			return err
		}
		index.Days = append(index.Days, timelineIndexDay{Date: d, ObjectKey: dayKey, Count: len(evs)})
	}

	// Highlights: top 10 events by recency.
	sort.Slice(all, func(i, j int) bool { return all[i].OccurredAt > all[j].OccurredAt })
	hl := timelineHighlightsObject{
		Kind:          "timeline_highlights",
		SchemaVersion: 1,
		AgentID:       agentID.String(),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Items:         all,
	}
	if len(hl.Items) > 10 {
		hl.Items = hl.Items[:10]
	}

	indexBody, err := json.Marshal(index)
	if err != nil {
		return err
	}
	if err := store.PutObject(ctx, fmt.Sprintf("agents/timeline/%s/index.json", agentID.String()), "application/json", indexBody); err != nil {
		return err
	}
	hlBody, err := json.Marshal(hl)
	if err != nil {
		return err
	}
	if err := store.PutObject(ctx, fmt.Sprintf("agents/timeline/%s/highlights/current.json", agentID.String()), "application/json", hlBody); err != nil {
		return err
	}
	return nil
}

func (s server) readHighlights(ctx context.Context, store agenthome.OSSObjectStore, agentID uuid.UUID) ([]timelineEvent, error) {
	key := fmt.Sprintf("agents/timeline/%s/highlights/current.json", agentID.String())
	raw, err := store.GetObject(ctx, key)
	if err != nil {
		return nil, err
	}
	var obj timelineHighlightsObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return obj.Items, nil
}

func (s server) handleGetAgentHighlights(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	key := fmt.Sprintf("agents/timeline/%s/highlights/current.json", agentID.String())
	raw, err := store.GetObject(ctx, key)
	if err != nil {
		if !isOSSNotFound(err) {
			logError(ctx, "get highlights failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
			return
		}
		if err := s.ensureTimelineMaterialized(ctx, store, agentID); err != nil {
			logError(ctx, "materialize timeline failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compute failed"})
			return
		}
		raw, err = store.GetObject(ctx, key)
		if err != nil {
			logError(ctx, "get highlights after materialize failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
			return
		}
	}
	var obj timelineHighlightsObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		logError(ctx, "decode highlights failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decode failed"})
		return
	}
	writeJSON(w, http.StatusOK, obj)
}

func (s server) handleOwnerGetTimeline(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursor != "" {
		if _, err := time.Parse("2006-01-02", cursor); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid cursor"})
			return
		}
	}

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if err := s.requireOwnerAgent(ctx, userID, agentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "check agent owner failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if err := s.ensureTimelineMaterialized(ctx, store, agentID); err != nil {
		logError(ctx, "materialize timeline failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "compute failed"})
		return
	}

	dayPrefix := fmt.Sprintf("agents/timeline/%s/days/", agentID.String())
	keys, err := store.ListObjects(ctx, dayPrefix, 2000)
	if err != nil {
		logError(ctx, "list timeline days failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
		return
	}
	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	var days []string
	for _, full := range keys {
		key := stripBasePrefix(full, basePrefix)
		if !strings.HasPrefix(key, dayPrefix) || !strings.HasSuffix(key, ".json") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, dayPrefix), ".json")
		if _, err := time.Parse("2006-01-02", name); err != nil {
			continue
		}
		days = append(days, name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))

	started := cursor == ""
	var out []timelineDayObject
	nextCursor := ""
	for _, d := range days {
		if !started {
			if d < cursor {
				started = true
			} else {
				continue
			}
		}
		dayKey := fmt.Sprintf("agents/timeline/%s/days/%s.json", agentID.String(), d)
		raw, err := store.GetObject(ctx, dayKey)
		if err != nil {
			logError(ctx, "get timeline day failed", err)
			continue
		}
		var day timelineDayObject
		if err := json.Unmarshal(raw, &day); err != nil {
			logError(ctx, "decode timeline day failed", err)
			continue
		}
		out = append(out, day)
		nextCursor = d
		if len(out) >= limit {
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"days": out, "next_cursor": nextCursor})
}

// --- 6) Curations (OSS-backed; pending → approved/rejected)

type createCurationRequest struct {
	Reason string         `json:"reason"`
	Refs   map[string]any `json:"refs,omitempty"`
}

type curationEntry struct {
	Kind         string         `json:"kind"`
	SchemaVersion int           `json:"schema_version"`
	CurationID   string         `json:"curation_id"`
	ReviewStatus string         `json:"review_status"`
	OwnerID      string         `json:"owner_id"`
	Reason       string         `json:"reason"`
	Refs         map[string]any `json:"refs,omitempty"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

func (s server) handleCreateCuration(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req createCurationRequest
	if !readJSONLimited(w, r, &req, 32*1024) {
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || len([]rune(reason)) > 600 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid reason"})
		return
	}

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	entry := curationEntry{
		Kind:          "curation_entry",
		SchemaVersion: 1,
		CurationID:    id,
		ReviewStatus:  "pending",
		OwnerID:       userID.String(),
		Reason:        reason,
		Refs:          req.Refs,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	body, err := json.Marshal(entry)
	if err != nil {
		logError(ctx, "marshal curation failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	key := "curations/pending/" + id + ".json"
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "put curation pending failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"curation_id": id, "review_status": "pending"})
}

func (s server) handleAdminSetCurationStatus(w http.ResponseWriter, r *http.Request, status string) {
	if status != "approved" && status != "rejected" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "curationID"))
	if id == "" || len(id) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid curation id"})
		return
	}

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Try to load the pending object (best-effort; if missing, still allow writing status object).
	pendingKey := "curations/pending/" + id + ".json"
	raw, err := store.GetObject(ctx, pendingKey)
	entry := curationEntry{}
	if err == nil {
		_ = json.Unmarshal(raw, &entry)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if entry.CurationID == "" {
		entry = curationEntry{
			Kind:          "curation_entry",
			SchemaVersion: 1,
			CurationID:    id,
			OwnerID:       "",
			Reason:        "",
			Refs:          map[string]any{},
			CreatedAt:     now,
		}
	}
	entry.ReviewStatus = status
	entry.UpdatedAt = now

	body, err := json.Marshal(entry)
	if err != nil {
		logError(ctx, "marshal curation status failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	key := "curations/" + status + "/" + id + ".json"
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "put curation status failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s server) handleAdminApproveCuration(w http.ResponseWriter, r *http.Request) {
	s.handleAdminSetCurationStatus(w, r, "approved")
}

func (s server) handleAdminRejectCuration(w http.ResponseWriter, r *http.Request) {
	s.handleAdminSetCurationStatus(w, r, "rejected")
}

func (s server) handleListCurations(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 50)

	store, ok := s.requireOSS(w, r)
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	prefix := "curations/approved/"
	keys, err := store.ListObjects(ctx, prefix, 500)
	if err != nil {
		logError(ctx, "list curations failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
		return
	}

	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	outKeys := make([]string, 0, len(keys))
	for _, full := range keys {
		key := stripBasePrefix(full, basePrefix)
		if strings.HasPrefix(key, prefix) && strings.HasSuffix(key, ".json") {
			outKeys = append(outKeys, key)
		}
	}
	sort.Strings(outKeys)
	// Latest-ish: reverse lexical (UUID not time-ordered, but stable).
	for i, j := 0, len(outKeys)-1; i < j; i, j = i+1, j-1 {
		outKeys[i], outKeys[j] = outKeys[j], outKeys[i]
	}

	items := make([]curationEntry, 0, limit)
	for _, key := range outKeys {
		if len(items) >= limit {
			break
		}
		raw, err := store.GetObject(ctx, key)
		if err != nil {
			logError(ctx, "get curation failed", err)
			continue
		}
		var e curationEntry
		if err := json.Unmarshal(raw, &e); err != nil {
			logError(ctx, "decode curation failed", err)
			continue
		}
		if strings.TrimSpace(e.ReviewStatus) != "approved" {
			continue
		}
		items = append(items, e)
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
