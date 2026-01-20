package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
)

type inferenceSuggestion struct {
	Pattern    string              `json:"pattern"`
	Confidence float64             `json:"confidence"`
	ReasonCode string              `json:"reason_code"`
	Summary    string              `json:"summary"`
	Recipes    map[string][]string `json:"recipes"`
	Notes      string              `json:"notes"`
	Tags       []string            `json:"tags"`
}

type inferenceMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type inferenceRequest struct {
	Model       string             `json:"model,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	Messages    []inferenceMessage `json:"messages"`
}

type inferenceResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Text string `json:"text"`
	} `json:"choices"`
}

const inferenceSystemPrompt = "You are a build-failure triage assistant. Return only a JSON object with: pattern, confidence (0-1), reason_code, summary, recipes (apt/dnf/pip/env arrays), notes, tags. Use minimal safe fixes."

func inferenceUserPrompt(ctx plan.HintContext, logContent string, existingRecipes []string) string {
	lines := []string{
		"Package: " + strings.TrimSpace(ctx.Package),
		"Version: " + strings.TrimSpace(ctx.Version),
		"Python: " + strings.TrimSpace(firstNonEmpty(ctx.PythonVersion, ctx.PythonTag)),
		"Platform: " + strings.TrimSpace(ctx.PlatformTag),
	}
	if len(existingRecipes) > 0 {
		lines = append(lines, "Existing recipes: "+strings.Join(existingRecipes, ", "))
	}
	lines = append(lines, "Log excerpt:", logContent)
	return strings.Join(lines, "\n")
}

func (w *Worker) inferHintFromLLM(ctx context.Context, logContent string, ctxHint plan.HintContext, existingRecipes []string) (plan.Hint, []string, string, bool, []string) {
	trace := []string{}
	if !w.Cfg.InferEnabled {
		return plan.Hint{}, nil, "", false, []string{"llm inference disabled"}
	}
	if strings.TrimSpace(w.Cfg.InferURL) == "" {
		return plan.Hint{}, nil, "", false, []string{"llm inference url not set"}
	}
	prompt := inferenceUserPrompt(ctxHint, logContent, existingRecipes)
	payload := inferenceRequest{
		Model:       strings.TrimSpace(w.Cfg.InferModel),
		Temperature: 0.2,
		Messages: []inferenceMessage{
			{Role: "system", Content: inferenceSystemPrompt},
			{Role: "user", Content: prompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return plan.Hint{}, nil, "", false, []string{fmt.Sprintf("llm request marshal failed: %v", err)}
	}
	timeoutSec := w.Cfg.InferTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 20
	}
	maxRetries := w.Cfg.InferMaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, w.Cfg.InferURL, bytes.NewReader(body))
		if reqErr != nil {
			return plan.Hint{}, nil, "", false, []string{fmt.Sprintf("llm request create failed: %v", reqErr)}
		}
		req.Header.Set("Content-Type", "application/json")
		if token := strings.TrimSpace(w.Cfg.InferToken); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
			continue
		}
		suggestion, err := decodeInferenceSuggestion(raw)
		if err != nil {
			return plan.Hint{}, nil, "", false, []string{fmt.Sprintf("llm decode failed: %v", err)}
		}
		suggestion = normalizeSuggestion(suggestion)
		if suggestion.Pattern == "" {
			return plan.Hint{}, nil, "", false, []string{"llm returned empty pattern"}
		}
		if len(suggestion.Recipes) == 0 {
			return plan.Hint{}, nil, "", false, []string{"llm returned no recipes"}
		}
		trace = append(trace, fmt.Sprintf("llm suggested pattern %s", suggestion.Pattern))
		confLabel := confidenceLabel(suggestion.Confidence)
		if confLabel != "" {
			trace = append(trace, fmt.Sprintf("llm confidence %s", confLabel))
		}
		hint := baseAutoHint(ctxHint, suggestion.Pattern)
		hint.Tags = append(hint.Tags, "llm", "suggested")
		if suggestion.ReasonCode != "" {
			hint.Tags = append(hint.Tags, strings.ToLower(suggestion.ReasonCode))
		}
		if len(suggestion.Tags) > 0 {
			hint.Tags = append(hint.Tags, suggestion.Tags...)
		}
		hint.Confidence = confLabel
		hint.Note = firstNonEmpty(suggestion.Summary, suggestion.Notes)
		hint.Recipes = suggestion.Recipes
		return hint, flattenRecipeMap(hint.Recipes), hint.Note, true, trace
	}
	if lastErr != nil {
		return plan.Hint{}, nil, "", false, []string{fmt.Sprintf("llm request failed: %v", lastErr)}
	}
	return plan.Hint{}, nil, "", false, []string{"llm request failed"}
}

func decodeInferenceSuggestion(raw []byte) (inferenceSuggestion, error) {
	var direct inferenceSuggestion
	if err := json.Unmarshal(raw, &direct); err == nil && (direct.Pattern != "" || len(direct.Recipes) > 0) {
		return direct, nil
	}
	var resp inferenceResponse
	if err := json.Unmarshal(raw, &resp); err == nil && len(resp.Choices) > 0 {
		content := strings.TrimSpace(resp.Choices[0].Message.Content)
		if content == "" {
			content = strings.TrimSpace(resp.Choices[0].Text)
		}
		if content != "" {
			return parseInferenceJSON(content)
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err == nil {
		for _, key := range []string{"output", "content", "result"} {
			if val, ok := payload[key].(string); ok && strings.TrimSpace(val) != "" {
				return parseInferenceJSON(val)
			}
		}
	}
	return inferenceSuggestion{}, fmt.Errorf("no inference payload detected")
}

func parseInferenceJSON(content string) (inferenceSuggestion, error) {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)
	var out inferenceSuggestion
	if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
		return out, nil
	}
	start := strings.Index(trimmed, "{")
	if start == -1 {
		return inferenceSuggestion{}, fmt.Errorf("no json object found")
	}
	dec := json.NewDecoder(strings.NewReader(trimmed[start:]))
	if err := dec.Decode(&out); err != nil {
		return inferenceSuggestion{}, err
	}
	return out, nil
}

func normalizeSuggestion(s inferenceSuggestion) inferenceSuggestion {
	s.Pattern = strings.TrimSpace(s.Pattern)
	s.ReasonCode = strings.TrimSpace(s.ReasonCode)
	s.Summary = strings.TrimSpace(s.Summary)
	s.Notes = strings.TrimSpace(s.Notes)
	s.Tags = dedupeStrings(s.Tags)
	if s.Confidence > 1 {
		if s.Confidence <= 100 {
			s.Confidence = s.Confidence / 100
		} else {
			s.Confidence = 1
		}
	}
	s.Recipes = normalizeRecipeMap(s.Recipes)
	return s
}

func normalizeRecipeMap(recipes map[string][]string) map[string][]string {
	if len(recipes) == 0 {
		return nil
	}
	out := make(map[string][]string)
	for mgr, steps := range recipes {
		key := strings.TrimSpace(strings.ToLower(mgr))
		if key == "" {
			continue
		}
		seen := make(map[string]bool)
		var cleaned []string
		for _, step := range steps {
			trimmed := strings.TrimSpace(step)
			if trimmed == "" {
				continue
			}
			if seen[strings.ToLower(trimmed)] {
				continue
			}
			seen[strings.ToLower(trimmed)] = true
			cleaned = append(cleaned, trimmed)
		}
		if len(cleaned) > 0 {
			out[key] = cleaned
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func confidenceLabel(score float64) string {
	switch {
	case score >= 0.7:
		return "high"
	case score >= 0.35:
		return "medium"
	case score > 0:
		return "low"
	default:
		return ""
	}
}
