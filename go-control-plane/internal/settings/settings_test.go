package settings

import "testing"

func TestApplyDefaultsBoolsRespectFalse(t *testing.T) {
	// explicit false should remain false
	autoPlan := false
	autoBuild := false
	s := Settings{
		PythonVersion: "3.10",
		PlatformTag:   "tag",
		PollMs:        123,
		RecentLimit:   7,
		AutoPlan:      &autoPlan,
		AutoBuild:     &autoBuild,
	}
	out := ApplyDefaults(s)
	if out.PythonVersion != "3.10" || out.PlatformTag != "tag" || out.PollMs != 123 || out.RecentLimit != 7 {
		t.Fatalf("unexpected defaults override on provided fields: %+v", out)
	}
	if BoolValue(out.AutoPlan) != false || BoolValue(out.AutoBuild) != false {
		t.Fatalf("expected explicit false to persist: %+v", out)
	}
}

func TestApplyDefaultsSetsMissing(t *testing.T) {
	out := ApplyDefaults(Settings{})
	if out.PythonVersion == "" || out.PlatformTag == "" {
		t.Fatalf("expected defaults to populate versions/tags: %+v", out)
	}
	if BoolValue(out.AutoPlan) || BoolValue(out.AutoBuild) {
		t.Fatalf("expected missing bools to default false: %+v", out)
	}
	if out.PollMs == 0 || out.RecentLimit == 0 {
		t.Fatalf("expected numeric defaults to be set: %+v", out)
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(Settings{PythonVersion: "3.10", PlatformTag: "manylinux2014_s390x"}); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if err := Validate(Settings{PythonVersion: "2.7"}); err == nil {
		t.Fatalf("expected error for python 2.7")
	}
	if err := Validate(Settings{PlatformTag: "bad tag"}); err == nil {
		t.Fatalf("expected error for invalid platform tag")
	}
}
