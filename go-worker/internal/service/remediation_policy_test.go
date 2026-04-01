package service

import (
	"strings"
	"testing"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/runner"
)

func TestResolveRemediationPlanEscalatesToNativeHeavyOnTimeout(t *testing.T) {
	w := &Worker{}
	job := runner.Job{
		Name:           "scikit-learn",
		Version:        "1.5.2",
		BuilderProfile: builderProfileDefault,
		Recipes: []string{
			"dnf:gcc-toolset-12",
			"dnf:gcc-toolset-12-gcc",
			"env:CC=/opt/rh/gcc-toolset-12/root/usr/bin/gcc",
		},
	}
	logContent := strings.Join([]string{
		"Using cached scikit_learn-1.5.2.tar.gz (7.0 MB)",
		"runner: command exceeded timeout elapsed_ms=900128",
		"status=error reason=timeout elapsed_ms=900128",
	}, "\n")
	plan := w.resolveRemediationPlan(
		job,
		failureReason{Code: "build_timeout", Detail: "command_timeout"},
		job.Recipes,
		logContent,
	)
	if plan.BuilderProfile != builderProfileNativeHeavy {
		t.Fatalf("expected native-heavy escalation, got %q", plan.BuilderProfile)
	}
	if plan.RemediationTier != remediationTierRepoPackage {
		t.Fatalf("expected repo package tier to be preserved, got %q", plan.RemediationTier)
	}
}
