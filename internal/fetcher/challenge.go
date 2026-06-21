package fetcher

import (
	"net/http"
	"strings"
)

func DetectChallenge(status int, headers http.Header, body string) ChallengeInfo {
	matched := detectChallengeMatches(status, headers, body)
	if len(matched) == 0 {
		return ChallengeInfo{}
	}
	return ChallengeInfo{
		Detected: true,
		Reason:   "cloudflare_or_antibot_challenge",
		Matched:  matched,
	}
}

func detectChallengeMatches(status int, headers http.Header, body string) []string {
	server := strings.ToLower(headers.Get("server"))
	lowerBody := strings.ToLower(body)
	matched := []string{}

	if strings.EqualFold(headers.Get("cf-mitigated"), "challenge") {
		matched = append(matched, "header: cf-mitigated=challenge")
	}
	if status == http.StatusForbidden && strings.Contains(server, "cloudflare") {
		matched = append(matched, "status/server: 403 cloudflare")
	}
	if status == http.StatusTooManyRequests {
		matched = append(matched, "status: 429")
	}
	if status == http.StatusServiceUnavailable && strings.Contains(server, "cloudflare") {
		matched = append(matched, "status/server: 503 cloudflare")
	}

	bodyChecks := []struct {
		needle string
		label  string
	}{
		{"just a moment", "body: Just a moment"},
		{"enable javascript and cookies to continue", "body: Enable JavaScript and cookies to continue"},
		{"cf-challenge-running", "body: cf-challenge-running"},
		{"cf-please-wait", "body: cf-please-wait"},
		{"__cf_chl_tk", "body: __cf_chl_tk"},
		{"ddos-guard", "body: ddos-guard"},
		{"access denied", "body: access denied"},
		{"人机验证", "body: 人机验证"},
		{"访问验证", "body: 访问验证"},
	}
	for _, check := range bodyChecks {
		if strings.Contains(lowerBody, check.needle) {
			matched = append(matched, check.label)
		}
	}

	hasChallengePlatform := strings.Contains(lowerBody, "challenge-platform")
	hasActiveCloudflareChallenge := strings.Contains(lowerBody, "_cf_chl_opt") ||
		strings.Contains(lowerBody, "/cdn-cgi/challenge-platform/h/")
	if hasChallengePlatform && hasActiveCloudflareChallenge {
		matched = append(matched, "body: active Cloudflare challenge-platform")
	}

	return matched
}
