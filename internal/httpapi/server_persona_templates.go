package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type personaTemplateDTO struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	OwnerID     string `json:"owner_id,omitempty"`
	ReviewStatus string `json:"review_status"`
	Persona     any    `json:"persona"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type approvedPersonaTemplateDTO struct {
	TemplateID   string `json:"template_id"`
	ReviewStatus string `json:"review_status"`
	Persona      any    `json:"persona"`
	UpdatedAt    string `json:"updated_at"`
}

func (s server) handleListApprovedPersonaTemplates(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select id, persona, updated_at
		from persona_templates
		where review_status = 'approved'
		order by updated_at desc
		limit $1
	`, limit)
	if err != nil {
		logError(ctx, "query approved persona_templates failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]approvedPersonaTemplateDTO, 0, limit)
	for rows.Next() {
		var (
			id        string
			personaRaw []byte
			updatedAt time.Time
		)
		if err := rows.Scan(&id, &personaRaw, &updatedAt); err != nil {
			logError(ctx, "scan approved persona_templates failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		var persona any
		if err := unmarshalJSONNullable(personaRaw, &persona); err != nil {
			logError(ctx, "unmarshal approved persona template failed", err)
			persona = map[string]any{}
		}
		out = append(out, approvedPersonaTemplateDTO{
			TemplateID:   id,
			ReviewStatus: "approved",
			Persona:      persona,
			UpdatedAt:    updatedAt.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "iterate approved persona_templates failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s server) handleAdminListPersonaTemplates(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "pending"
	}
	if status != "pending" && status != "approved" && status != "rejected" && status != "all" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	if offset < 0 {
		offset = 0
	}

	where := ""
	args := []any{}
	if status != "all" {
		where = "where review_status = $1"
		args = append(args, status)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select id, source, owner_id, persona, review_status, created_at, updated_at
		from persona_templates
		`+where+`
		order by updated_at desc
		limit $`+strconv.Itoa(len(args)+1)+` offset $`+strconv.Itoa(len(args)+2)+`
	`, append(args, limit, offset)...)
	if err != nil {
		logError(ctx, "query persona_templates failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	var out []personaTemplateDTO
	for rows.Next() {
		var (
			id          string
			source      string
			ownerID     *uuid.UUID
			personaRaw  []byte
			reviewStatus string
			createdAt   time.Time
			updatedAt   time.Time
		)
		if err := rows.Scan(&id, &source, &ownerID, &personaRaw, &reviewStatus, &createdAt, &updatedAt); err != nil {
			logError(ctx, "scan persona_templates failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		var persona any
		if err := unmarshalJSONNullable(personaRaw, &persona); err != nil {
			logError(ctx, "unmarshal persona template failed", err)
			persona = map[string]any{}
		}
		dto := personaTemplateDTO{
			ID:           id,
			Source:       source,
			ReviewStatus: reviewStatus,
			Persona:      persona,
			CreatedAt:    createdAt.UTC().Format(time.RFC3339),
			UpdatedAt:    updatedAt.UTC().Format(time.RFC3339),
		}
		if ownerID != nil {
			dto.OwnerID = ownerID.String()
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "iterate persona_templates failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": out, "next_offset": offset + len(out)})
}

func (s server) handleAdminApprovePersonaTemplate(w http.ResponseWriter, r *http.Request) {
	s.handleAdminSetPersonaTemplateStatus(w, r, "approved")
}

func (s server) handleAdminRejectPersonaTemplate(w http.ResponseWriter, r *http.Request) {
	s.handleAdminSetPersonaTemplateStatus(w, r, "rejected")
}

func (s server) handleAdminSetPersonaTemplateStatus(w http.ResponseWriter, r *http.Request, status string) {
	templateID := strings.TrimSpace(chi.URLParam(r, "templateID"))
	if templateID == "" || len(templateID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid template id"})
		return
	}
	if status != "approved" && status != "rejected" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	ct, err := s.db.Exec(ctx, `
		update persona_templates
		set review_status = $1, updated_at = now()
		where id = $2
	`, status, templateID)
	if err != nil {
		logError(ctx, "update persona_templates failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	s.audit(ctx, "admin", uuid.Nil, "persona_template_"+status, map[string]any{"template_id": templateID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Owner endpoint (draft): submit a custom persona template for review.
type submitPersonaTemplateRequest struct {
	ID     string `json:"id,omitempty"`
	Persona any   `json:"persona"`
}

func (s server) handleSubmitPersonaTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req submitPersonaTemplateRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	if req.Persona == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing persona"})
		return
	}

	templateID := strings.TrimSpace(req.ID)
	if templateID == "" {
		templateID = "custom_" + uuid.New().String()
	}
	if len(templateID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id too long"})
		return
	}

	personaJSON, err := marshalJSONB(req.Persona)
	if err != nil {
		logError(r.Context(), "marshal custom persona failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err = s.db.Exec(ctx, `
		insert into persona_templates (id, source, owner_id, persona, review_status)
		values ($1, 'custom', $2, $3, 'pending')
		on conflict (id) do update
		set persona = excluded.persona,
		    review_status = 'pending',
		    updated_at = now(),
		    owner_id = excluded.owner_id,
		    source = 'custom'
	`, templateID, userID, personaJSON)
	if err != nil {
		if strings.Contains(err.Error(), "persona_templates_review_status") {
			// unlikely, but keep user-facing error clear.
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid persona"})
			return
		}
		logError(ctx, "insert custom persona template failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	s.audit(ctx, "user", userID, "persona_template_submitted", map[string]any{"template_id": templateID})
	writeJSON(w, http.StatusCreated, map[string]any{"id": templateID, "review_status": "pending"})
}
