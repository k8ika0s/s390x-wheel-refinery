package service

import (
	"regexp"
	"strings"
)

var summaryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)fatal error:`),
	regexp.MustCompile(`(?i)moduleNotFoundError`),
	regexp.MustCompile(`(?i)no such file or directory`),
	regexp.MustCompile(`(?i)command not found`),
	regexp.MustCompile(`(?i)could not build wheels`),
	regexp.MustCompile(`(?i)exception`),
	regexp.MustCompile(`(?i)error:`),
	regexp.MustCompile(`(?i)failed`),
}

func summarizeLog(logContent string) string {
	if strings.TrimSpace(logContent) == "" {
		return ""
	}
	tail := tailLogLines(logContent, 200)
	lines := strings.Split(tail, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if isNoiseLine(line) {
			continue
		}
		if matchesAny(line, summaryPatterns) {
			return trimSummary(line)
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || isNoiseLine(line) {
			continue
		}
		return trimSummary(line)
	}
	return ""
}

func matchesAny(line string, patterns []*regexp.Regexp) bool {
	for _, re := range patterns {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func isNoiseLine(line string) bool {
	noise := []string{
		"running setup.py",
		"building wheel",
		"installing build dependencies",
		"collecting",
		"copying",
		"writing",
	}
	lower := strings.ToLower(line)
	for _, token := range noise {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}

func trimSummary(line string) string {
	const maxLen = 240
	if len(line) <= maxLen {
		return line
	}
	return line[:maxLen] + "..."
}
