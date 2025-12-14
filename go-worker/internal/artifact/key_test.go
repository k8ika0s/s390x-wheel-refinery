package artifact

import "testing"

func TestRuntimeKeyDigestDeterministic(t *testing.T) {
	k := RuntimeKey{
		Arch:             "s390x",
		PolicyBaseDigest: "sha256:base",
		PythonVersion:    "3.11.4",
		BuildFlags:       "lto",
		ToolchainID:      "gcc-12",
		DepsHash:         "sha256:deps",
	}
	if k.Digest() != k.Digest() {
		t.Fatalf("runtime digest not deterministic")
	}
}

func TestPackKeyDigestChanges(t *testing.T) {
	k1 := PackKey{Arch: "s390x", PolicyBaseDigest: "sha256:base", Name: "openssl", Version: "3.2"}
	k2 := PackKey{Arch: "s390x", PolicyBaseDigest: "sha256:base", Name: "openssl", Version: "3.3"}
	if k1.Digest() == k2.Digest() {
		t.Fatalf("pack digest should differ when version changes")
	}
}

func TestWheelKeyDigestSortsPacks(t *testing.T) {
	k1 := WheelKey{
		SourceDigest:  "sha256:src",
		PyTag:         "cp311",
		PlatformTag:   "manylinux2014_s390x",
		RuntimeDigest: "sha256:rt",
		PackDigests:   []string{"b", "a"},
	}
	k2 := WheelKey{
		SourceDigest:  "sha256:src",
		PyTag:         "cp311",
		PlatformTag:   "manylinux2014_s390x",
		RuntimeDigest: "sha256:rt",
		PackDigests:   []string{"a", "b"},
	}
	if k1.Digest() != k2.Digest() {
		t.Fatalf("wheel digest should ignore pack order")
	}
}

func TestRepairKeyDigestChanges(t *testing.T) {
	k1 := RepairKey{InputWheelDigest: "sha256:wheel", RepairToolVersion: "audit1", PolicyRulesDigest: "sha256:rules1"}
	k2 := RepairKey{InputWheelDigest: "sha256:wheel", RepairToolVersion: "audit2", PolicyRulesDigest: "sha256:rules1"}
	if k1.Digest() == k2.Digest() {
		t.Fatalf("repair digest should differ when tool version changes")
	}
}
