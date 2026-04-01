package service

import (
	"strings"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

func sanitizePlanSnapshot(cfg Config, snap *plan.Snapshot) {
	if snap == nil || len(snap.Plan) == 0 {
		return
	}
	worker := &Worker{Cfg: cfg}
	for i := range snap.Plan {
		node := snap.Plan[i]
		if !strings.EqualFold(strings.TrimSpace(node.Action), "build") {
			continue
		}
		job := runner.Job{
			Name:           node.Name,
			Version:        node.Version,
			PythonVersion:  node.PythonVersion,
			PythonTag:      node.PythonTag,
			PlatformTag:    node.PlatformTag,
			BuilderProfile: defaultBuilderProfileForPackage(node.Name),
			Recipes:        planRecipeNames(node.Recipes),
		}
		logContext := strings.Join(planHintSignals(node.Hints), "\n")
		resolution := worker.resolveRemediationPlan(job, failureReason{}, job.Recipes, logContext)
		node.Recipes = planRecipeMatches(resolution.Recipes, node.Recipes)
		node.Hints = sanitizePlanHints(node.Hints, resolution)
		snap.Plan[i] = node
	}
}

func planRecipeNames(recipes []plan.RecipeMatch) []string {
	if len(recipes) == 0 {
		return nil
	}
	out := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		name := strings.TrimSpace(recipe.Name)
		if name != "" {
			out = append(out, name)
		}
	}
	return dedupeStrings(out)
}

func planRecipeMatches(names []string, existing []plan.RecipeMatch) []plan.RecipeMatch {
	if len(names) == 0 {
		return nil
	}
	reasons := map[string]string{}
	for _, recipe := range existing {
		key := strings.ToLower(strings.TrimSpace(recipe.Name))
		if key == "" || reasons[key] != "" {
			continue
		}
		reasons[key] = strings.TrimSpace(recipe.Reason)
	}
	out := make([]plan.RecipeMatch, 0, len(names))
	for _, name := range dedupeStrings(names) {
		key := strings.ToLower(strings.TrimSpace(name))
		out = append(out, plan.RecipeMatch{
			Name:   name,
			Reason: reasons[key],
		})
	}
	return out
}

func planHintSignals(hints []plan.HintMatch) []string {
	if len(hints) == 0 {
		return nil
	}
	out := make([]string, 0, len(hints)*3)
	for _, hint := range hints {
		for _, raw := range []string{hint.Pattern, hint.Note, strings.Join(flattenRecipeMap(hint.Recipes), "\n")} {
			raw = strings.TrimSpace(raw)
			if raw != "" {
				out = append(out, raw)
			}
		}
	}
	return dedupeStrings(out)
}

func sanitizePlanHints(hints []plan.HintMatch, resolution remediationPlan) []plan.HintMatch {
	if len(hints) == 0 {
		return nil
	}
	out := make([]plan.HintMatch, 0, len(hints))
	for _, hint := range hints {
		hint.Recipes = sanitizeHintRecipes(hint.Recipes, resolution)
		out = append(out, hint)
	}
	return out
}

func sanitizeHintRecipes(recipes map[string][]string, resolution remediationPlan) map[string][]string {
	if len(recipes) == 0 {
		return nil
	}
	if strings.TrimSpace(resolution.RemediationTier) != remediationTierDependencyPack {
		return recipes
	}
	cleaned := stripDependencyRecipesForPackFallback(flattenRecipeMap(recipes), resolutionIntentList(resolution))
	return recipeMapFromNames(cleaned)
}

func resolutionIntentList(resolution remediationPlan) []string {
	if len(resolution.PackResolutionResult) == 0 {
		return nil
	}
	raw, ok := resolution.PackResolutionResult["logical_dependencies"]
	if !ok || raw == nil {
		return nil
	}
	switch vals := raw.(type) {
	case []string:
		return dedupeStrings(vals)
	case []any:
		out := make([]string, 0, len(vals))
		for _, val := range vals {
			if s, ok := val.(string); ok {
				out = append(out, s)
			}
		}
		return dedupeStrings(out)
	default:
		return nil
	}
}

func recipeMapFromNames(names []string) map[string][]string {
	if len(names) == 0 {
		return nil
	}
	out := map[string][]string{}
	for _, recipe := range dedupeStrings(names) {
		parts := strings.SplitN(recipe, ":", 2)
		if len(parts) != 2 {
			continue
		}
		mgr := strings.TrimSpace(parts[0])
		step := strings.TrimSpace(parts[1])
		if mgr == "" || step == "" {
			continue
		}
		out[mgr] = append(out[mgr], step)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneHint(h plan.Hint) *plan.Hint {
	clone := h
	if len(h.Tags) > 0 {
		clone.Tags = append([]string(nil), h.Tags...)
	}
	if len(h.Examples) > 0 {
		clone.Examples = append([]string(nil), h.Examples...)
	}
	if len(h.AppliesTo) > 0 {
		clone.AppliesTo = make(map[string][]string, len(h.AppliesTo))
		for key, vals := range h.AppliesTo {
			clone.AppliesTo[key] = append([]string(nil), vals...)
		}
	}
	if len(h.Recipes) > 0 {
		clone.Recipes = make(map[string][]string, len(h.Recipes))
		for key, vals := range h.Recipes {
			clone.Recipes[key] = append([]string(nil), vals...)
		}
	}
	return &clone
}

func persistableResolvedHint(hint plan.Hint, resolution remediationPlan) (*plan.Hint, string) {
	switch strings.TrimSpace(resolution.RemediationTier) {
	case remediationTierRepoPackage, remediationTierNormalizedAlternative:
		recipes := recipeMapFromNames(resolution.Recipes)
		if len(recipes) == 0 {
			return nil, "no normalized recipes to persist"
		}
		hint.Recipes = recipes
		if hint.ID == "" {
			return nil, "missing hint id"
		}
		return &hint, ""
	case remediationTierDependencyPack:
		return nil, "dependency-pack fallback selected"
	case remediationTierFeatureDegraded:
		return nil, "feature-degraded fallback selected"
	case remediationTierBlocked:
		return nil, "remediation blocked"
	default:
		if len(resolution.PackRequirements) > 0 {
			return nil, "dependency-pack fallback selected"
		}
		return nil, "no persistable remediation selected"
	}
}
