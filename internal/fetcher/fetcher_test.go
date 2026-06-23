package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultRuntimeOptions(t *testing.T) {
	opts := DefaultRuntimeOptions()
	if opts.Timeout != 30*time.Second {
		t.Fatalf("unexpected default timeout: %s", opts.Timeout)
	}
	if opts.Challenge != ChallengeDetect {
		t.Fatalf("unexpected default challenge mode: %s", opts.Challenge)
	}
	if opts.FlareSolverrWait != 60*time.Second {
		t.Fatalf("unexpected default FlareSolverr wait: %s", opts.FlareSolverrWait)
	}
	if opts.PlaywrightWait != 60*time.Second {
		t.Fatalf("unexpected Playwright wait: %s", opts.PlaywrightWait)
	}
}

func TestFetchHTTPAppliesHeadersCookieAndDecodesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("User-Agent"); got != "CustomAgent" {
			t.Fatalf("unexpected user agent: %q", got)
		}
		if got := r.Header.Get("X-Test"); got != "yes" {
			t.Fatalf("unexpected X-Test header: %q", got)
		}
		if got := r.Header.Get("Cookie"); got != "a=1; b=2" {
			t.Fatalf("unexpected cookie: %q", got)
		}
		w.Header().Set("Content-Type", "text/plain; charset=iso-8859-1")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte{0xe9})
	}))
	defer server.Close()

	resp, err := FetchHTTP(context.Background(), Request{
		Method: http.MethodPost,
		URL:    server.URL + "/latin",
		Headers: map[string]string{
			"User-Agent": "CustomAgent",
			"X-Test":     "yes",
		},
	}, RuntimeOptions{
		Timeout: time.Second,
		Cookie:  "a=1; b=2",
	})
	if err != nil {
		t.Fatal(err)
	}

	if resp.Status != http.StatusAccepted {
		t.Fatalf("unexpected status: %d", resp.Status)
	}
	if resp.FinalURL != server.URL+"/latin" {
		t.Fatalf("unexpected final URL: %q", resp.FinalURL)
	}
	if resp.Body != "é" {
		t.Fatalf("expected decoded ISO-8859-1 body, got %q", resp.Body)
	}
	if resp.Channel != ChannelHTTP {
		t.Fatalf("unexpected channel: %s", resp.Channel)
	}
}

