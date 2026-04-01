package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/artifact"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/pack"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

const (
	builderProfileDefault     = "default"
	builderProfileNativeHeavy = "native-heavy"

	remediationTierRepoPackage           = "repo_package"
	remediationTierNormalizedAlternative = "normalized_alternative"
	remediationTierDependencyPack        = "dependency_pack"
	remediationTierFeatureDegraded       = "feature_degraded"
	remediationTierBlocked               = "blocked"
)

type remediationPlan struct {
	BuilderProfile       string
	RemediationTier      string
	Recipes              []string
	MissingPackages      []string
	PackRequirements     []string
	PackResolutionResult map[string]any
	DegradedReason       string
}

func builderImageForProfile(cfg Config, profile string) string {
	if strings.EqualFold(strings.TrimSpace(profile), builderProfileNativeHeavy) {
		if image := strings.TrimSpace(cfg.ContainerImageNativeHeavy); image != "" {
			return image
		}
	}
	return strings.TrimSpace(cfg.ContainerImage)
}

func defaultBuilderProfileForPackage(pkg string) string {
	pkg = strings.ToLower(strings.TrimSpace(pkg))
	switch pkg {
	case "numpy", "scipy", "scikit-learn", "pandas", "lxml", "cryptography", "cffi":
		return builderProfileNativeHeavy
	default:
		return builderProfileDefault
	}
}

func explicitBuilderProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case builderProfileDefault:
		return builderProfileDefault
	case builderProfileNativeHeavy:
		return builderProfileNativeHeavy
	default:
		return ""
	}
}

func resolveBuilderProfile(job runner.Job, failure failureReason, logContent string) string {
	current := explicitBuilderProfile(job.BuilderProfile)
	if current == "" {
		current = explicitBuilderProfile(metadataString(job.Metadata, "builder_profile"))
	}
	desired := defaultBuilderProfileForPackage(job.Name)
	if current == "" {
		current = desired
	}
	if current == builderProfileDefault && desired == builderProfileNativeHeavy && shouldEscalateToNativeHeavy(job, failure, logContent) {
		return builderProfileNativeHeavy
	}
	return current
}

func normalizeBuilderProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "", builderProfileDefault:
		return builderProfileDefault
	case builderProfileNativeHeavy:
		return builderProfileNativeHeavy
	default:
		return builderProfileDefault
	}
}

func shouldEscalateToNativeHeavy(job runner.Job, failure failureReason, logContent string) bool {
	if defaultBuilderProfileForPackage(job.Name) != builderProfileNativeHeavy {
		return false
	}
	switch strings.TrimSpace(failure.Code) {
	case "compiler_version_too_old", "package_unavailable", "build_timeout":
		return true
	}
	lowerLog := strings.ToLower(logContent)
	if strings.Contains(lowerLog, "runner: command exceeded timeout") ||
		strings.Contains(lowerLog, "status=error reason=timeout") {
		return true
	}
	for _, recipe := range job.Recipes {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(recipe)), "dnf:gcc-toolset-12") {
			return true
		}
	}
	return false
}

func packRequirementsFromMetadata(meta map[string]any) []string {
	if len(meta) == 0 {
		return nil
	}
	return dedupeStrings(metadataStringSlice(meta, "pack_requirements"))
}

