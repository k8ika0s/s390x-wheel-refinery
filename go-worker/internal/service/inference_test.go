package service

import "testing"

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