func TestFetchHTTPFallsBackWhenCharsetIsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=definitely-unknown")
		w.Write([]byte("plain body"))
	}))
	defer server.Close()

	resp, err := FetchHTTP(context.Background(), Request{
		Method: http.MethodGet,
		URL:    server.URL,
	}, RuntimeOptions{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Body != "plain body" {
		t.Fatalf("unexpected fallback body: %q", resp.Body)
	}
}

func TestFetchHTTPRejectsInvalidRequestURL(t *testing.T) {
	_, err := FetchHTTP(context.Background(), Request{
		Method: http.MethodGet,
		URL:    "http://%",
	}, RuntimeOptions{})
	if err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestFetchHonorsChallengeOff(t *testing.T) {
	server := challengeServer(t)
	defer server.Close()

	result, err := Fetch(context.Background(), Request{
		Method: http.MethodGet,
		URL:    server.URL,
	}, RuntimeOptions{
		Timeout:   time.Second,
		Challenge: ChallengeOff,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Challenge.Detected {
		t.Fatalf("challenge detection should be skipped, got %+v", result.Challenge)
	}
	if result.Response.Status != http.StatusForbidden {
		t.Fatalf("unexpected response status: %d", result.Response.Status)
	}
}

func TestFetchDetectsChallengeWithoutBypass(t *testing.T) {
	server := challengeServer(t)
	defer server.Close()

	result, err := Fetch(context.Background(), Request{
		Method: http.MethodGet,
		URL:    server.URL,
	}, RuntimeOptions{
		Timeout:   time.Second,
		Challenge: ChallengeDetect,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Challenge.Detected {
		t.Fatal("expected challenge to be detected")
	}
	if result.Response.Channel != ChannelHTTP {
		t.Fatalf("unexpected response channel: %s", result.Response.Channel)
	}
}

func TestFetchBypassRequiresFlareSolverrURL(t *testing.T) {
	server := challengeServer(t)
	defer server.Close()

	result, err := Fetch(context.Background(), Request{
		Method: http.MethodGet,
		URL:    server.URL,
	}, RuntimeOptions{
		Timeout:   time.Second,
		Challenge: ChallengeBypass,
	})
	if err == nil {
		t.Fatal("expected missing FlareSolverr URL error")
	}
	if result == nil || !result.Challenge.Detected {
		t.Fatalf("expected challenge result with error, got result=%+v err=%v", result, err)
	}
}

func TestFetchBypassesChallengeWithFlareSolverr(t *testing.T) {
	target := challengeServer(t)
	defer target.Close()

	flaresolverr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload flaresolverrRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.URL != target.URL {
			t.Fatalf("unexpected FlareSolverr URL: %q", payload.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(flaresolverrResponse{
			Status: "ok",
			Solution: &flaresolverrSolution{
				URL:      target.URL + "/solved",
				Status:   http.StatusOK,
				Response: "<html>solved</html>",
			},
		})
	}))
	defer flaresolverr.Close()

	result, err := Fetch(context.Background(), Request{
		Method: http.MethodGet,
		URL:    target.URL,
	}, RuntimeOptions{
		Timeout:          time.Second,
		Challenge:        ChallengeBypass,
		FlareSolverrURL:  flaresolverr.URL,
		FlareSolverrWait: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Challenge.Detected {
		t.Fatalf("expected bypass response to be challenge-free, got %+v", result.Challenge)
	}
	if result.Response.Channel != ChannelFlareSolver || result.Response.Body != "<html>solved</html>" {
		t.Fatalf("unexpected bypass response: %+v", result.Response)
	}
}

func TestFetchBypassReturnsFlareSolverrErrorWithChallengeContext(t *testing.T) {
	target := challengeServer(t)
	defer target.Close()

	flaresolverr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("not available"))
	}))
	defer flaresolverr.Close()

	result, err := Fetch(context.Background(), Request{
		Method: http.MethodGet,
		URL:    target.URL,
	}, RuntimeOptions{
		Timeout:          time.Second,
		Challenge:        ChallengeBypass,
		FlareSolverrURL:  flaresolverr.URL,
		FlareSolverrWait: time.Second,
	})
	if err == nil {
		t.Fatal("expected FlareSolverr error")
	}
	if result == nil || !result.Challenge.Detected || result.Response.Channel != ChannelHTTP {
		t.Fatalf("expected original challenge context with error, got result=%+v err=%v", result, err)
	}
}

func TestFetchUsesPlaywrightWhenURLIsProvided(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ordinary HTTP fetch should be skipped when Playwright URL is provided")
	}))
	defer target.Close()

	playwright := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload playwrightRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.URL != target.URL || payload.Method != http.MethodGet {
			t.Fatalf("unexpected Playwright payload: %+v", payload)
		}
		if payload.Cookies != "a=1" {
			t.Fatalf("unexpected Playwright cookies: %q", payload.Cookies)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(playwrightResponse{
			Status:   http.StatusOK,
			FinalURL: target.URL + "/solved",
			Body:     "<html>solved</html>",
		})
	}))
	defer playwright.Close()

	result, err := Fetch(context.Background(), Request{
		Method: http.MethodGet,
		URL:    target.URL,
	}, RuntimeOptions{
		Timeout:        time.Second,
		Challenge:      ChallengeBypass,
		PlaywrightURL:  playwright.URL,
		PlaywrightWait: time.Second,
		Cookie:         "a=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Challenge.Detected {
		t.Fatalf("expected Playwright response to be challenge-free, got %+v", result.Challenge)
	}
	if result.Response.Channel != ChannelPlaywright || result.Response.Body != "<html>solved</html>" {
		t.Fatalf("unexpected Playwright response: %+v", result.Response)
	}
}