func metadataString(meta map[string]any, key string) string {
	if len(meta) == 0 {
		return ""
	}
	raw, ok := meta[key]
	if !ok {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func metadataStringSlice(meta map[string]any, key string) []string {
	if len(meta) == 0 {
		return nil
	}
	raw, ok := meta[key]
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

func autoFixSignature(recipes []string, packRequirements []string, builderProfile string, degradedReason string) string {
	parts := []string{}
	if rec := recipeSignature(recipes); rec != "" {
		parts = append(parts, "recipes="+rec)
	}
	if packs := strings.ToLower(strings.Join(dedupeStrings(packRequirements), "|")); packs != "" {
		parts = append(parts, "packs="+packs)
	}
	if profile := normalizeBuilderProfile(builderProfile); profile != "" {
		parts = append(parts, "profile="+profile)
	}
	if degraded := strings.TrimSpace(degradedReason); degraded != "" {
		parts = append(parts, "degraded="+strings.ToLower(degraded))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ";")
}

func (w *Worker) resolveRemediationPlan(job runner.Job, failure failureReason, recipes []string, logContent string) remediationPlan {
	recipes, _ = sanitizeRecipesForFailure(recipes, failure, logContent)
	plan := remediationPlan{
		BuilderProfile: resolveBuilderProfile(job, failure, logContent),
		Recipes:        dedupeStrings(recipes),
	}
	if len(plan.Recipes) > 0 {
		plan.RemediationTier = remediationTierRepoPackage
	}
	intents := inferDependencyIntents(nil, logContent, plan.Recipes)
	if shouldPreferPackFallback(plan.BuilderProfile, intents) {
		if packs := dependencyPackFallbacks(intents, w.Cfg.PackCatalog); len(packs) > 0 {
			plan.RemediationTier = remediationTierDependencyPack
			plan.PackRequirements = packs
			plan.Recipes = stripDependencyRecipesForPackFallback(plan.Recipes, intents)
			plan.PackResolutionResult = map[string]any{
				"logical_dependencies": intents,
				"selected_packs":       packs,
				"resolution":           "pack_fallback_preferred",
			}
			return plan
		}
	}
	unavailable := unavailablePackages(failure, logContent)
	if len(unavailable) == 0 {
		return plan
	}
	plan.MissingPackages = sortKeysWithPrefixTrim(unavailable)
	intents = inferDependencyIntents(plan.MissingPackages, logContent, plan.Recipes)
	result := map[string]any{
		"missing_packages": plan.MissingPackages,
	}
	if len(intents) > 0 {
		result["logical_dependencies"] = intents
	}

	alternatives := normalizedAlternativeRecipes(plan.BuilderProfile, intents, unavailable)
	if len(alternatives) > 0 {
		plan.RemediationTier = remediationTierNormalizedAlternative
		plan.Recipes = mergeRecipes(plan.Recipes, alternatives)
		result["normalized_recipes"] = dedupeStrings(alternatives)
		plan.PackResolutionResult = result
		return plan
	}

	packs := dependencyPackFallbacks(intents, w.Cfg.PackCatalog)
	if len(packs) > 0 {
		plan.RemediationTier = remediationTierDependencyPack
		plan.PackRequirements = packs
		result["selected_packs"] = packs
		result["resolution"] = "pack_fallback"
		plan.PackResolutionResult = result
		return plan
	}

	if degradedRecipes, degradedReason := degradedBuildFallback(job.Name, intents); len(degradedRecipes) > 0 {
		plan.RemediationTier = remediationTierFeatureDegraded
		plan.Recipes = mergeRecipes(plan.Recipes, degradedRecipes)
		plan.DegradedReason = degradedReason
		result["degraded_reason"] = degradedReason
		result["degraded_recipes"] = dedupeStrings(degradedRecipes)
		plan.PackResolutionResult = result
		return plan
	}

	plan.RemediationTier = remediationTierBlocked
	result["resolution"] = "blocked"
	plan.PackResolutionResult = result
	return plan
}

func inferDependencyIntents(missingPackages []string, logContent string, recipes []string) []string {
	intents := map[string]bool{}
	addIntent := func(intent string) {
		if strings.TrimSpace(intent) != "" {
			intents[intent] = true
		}
	}
	lowerLog := strings.ToLower(logContent)
	for _, pkg := range missingPackages {
		lpkg := strings.ToLower(strings.TrimSpace(pkg))
		switch {
		case strings.Contains(lpkg, "openblas"), strings.Contains(lpkg, "lapack"), strings.Contains(lpkg, "blas"):
			addIntent("blas_lapack")
		case strings.Contains(lpkg, "pkgconf"), strings.Contains(lpkg, "pkg-config"):
			addIntent("pkgconf")
		case strings.Contains(lpkg, "cmake"):
			addIntent("cmake")
		case strings.Contains(lpkg, "ninja"):
			addIntent("ninja")
		}
	}
	if strings.Contains(lowerLog, "dependency \"openblas\" not found") ||
		strings.Contains(lowerLog, "openblas") ||
		strings.Contains(lowerLog, "lapack") ||
		strings.Contains(lowerLog, "blas") {
		addIntent("blas_lapack")
	}
	for _, recipe := range recipes {
		lrecipe := strings.ToLower(strings.TrimSpace(recipe))
		switch {
		case strings.Contains(lrecipe, "openblas"), strings.Contains(lrecipe, "lapack"), strings.Contains(lrecipe, "blas"):
			addIntent("blas_lapack")
		}
	}
	if len(intents) == 0 {
		return nil
	}
	out := make([]string, 0, len(intents))
	for intent := range intents {
		out = append(out, intent)
	}
	sort.Strings(out)
	return out
}

func normalizedAlternativeRecipes(profile string, intents []string, unavailable map[string]bool) []string {
	var out []string
	for _, intent := range intents {
		switch intent {
		case "pkgconf":
			out = append(out, "dnf:pkgconf", "apt:pkg-config")
		case "cmake":
			out = append(out, "dnf:cmake", "apt:cmake")
		case "ninja":
			out = append(out, "dnf:ninja-build", "apt:ninja-build")
		case "blas_lapack":
			// The current default/native-heavy builders are UBI based; if the
			// desired RPM names are unavailable, fall through to dependency-pack
			// fallback rather than inventing inert apt recipes.
			switch normalizeBuilderProfile(profile) {
			case builderProfileDefault, builderProfileNativeHeavy:
				// no repo alternative
			}
		}
	}
	filtered := make([]string, 0, len(out))
	for _, recipe := range dedupeStrings(out) {
		parts := strings.SplitN(recipe, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[1]))
		if unavailable[key] || unavailable[strings.ToLower(recipe)] {
			continue
		}
		filtered = append(filtered, recipe)
	}
	return dedupeStrings(filtered)
}

func dependencyPackFallbacks(intents []string, catalog *pack.Catalog) []string {
	if catalog == nil || len(intents) == 0 {
		return nil
	}
	packs := map[string]bool{}
	for _, intent := range intents {
		switch intent {
		case "blas_lapack":
			if _, ok := catalog.GetPack("openblas"); ok {
				packs["openblas"] = true
			}
		case "cmake":
			if _, ok := catalog.GetPack("cmake"); ok {
				packs["cmake"] = true
			}
		case "ninja":
			if _, ok := catalog.GetPack("ninja"); ok {
				packs["ninja"] = true
			}
		case "pkgconf":
			if _, ok := catalog.GetPack("pkgconf"); ok {
				packs["pkgconf"] = true
			}
		}
	}
	if len(packs) == 0 {
		return nil
	}
	out := make([]string, 0, len(packs))
	for name := range packs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func shouldPreferPackFallback(profile string, intents []string) bool {
	if normalizeBuilderProfile(profile) != builderProfileNativeHeavy {
		return false
	}
	for _, intent := range intents {
		switch strings.TrimSpace(intent) {
		case "blas_lapack":
			return true
		}
	}
	return false
}

func stripDependencyRecipesForPackFallback(recipes []string, intents []string) []string {
	if len(recipes) == 0 || len(intents) == 0 {
		return dedupeStrings(recipes)
	}
	dropBLAS := false
	for _, intent := range intents {
		if strings.TrimSpace(intent) == "blas_lapack" {
			dropBLAS = true
			break
		}
	}
	if !dropBLAS {
		return dedupeStrings(recipes)
	}
	out := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		trimmed := strings.TrimSpace(recipe)
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "openblas") || strings.Contains(lower, "lapack") || strings.Contains(lower, "blas") {
			continue
		}
		out = append(out, trimmed)
	}
	return dedupeStrings(out)
}

func degradedBuildFallback(pkg string, intents []string) ([]string, string) {
	_ = pkg
	_ = intents
	return nil, ""
}

func sortKeysWithPrefixTrim(in map[string]bool) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for key := range in {
		key = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(key), "dnf:"))
		key = strings.TrimSpace(strings.TrimPrefix(key, "apt:"))
		if key == "" || strings.Contains(key, ":") || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func packArtifactID(def pack.PackDef) artifact.ID {
	key := artifact.PackKey{
		Arch:             "s390x",
		PolicyBaseDigest: "",
		Name:             def.Name,
		Version:          def.Version,
		RecipeDigest:     def.RecipeDigest,
	}
	return artifact.ID{Type: artifact.PackType, Digest: key.Digest()}
}

