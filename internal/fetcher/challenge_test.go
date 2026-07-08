package fetcher

import (
	"net/http"
	"testing"
)

func TestDetectChallengeIgnoresCloudflareAnalyticsJSD(t *testing.T) {
	body := `<html><head><title>「PRED-886」の検索結果</title></head>
<body>
<a class="font-bold" href="/works/premium:PRED-886">real result</a>
<script>window.__CF$cv$params={};var a=document.createElement('script');a.src='/cdn-cgi/challenge-platform/scripts/jsd/main.js';</script>
</body></html>`

	info := DetectChallenge(http.StatusOK, http.Header{}, body)
	if info.Detected {
		t.Fatalf("expected analytics jsd page not to be treated as challenge, got: %+v", info)
	}
}

func TestDetectChallengeIgnoresPassiveChallengePlatformBeacon(t *testing.T) {
	body := `<html><head><title>高清中文字幕</title></head>
<body>
<table><tbody id="normalthread_1"><tr><a class="xst">thread</a></tr></tbody></table>
<script src="/cdn-cgi/challenge-platform/h/b/scripts/jsd/abc123/main.js"></script>
</body></html>`

	info := DetectChallenge(http.StatusOK, http.Header{}, body)
	if info.Detected {
		t.Fatalf("expected passive challenge-platform beacon not to be treated as challenge, got: %+v", info)
	}
}

func TestDetectChallengeFindsActiveCloudflareChallenge(t *testing.T) {
	body := `<html><head><title>Just a moment...</title></head>
<body><script>window._cf_chl_opt={};</script><script src="/cdn-cgi/challenge-platform/h/b/orchestrate/chl_page/v1"></script></body></html>`

	info := DetectChallenge(http.StatusForbidden, http.Header{"Server": {"cloudflare"}}, body)
	if !info.Detected {
		t.Fatal("expected active challenge to be detected")
	}
}