func TestFetchUsesPlaywrightDirectlyWhenURLIsProvided(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ordinary HTTP fetch should be skipped when Playwright URL is provided")
	}))
	defer target.Close()

	playwright := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(playwrightResponse{
			Status:   http.StatusOK,
			FinalURL: target.URL + "/final",
			Body:     "<html>browser body</html>",
		})
	}))
	defer playwright.Close()

	result, err := Fetch(context.Background(), Request{
		Method: http.MethodGet,
		URL:    target.URL,
	}, RuntimeOptions{
		Timeout:        time.Second,
		Challenge:      ChallengeDetect,
		PlaywrightURL:  playwright.URL,
		PlaywrightWait: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Response.Channel != ChannelPlaywright || result.Response.Body != "<html>browser body</html>" {
		t.Fatalf("unexpected Playwright direct response: %+v", result.Response)
	}
}

func TestFetchFlareSolverrSendsExpectedPayloadAndParsesSolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("unexpected content type: %q", got)
		}

		var payload flaresolverrRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Cmd != "request.get" || payload.URL != "https://example.test/page" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		if payload.MaxTimeout != 1500 {
			t.Fatalf("unexpected max timeout: %d", payload.MaxTimeout)
		}
		wantCookies := []flaresolverrCookie{{Name: "a", Value: "1"}, {Name: "spaced", Value: "ok"}}
		if !reflect.DeepEqual(payload.Cookies, wantCookies) {
			t.Fatalf("unexpected cookies:\nwant: %#v\n got: %#v", wantCookies, payload.Cookies)
		}

		json.NewEncoder(w).Encode(flaresolverrResponse{
			Status: "ok",
			Solution: &flaresolverrSolution{
				URL:      "https://example.test/final",
				Status:   http.StatusAccepted,
				Response: "<html>ok</html>",
			},
		})
	}))
	defer server.Close()

	resp, err := FetchFlareSolverr(context.Background(), Request{
		URL: "https://example.test/page",
	}, RuntimeOptions{
		FlareSolverrURL:  server.URL + "/",
		FlareSolverrWait: 1500 * time.Millisecond,
		Cookie:           "a=1; malformed; spaced = ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusAccepted || resp.FinalURL != "https://example.test/final" || resp.Body != "<html>ok</html>" {
		t.Fatalf("unexpected FlareSolverr response: %+v", resp)
	}
	if resp.Channel != ChannelFlareSolver {
		t.Fatalf("unexpected channel: %s", resp.Channel)
	}
}

