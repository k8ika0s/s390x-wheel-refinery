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

type llmInferenceDecision struct {
	Hint             plan.Hint
	Recipes          []string
	Note             string
	OK               bool
	Trace            []string
	RawOutput        string
	NormalizedOutput any
	Ignored          bool
	IgnoreReason     string
	PromptVersion    string
}

const legacyInferenceSystemPrompt = "You are a build-failure triage assistant. Return only a JSON object with: pattern, confidence (0-1), reason_code, summary, recipes (apt/dnf/pip/env arrays), notes, tags. Use minimal safe fixes."
const defaultInferenceSystemPrompt = legacyInferenceSystemPrompt + " Recipes must be raw package or requirement names only for apt/dnf/pip, and plain KEY=VALUE entries only for env. Do not return sudo, export, shell commands, quotes, or pipelines. Do not include the target package itself in pip recipes. Prefer deterministic OS/compiler fixes over transitive Python dependency pinning when the log clearly shows a missing system package or compiler version problem."
const defaultInferenceUserPromptTemplate = "Package: {{package}}\nVersion: {{version}}\nPython: {{python}}\nPlatform: {{platform}}\nExisting recipes: {{existing_recipes}}\nLog excerpt:\n{{log_excerpt}}"

func renderInferencePrompt(template string, ctx plan.HintContext, logContent string, existingRecipes []string) string {
	template = strings.TrimSpace(template)
	if template == "" {
		template = defaultInferenceUserPromptTemplate
	}
	recipes := strings.Join(existingRecipes, ", ")
	if recipes == "" {
		recipes = "(none)"
	}
	replacer := strings.NewReplacer(
		"{{package}}", strings.TrimSpace(ctx.Package),
		"{{version}}", strings.TrimSpace(ctx.Version),
		"{{python}}", strings.TrimSpace(firstNonEmpty(ctx.PythonVersion, ctx.PythonTag)),
		"{{platform}}", strings.TrimSpace(ctx.PlatformTag),
		"{{existing_recipes}}", recipes,
		"{{log_excerpt}}", logContent,
	)
	return replacer.Replace(template)
}

func (w *Worker) inferHintFromLLM(ctx context.Context, logContent string, ctxHint plan.HintContext, existingRecipes []string) (plan.Hint, []string, string, bool, []string) {
	decision := w.inferHintFromLLMDecision(ctx, logContent, ctxHint, existingRecipes)
	return decision.Hint, decision.Recipes, decision.Note, decision.OK, decision.Trace
}

func (w *Worker) inferHintFromLLMDecision(ctx context.Context, logContent string, ctxHint plan.HintContext, existingRecipes []string) llmInferenceDecision {
	trace := []string{}
	if !w.Cfg.InferEnabled {
		return llmInferenceDecision{Trace: []string{"llm inference disabled"}, PromptVersion: promptVersion(w.Cfg)}
	}
	if strings.TrimSpace(w.Cfg.InferURL) == "" {
		return llmInferenceDecision{Trace: []string{"llm inference url not set"}, PromptVersion: promptVersion(w.Cfg)}
	}
	prompt := renderInferencePrompt(w.Cfg.InferUserPromptTemplate, ctxHint, logContent, existingRecipes)
	systemPrompt := strings.TrimSpace(w.Cfg.InferSystemPrompt)
	if systemPrompt == "" || systemPrompt == legacyInferenceSystemPrompt {
		systemPrompt = defaultInferenceSystemPrompt
	}
	currentPromptVersion := promptVersion(w.Cfg)
	payload := inferenceRequest{
		Model:       strings.TrimSpace(w.Cfg.InferModel),
		Temperature: 0.2,
		Messages: []inferenceMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llmInferenceDecision{Trace: []string{fmt.Sprintf("llm request marshal failed: %v", err)}, PromptVersion: currentPromptVersion}
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
	attempts := maxRetries + 1
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		reqCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
		started := time.Now()
		req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodPost, w.Cfg.InferURL, bytes.NewReader(body))
		if reqErr != nil {
			cancel()
			return llmInferenceDecision{Trace: []string{fmt.Sprintf("llm request create failed: %v", reqErr)}, PromptVersion: currentPromptVersion}
		}
		req.Header.Set("Content-Type", "application/json")
		if token := strings.TrimSpace(w.Cfg.InferToken); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			trace = append(trace, fmt.Sprintf("llm attempt %d/%d failed after %s: %v", attempt+1, attempts, time.Since(started).Round(100*time.Millisecond), err))
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		trace = append(trace, fmt.Sprintf("llm attempt %d/%d status %d in %s", attempt+1, attempts, resp.StatusCode, time.Since(started).Round(100*time.Millisecond)))
		if resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
			continue
		}
		suggestion, err := decodeInferenceSuggestion(raw)
		if err != nil {
			return llmInferenceDecision{
				Trace:         []string{fmt.Sprintf("llm decode failed: %v", err)},
				RawOutput:     strings.TrimSpace(string(raw)),
				PromptVersion: currentPromptVersion,
			}
		}
		suggestion = normalizeSuggestion(suggestion)
		suggestion.Recipes = filterSuggestionRecipes(suggestion.Recipes, ctxHint)
		if suggestion.Pattern == "" {
			return llmInferenceDecision{
				Trace:            []string{"llm returned empty pattern"},
				RawOutput:        strings.TrimSpace(string(raw)),
				NormalizedOutput: normalizeForMetadata(suggestion),
				Ignored:          true,
				IgnoreReason:     "empty_pattern",
				PromptVersion:    currentPromptVersion,
			}
		}
		if len(suggestion.Recipes) == 0 {
			return llmInferenceDecision{
				Trace:            []string{"llm returned no recipes"},
				RawOutput:        strings.TrimSpace(string(raw)),
				NormalizedOutput: normalizeForMetadata(suggestion),
				Ignored:          true,
				IgnoreReason:     "no_recipes",
				PromptVersion:    currentPromptVersion,
			}
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
		return llmInferenceDecision{
			Hint:             hint,
			Recipes:          flattenRecipeMap(hint.Recipes),
			Note:             hint.Note,
			OK:               true,
			Trace:            trace,
			RawOutput:        strings.TrimSpace(string(raw)),
			NormalizedOutput: normalizeForMetadata(suggestion),
			PromptVersion:    currentPromptVersion,
		}
	}
	if lastErr != nil {
		return llmInferenceDecision{Trace: []string{fmt.Sprintf("llm request failed: %v", lastErr)}, PromptVersion: currentPromptVersion}
	}
	return llmInferenceDecision{Trace: []string{"llm request failed"}, PromptVersion: currentPromptVersion}
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
			for _, normalized := range normalizeRecipeSteps(key, step) {
				if seen[strings.ToLower(normalized)] {
					continue
				}
				seen[strings.ToLower(normalized)] = true
				cleaned = append(cleaned, normalized)
			}
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

