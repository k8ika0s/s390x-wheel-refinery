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

func TestNormalizeSuggestionRecipeCommands(t *testing.T) {
	s := normalizeSuggestion(inferenceSuggestion{
		Pattern: "build_dependency_failure",
		Recipes: map[string][]string{
			"dnf": {
				"sudo dnf install gcc-toolset-12",
				"scl enable gcc-toolset-12 'python -m pip install pandas==2.2.3'",
			},
			"env": {
				"export CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc",
				`"CXX=/opt/rh/gcc-toolset-12/root/usr/bin/g++"`,
			},
			"pip": {
				"python -m pip install numpy==2.1.2 wheel",
			},
		},
	})
	if got := strings.Join(s.Recipes["dnf"], ","); got != "gcc-toolset-12" {
		t.Fatalf("expected normalized dnf recipe, got %q", got)
	}
	if got := strings.Join(s.Recipes["env"], ","); got != "CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc,CXX=/opt/rh/gcc-toolset-12/root/usr/bin/g++" {
		t.Fatalf("expected normalized env recipes, got %q", got)
	}
	if got := strings.Join(s.Recipes["pip"], ","); got != "numpy==2.1.2,wheel" {
		t.Fatalf("expected normalized pip recipes, got %q", got)
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

func TestInferHintFromLLMIgnoresCanceledParentContext(t *testing.T) {
	ctxHint := plan.HintContext{
		Package:       "pandas",
		Version:       "2.2.3",
		PythonVersion: "3.11",
		PlatformTag:   "manylinux2014_s390x",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"pattern\":\"build_dependency_failure\",\"confidence\":0.95,\"reason_code\":\"gcc_version_too_low\",\"summary\":\"Upgrade compiler\",\"recipes\":{\"dnf\":[\"sudo dnf install gcc-toolset-12\"],\"env\":[\"export CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc\"]},\"tags\":[\"gcc\"]}"}}]}`))
	}))
	defer srv.Close()

	worker := &Worker{
		Cfg: Config{
			InferEnabled:    true,
			InferURL:        srv.URL,
			InferTimeoutSec: 5,
		},
	}
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	hint, recipes, _, ok, trace := worker.inferHintFromLLM(parent, "NumPy requires GCC >= 9.3", ctxHint, nil)
	if !ok {
		t.Fatalf("expected inference success with canceled parent context, trace=%v", trace)
	}
	if hint.Pattern != "build_dependency_failure" {
		t.Fatalf("expected hint pattern, got %+v", hint)
	}
	if got := strings.Join(recipes, ","); got != "dnf:gcc-toolset-12,env:CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc" {
		t.Fatalf("expected normalized recipes, got %q", got)
	}
}

func TestFilterSuggestionRecipesDropsTargetPackagePipRecipe(t *testing.T) {
	ctxHint := plan.HintContext{Package: "pandas"}
	recipes := filterSuggestionRecipes(map[string][]string{
		"pip": {"pandas==2.2.3", "numpy==2.0.2"},
		"env": {"NPY_ALLOW_BLAS_UNSAFE=1"},
	}, ctxHint)
	if got := strings.Join(recipes["pip"], ","); got != "numpy==2.0.2" {
		t.Fatalf("expected only transitive pip recipe, got %q", got)
	}
	if got := strings.Join(recipes["env"], ","); got != "NPY_ALLOW_BLAS_UNSAFE=1" {
		t.Fatalf("expected env recipe preserved, got %q", got)
	}
}