func TestFetchFlareSolverrUsesDefaultsFromSolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(flaresolverrResponse{
			Status:   "ok",
			Solution: &flaresolverrSolution{Response: "body"},
		})
	}))
	defer server.Close()

	resp, err := FetchFlareSolverr(context.Background(), Request{
		URL: "https://example.test/page",
	}, RuntimeOptions{
		FlareSolverrURL:  server.URL,
		FlareSolverrWait: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusOK || resp.FinalURL != "https://example.test/page" {
		t.Fatalf("unexpected defaulted response: %+v", resp)
	}
}

func TestFetchFlareSolverrReportsErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "non-200",
			status:  http.StatusBadGateway,
			body:    "bad gateway",
			wantErr: "HTTP 502",
		},
		{
			name:    "invalid json",
			status:  http.StatusOK,
			body:    "{",
			wantErr: "unexpected end",
		},
		{
			name:    "failed status",
			status:  http.StatusOK,
			body:    `{"status":"error","message":"blocked"}`,
			wantErr: `status "error": blocked`,
		},
		{
			name:    "missing solution",
			status:  http.StatusOK,
			body:    `{"status":"ok"}`,
			wantErr: "no solution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := FetchFlareSolverr(context.Background(), Request{
				URL: "https://example.test/page",
			}, RuntimeOptions{
				FlareSolverrURL:  server.URL,
				FlareSolverrWait: time.Second,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestFetchFlareSolverrRequiresURL(t *testing.T) {
	_, err := FetchFlareSolverr(context.Background(), Request{URL: "https://example.test"}, RuntimeOptions{})
	if err == nil {
		t.Fatal("expected missing URL error")
	}
	if !strings.Contains(err.Error(), "flaresolverr url is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchPlaywrightSendsExpectedPayloadAndParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fetch" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload playwrightRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.URL != "https://example.test/page" || payload.Method != http.MethodPost {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		if payload.Timeout != 1500 {
			t.Fatalf("unexpected timeout: %d", payload.Timeout)
		}
		if payload.Cookies != "a=1; b=2" {
			t.Fatalf("unexpected cookies: %q", payload.Cookies)
		}
		if payload.Autoclick == nil || payload.Autoclick.XPath != "//button[contains(@class, 'confirm')]" {
			t.Fatalf("unexpected autoclick: %#v", payload.Autoclick)
		}
		if got := payload.Headers["X-Test"]; got != "yes" {
			t.Fatalf("unexpected headers: %#v", payload.Headers)
		}
		json.NewEncoder(w).Encode(playwrightResponse{
			Status:   http.StatusAccepted,
			FinalURL: "https://example.test/final",
			Body:     "<html>ok</html>",
			Headers:  map[string]string{"Content-Type": "text/html"},
		})
	}))
	defer server.Close()

	resp, err := FetchPlaywright(context.Background(), Request{
		Method: http.MethodPost,
		URL:    "https://example.test/page",
		Headers: map[string]string{
			"X-Test": "yes",
		},
	}, RuntimeOptions{
		PlaywrightURL:  server.URL + "/",
		PlaywrightWait: 1500 * time.Millisecond,
		Cookie:         "a=1; b=2",
		Autoclick:      &AutoclickConfig{XPath: "//button[contains(@class, 'confirm')]"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusAccepted || resp.FinalURL != "https://example.test/final" || resp.Body != "<html>ok</html>" {
		t.Fatalf("unexpected Playwright response: %+v", resp)
	}
	if resp.Channel != ChannelPlaywright {
		t.Fatalf("unexpected channel: %s", resp.Channel)
	}
	if resp.Headers.Get("Content-Type") != "text/html" {
		t.Fatalf("unexpected response headers: %#v", resp.Headers)
	}
}

func TestFetchPlaywrightReportsErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{
			name:    "non-200",
			status:  http.StatusBadGateway,
			body:    "bad gateway",
			wantErr: "HTTP 502",
		},
		{
			name:    "invalid json",
			status:  http.StatusOK,
			body:    "{",
			wantErr: "unexpected end",
		},
		{
			name:    "service error",
			status:  http.StatusOK,
			body:    `{"error":"blocked"}`,
			wantErr: "blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := FetchPlaywright(context.Background(), Request{
				URL: "https://example.test/page",
			}, RuntimeOptions{
				PlaywrightURL:  server.URL,
				PlaywrightWait: time.Second,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestFetchPlaywrightRequiresURL(t *testing.T) {
	_, err := FetchPlaywright(context.Background(), Request{URL: "https://example.test"}, RuntimeOptions{})
	if err == nil {
		t.Fatal("expected missing URL error")
	}
	if !strings.Contains(err.Error(), "playwright url is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchFlareSolverrRejectsInvalidEndpointURL(t *testing.T) {
	_, err := FetchFlareSolverr(context.Background(), Request{URL: "https://example.test"}, RuntimeOptions{
		FlareSolverrURL:  "http://%",
		FlareSolverrWait: time.Second,
	})
	if err == nil {
		t.Fatal("expected invalid endpoint URL error")
	}
	if !strings.Contains(err.Error(), "invalid URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchHTTPReturnsNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	_, err := FetchHTTP(context.Background(), Request{
		Method: http.MethodGet,
		URL:    url,
	}, RuntimeOptions{Timeout: time.Second})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestDetectChallengeMatchesStatusHeadersAndBody(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header http.Header
		body   string
		want   string
	}{
		{
			name:   "cf mitigated header",
			status: http.StatusOK,
			header: http.Header{"Cf-Mitigated": {"challenge"}},
			want:   "cf-mitigated",
		},
		{
			name:   "cloudflare service unavailable",
			status: http.StatusServiceUnavailable,
			header: http.Header{"Server": {"cloudflare"}},
			want:   "503 cloudflare",
		},
		{
			name:   "too many requests",
			status: http.StatusTooManyRequests,
			want:   "429",
		},
		{
			name: "body anti bot marker",
			body: "访问验证",
			want: "访问验证",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := DetectChallenge(tt.status, tt.header, tt.body)
			if !info.Detected {
				t.Fatal("expected challenge")
			}
			if !strings.Contains(strings.Join(info.Matched, ","), tt.want) {
				t.Fatalf("expected match containing %q, got %+v", tt.want, info.Matched)
			}
		})
	}
}

func TestParseCookieHeader(t *testing.T) {
	got := parseCookieHeader(" a = 1 ; broken ; b=two=parts ; empty= ")
	want := []flaresolverrCookie{
		{Name: "a", Value: "1"},
		{Name: "b", Value: "two=parts"},
		{Name: "empty", Value: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected parsed cookies:\nwant: %#v\n got: %#v", want, got)
	}
	if parseCookieHeader("   ") != nil {
		t.Fatal("blank cookie header should return nil")
	}
}

func challengeServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("cf-mitigated", "challenge")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("<html><title>Just a moment...</title>Enable JavaScript and cookies to continue</html>"))
	}))
}