func filterSuggestionRecipes(recipes map[string][]string, ctx plan.HintContext) map[string][]string {
	if len(recipes) == 0 {
		return nil
	}
	target := normalizePackageName(ctx.Package)
	if target == "" {
		return recipes
	}
	out := make(map[string][]string, len(recipes))
	for mgr, steps := range recipes {
		if strings.ToLower(mgr) != "pip" {
			out[mgr] = steps
			continue
		}
		filtered := make([]string, 0, len(steps))
		for _, step := range steps {
			name := normalizeRecipeRequirementName(step)
			if name == "" || name != target {
				filtered = append(filtered, step)
			}
		}
		if len(filtered) > 0 {
			out[mgr] = filtered
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRecipeRequirementName(step string) string {
	trimmed := strings.TrimSpace(step)
	if trimmed == "" {
		return ""
	}
	for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<", "["} {
		if idx := strings.Index(trimmed, sep); idx >= 0 {
			trimmed = trimmed[:idx]
			break
		}
	}
	return normalizePackageName(trimmed)
}

func normalizePackageName(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
}

func normalizeRecipeSteps(manager, step string) []string {
	trimmed := strings.TrimSpace(step)
	if trimmed == "" {
		return nil
	}
	switch manager {
	case "env":
		return normalizeEnvRecipeStep(trimmed)
	case "apt":
		return normalizePackageRecipeStep(trimmed, "apt-get install", "apt install")
	case "dnf":
		return normalizePackageRecipeStep(trimmed, "dnf install", "yum install", "microdnf install")
	case "pip":
		return normalizePackageRecipeStep(trimmed, "python -m pip install", "python3 -m pip install", "pip install", "pip3 install")
	default:
		return []string{trimmed}
	}
}

func normalizeEnvRecipeStep(step string) []string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(step, "export "))
	trimmed = strings.Trim(trimmed, `"'`)
	if trimmed == "" || containsShellOperators(trimmed) {
		return nil
	}
	parts := strings.SplitN(trimmed, "=", 2)
	if len(parts) != 2 {
		return nil
	}
	key := strings.TrimSpace(parts[0])
	if key == "" || strings.ContainsAny(key, " \t") {
		return nil
	}
	return []string{key + "=" + strings.TrimSpace(parts[1])}
}

func normalizePackageRecipeStep(step string, installPrefixes ...string) []string {
	trimmed := strings.TrimSpace(step)
	trimmed = strings.TrimPrefix(trimmed, "sudo ")
	lower := strings.ToLower(trimmed)
	matchedPrefix := false
	for _, prefix := range installPrefixes {
		if strings.HasPrefix(lower, prefix+" ") {
			trimmed = strings.TrimSpace(trimmed[len(prefix):])
			lower = strings.ToLower(trimmed)
			matchedPrefix = true
			break
		}
	}
	if trimmed == "" || containsShellOperators(trimmed) {
		return nil
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil
	}
	if !matchedPrefix {
		if strings.ContainsAny(trimmed, `"'`) {
			return nil
		}
		switch strings.ToLower(fields[0]) {
		case "scl", "bash", "sh", "python", "python3", "pip", "pip3", "dnf", "yum", "apt", "apt-get", "microdnf", "sudo", "export":
			return nil
		}
	}
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.HasPrefix(field, "-") {
			continue
		}
		field = strings.Trim(field, `"'`)
		if field == "" {
			continue
		}
		out = append(out, field)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func containsShellOperators(step string) bool {
	return strings.ContainsAny(step, "\n;&|`") || strings.Contains(step, "$(")
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
