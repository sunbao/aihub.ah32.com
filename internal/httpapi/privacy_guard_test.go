package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func hasKind(kinds []privacyKind, want privacyKind) bool {
	for _, k := range kinds {
		if k == want {
			return true
		}
	}
	return false
}

func hasFinding(findings []privacyFinding, wantKind privacyKind, wantField string) bool {
	for _, f := range findings {
		if f.Kind == wantKind && f.Field == wantField {
			return true
		}
	}
	return false
}

func TestDetectPrivacyInString(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want privacyKind
	}{
		{name: "email", in: "contact me: test@example.com", want: privacyEmail},
		{name: "phone_cn", in: "我的电话是 13812345678", want: privacyPhone},
		{name: "phone_e164", in: "call +14155552671 ok", want: privacyPhone},
		{name: "id18", in: "id=11010519491231002X", want: privacyIDNumber},
		{name: "secret_openai", in: "sk-abcdefghijklmnopqrstuvwxyz123456", want: privacySecret},
		{name: "private_key_pem", in: "-----BEGIN PRIVATE KEY-----\nabc", want: privacyPrivateKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectPrivacyInString(tc.in)
			if !hasKind(got, tc.want) {
				t.Fatalf("expected kind %q, got=%v", tc.want, got)
			}
		})
	}
}

func TestDetectPrivacyFindings_TraversesNested(t *testing.T) {
	payload := map[string]any{
		"text": "email a@b.com",
		"nested": map[string]any{
			"phone": "+14155552671",
		},
		"list": []any{
			map[string]any{"id": "11010519491231002X"},
		},
	}
	findings := detectPrivacyFindings(payload, "payload")
	if !hasFinding(findings, privacyEmail, "payload.text") {
		t.Fatalf("missing email finding: %v", findings)
	}
	if !hasFinding(findings, privacyPhone, "payload.nested.phone") {
		t.Fatalf("missing phone finding: %v", findings)
	}
	if !hasFinding(findings, privacyIDNumber, "payload.list[].id") {
		t.Fatalf("missing id_number finding: %v", findings)
	}
}

func TestRejectIfPrivacyViolation_WritesBadRequest(t *testing.T) {
	rr := httptest.NewRecorder()
	ctx := context.Background()
	ok := rejectIfPrivacyViolation(ctx, rr, map[string]any{"text": "x@y.com"}, "content", "test")
	if !ok {
		t.Fatalf("expected rejection")
	}
	if rr.Code != 400 {
		t.Fatalf("expected status 400, got=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json response: %v body=%s", err, rr.Body.String())
	}
	if body["error"] != "privacy_violation" {
		t.Fatalf("unexpected error field: %v", body)
	}
	kinds, _ := body["kinds"].([]any)
	if len(kinds) == 0 {
		t.Fatalf("expected non-empty kinds: %v", body)
	}
}
