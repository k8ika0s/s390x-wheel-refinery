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
	cmakeMissingRe     = regexp.MustCompile(`(?i)could not find ([a-z0-9_+.\-]+)`)
	cmdNotFoundRe      = regexp.MustCompile(`(?i)(?:^|\\s)([a-z0-9_+.\-]+): command not found`)
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
	if match := cmakeMissingRe.FindStringSubmatch(text); len(match) > 1 {
		return failureReason{Code: "cmake_missing", Detail: match[1]}
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
