package service

import (
	"regexp"
	"strings"
)

type failureReason struct {
	Code   string
	Detail string
}

var (
	moduleNotFoundRe   = regexp.MustCompile(`(?i)module(?:notfounderror)?: no module named ['"]([^'"]+)['"]`)
	missingHeaderRe    = regexp.MustCompile(`(?i)fatal error: ([^:\s]+): no such file or directory`)
	missingLibraryRe   = regexp.MustCompile(`(?i)cannot find -l([a-z0-9_+.\-]+)`)
	pkgConfigMissingRe = regexp.MustCompile(`(?i)no package ['"]?([^'"]+)['"]? found`)
	pkgUnavailableRe   = regexp.MustCompile(`(?i)(?:no match for argument|unable to find a match):\s*([a-z0-9_+.\-]+)`)
	cmakeMissingRe     = regexp.MustCompile(`(?i)could not find ([a-z0-9_+.\-]+)`)
	cmakeFailureRe     = regexp.MustCompile(`(?i)cmake error|cmake failed`)
	linkerErrorRe      = regexp.MustCompile(`(?i)undefined reference to|ld: cannot find|linker command failed|ld returned \d+ exit status|collect2: error`)
	cmdNotFoundRe      = regexp.MustCompile(`(?i)(?:^|\\s)([a-z0-9_+.\-]+): command not found`)
	gccVersionRe       = regexp.MustCompile(`(?i)requires gcc >=\s*([0-9.]+)`)
	builderImageRe     = regexp.MustCompile(`(?i)(image not known|manifest unknown|repository name not known to registry)`)
)

var compilerTools = map[string]bool{
	"gcc":     true,
	"g++":     true,
	"cc":      true,
	"clang":   true,
	"clang++": true,
}

// classifyFailureReason returns a standardized reason code and detail string.
func classifyFailureReason(err error, logText string) failureReason {
	text := strings.TrimSpace(logText)
	if err != nil {
		if text != "" {
			text = err.Error() + "\n" + text
		} else {
			text = err.Error()
		}
	}
	if text == "" {
		return failureReason{}
	}
	if strings.Contains(strings.ToLower(text), "unable to get the locale encoding") &&
		strings.Contains(strings.ToLower(text), "no module named 'encodings'") {
		return failureReason{Code: "runtime_stdlib_missing", Detail: "encodings"}
	}
	if strings.Contains(strings.ToLower(text), "runner: command exceeded timeout") ||
		strings.Contains(strings.ToLower(text), "status=error reason=timeout") ||
		strings.Contains(strings.ToLower(text), "podman run failed (timeout)") {
		return failureReason{Code: "build_timeout", Detail: "command_timeout"}
	}
	if builderImageRe.MatchString(text) {
		return failureReason{Code: "builder_image_missing", Detail: "refinery-builder"}
	}
	if match := gccVersionRe.FindStringSubmatch(text); len(match) > 1 {
		return failureReason{Code: "compiler_version_too_old", Detail: "gcc>=" + match[1]}
	}
	if match := moduleNotFoundRe.FindStringSubmatch(text); len(match) > 1 {
		return failureReason{Code: "missing_module", Detail: match[1]}
	}
	if match := missingHeaderRe.FindStringSubmatch(text); len(match) > 1 {
		return failureReason{Code: "missing_header", Detail: match[1]}
	}
	if match := missingLibraryRe.FindStringSubmatch(text); len(match) > 1 {
		return failureReason{Code: "missing_library", Detail: match[1]}
	}
	if match := pkgConfigMissingRe.FindStringSubmatch(text); len(match) > 1 {
		return failureReason{Code: "pkg_config_missing", Detail: match[1]}
	}
	if match := pkgUnavailableRe.FindStringSubmatch(text); len(match) > 1 {
		return failureReason{Code: "package_unavailable", Detail: match[1]}
	}
	if match := cmakeMissingRe.FindStringSubmatch(text); len(match) > 1 {
		return failureReason{Code: "cmake_missing", Detail: match[1]}
	}
	if cmakeFailureRe.MatchString(text) {
		return failureReason{Code: "cmake_failure", Detail: ""}
	}
	if linkerErrorRe.MatchString(text) {
		return failureReason{Code: "linker_error", Detail: ""}
	}
	if strings.Contains(strings.ToLower(text), "rust compiler not found") ||
		strings.Contains(strings.ToLower(text), "rustc: command not found") {
		return failureReason{Code: "rust_toolchain_missing", Detail: "rustc"}
	}
	if match := cmdNotFoundRe.FindStringSubmatch(text); len(match) > 1 {
		tool := strings.ToLower(match[1])
		if compilerTools[tool] {
			return failureReason{Code: "compiler_missing", Detail: tool}
		}
		return failureReason{Code: "build_tool_missing", Detail: tool}
	}
	return failureReason{}
}
