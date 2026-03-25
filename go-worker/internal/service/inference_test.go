package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
)

func TestDecodeInferenceSuggestionDirect(t *testing.T) {
	raw := []byte(`{"pattern":"fatal error","confidence":0.8,"reason_code":"missing_header","summary":"Install headers","recipes":{"apt":["libssl-dev"]},"tags":["openssl"]}`)
	s, err := decodeInferenceSuggestion(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Pattern != "fatal error" {
		t.Fatalf("pattern mismatch: %q", s.Pattern)
	}
	if len(s.Recipes["apt"]) != 1 {
		t.Fatalf("expected apt recipe")
	}
}

func TestDecodeInferenceSuggestionChat(t *testing.T) {
	raw := []byte(`{"choices":[{"message":{"content":"{\"pattern\":\"No package 'zlib' found\",\"confidence\":0.6,\"reason_code\":\"missing_library\",\"summary\":\"Install zlib dev\",\"recipes\":{\"apt\":[\"zlib1g-dev\"]},\"tags\":[\"zlib\"]}"}}]}`)
	s, err := decodeInferenceSuggestion(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Pattern == "" || len(s.Recipes["apt"]) == 0 {
		t.Fatalf("expected parsed suggestion")
	}
}

func TestParseInferenceJSONCodeFence(t *testing.T) {
	content := "```json\n{\"pattern\":\"missing\",\"confidence\":95,\"recipes\":{\"dnf\":[\"openssl-devel\"]}}\n```"
	s, err := parseInferenceJSON(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Confidence != 95 {
		t.Fatalf("expected confidence 95, got %v", s.Confidence)
	}
	normalized := normalizeSuggestion(s)
	if normalized.Confidence <= 0.9 {
		t.Fatalf("expected normalized confidence > 0.9, got %v", normalized.Confidence)
	}
}

func TestRenderInferencePromptTemplate(t *testing.T) {
	ctxHint := plan.HintContext{
		Package:       "cryptography",
		Version:       "42.0.0",
		PythonVersion: "3.11",
		PlatformTag:   "manylinux2014_s390x",
	}
	prompt := renderInferencePrompt(
		"Package={{package}}\nVersion={{version}}\nPython={{python}}\nPlatform={{platform}}\nRecipes={{existing_recipes}}\n{{log_excerpt}}",
		ctxHint,
		"fatal error: openssl/ssl.h",
		[]string{"openssl-devel", "pkg-config"},
	)
	if !strings.Contains(prompt, "Package=cryptography") || !strings.Contains(prompt, "Recipes=openssl-devel, pkg-config") {
		t.Fatalf("expected rendered prompt to include template values, got %q", prompt)
	}
}

func TestInferHintFromLLMUsesConfiguredPromptsAndBearerToken(t *testing.T) {
	ctxHint := plan.HintContext{
		Package:       "cryptography",
		Version:       "42.0.0",
		PythonVersion: "3.11",
		PlatformTag:   "manylinux2014_s390x",
	}
	var gotAuth string
	var gotSystem string
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var req inferenceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("expected two chat messages, got %d", len(req.Messages))
		}
		gotSystem = req.Messages[0].Content
		gotUser = req.Messages[1].Content
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"pattern\":\"fatal error: openssl/ssl.h\",\"confidence\":0.8,\"reason_code\":\"missing_header\",\"summary\":\"Install openssl headers\",\"recipes\":{\"apt\":[\"libssl-dev\"]},\"tags\":[\"openssl\"]}"}}]}`))
	}))
	defer srv.Close()

	worker := &Worker{
		Cfg: Config{
			InferEnabled:            true,
			InferURL:                srv.URL,
			InferToken:              "secret-token",
			InferModel:              "gpt-4.1-mini",
			InferTimeoutSec:         5,
			InferMaxRetries:         0,
			InferSystemPrompt:       "custom system prompt",
			InferUserPromptTemplate: "Pkg {{package}} uses {{existing_recipes}}\n{{log_excerpt}}",
		},
	}
	hint, recipes, note, ok, trace := worker.inferHintFromLLM(context.Background(), "fatal error: openssl/ssl.h", ctxHint, []string{"openssl-devel"})
	if !ok {
		t.Fatalf("expected inference success, trace=%v", trace)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("expected bearer token, got %q", gotAuth)
	}
	if gotSystem != "custom system prompt" {
		t.Fatalf("expected custom system prompt, got %q", gotSystem)
	}
	if !strings.Contains(gotUser, "Pkg cryptography uses openssl-devel") || !strings.Contains(gotUser, "fatal error: openssl/ssl.h") {
		t.Fatalf("expected rendered user prompt, got %q", gotUser)
	}
	if hint.Pattern == "" || note == "" || len(recipes) == 0 {
		t.Fatalf("expected normalized hint output, got hint=%+v recipes=%v note=%q", hint, recipes, note)
	}
}