func (w *Worker) expandPackRequirements(names []string) ([]artifact.ID, map[string]string, map[string]map[string]any, []string) {
	if len(names) == 0 || w.Cfg.PackCatalog == nil {
		return nil, nil, nil, nil
	}
	seen := map[string]bool{}
	var ordered []artifact.ID
	actions := map[string]string{}
	meta := map[string]map[string]any{}
	var unresolved []string
	var visit func(string)
	visit = func(name string) {
		def, ok := w.Cfg.PackCatalog.GetPack(name)
		if !ok {
			unresolved = append(unresolved, name)
			return
		}
		id := packArtifactID(def)
		if seen[id.Digest] {
			return
		}
		seen[id.Digest] = true
		for _, dep := range def.Dependencies {
			visit(dep)
		}
		ordered = append(ordered, id)
		actions[id.Digest] = "build"
		meta[id.Digest] = map[string]any{
			"name":         def.Name,
			"version":      def.Version,
			"recipe":       def.Recipe,
			"dependencies": def.Dependencies,
			"source":       "dependency_pack_fallback",
		}
	}
	for _, name := range dedupeStrings(names) {
		visit(name)
	}
	return ordered, actions, meta, dedupeStrings(unresolved)
}

func mergePackIDs(existing, additional []artifact.ID) []artifact.ID {
	if len(existing) == 0 && len(additional) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]artifact.ID, 0, len(existing)+len(additional))
	for _, id := range append(append([]artifact.ID(nil), existing...), additional...) {
		if id.Digest == "" || seen[id.Digest] {
			continue
		}
		seen[id.Digest] = true
		out = append(out, id)
	}
	return out
}

func effectivePackMounts(packNames []string, packPaths []string) []string {
	if len(packPaths) == 0 {
		return nil
	}
	out := make([]string, 0, len(packPaths))
	for i, path := range packPaths {
		label := path
		if i < len(packNames) && strings.TrimSpace(packNames[i]) != "" {
			label = fmt.Sprintf("%s:%s", packNames[i], path)
		}
		out = append(out, label)
	}
	return out
}
