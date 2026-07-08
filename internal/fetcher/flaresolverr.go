package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var sehuatangSafeIDPattern = regexp.MustCompile(`(?i)safeid\s*=\s*['"]([^'"]+)['"]`)

type flaresolverrRequest struct {
	Cmd        string               `json:"cmd"`
	URL        string               `json:"url"`
	Cookies    []flaresolverrCookie `json:"cookies,omitempty"`
	MaxTimeout int                  `json:"maxTimeout"`
}

type flaresolverrCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type flaresolverrResponse struct {
	Status   string                `json:"status"`
	Message  string                `json:"message"`
	Solution *flaresolverrSolution `json:"solution"`
}

type flaresolverrSolution struct {
	URL      string `json:"url"`
	Status   int    `json:"status"`
	Response string `json:"response"`
}

func FetchFlareSolverr(ctx context.Context, req Request, opts RuntimeOptions) (*Response, error) {
	if strings.TrimSpace(opts.FlareSolverrURL) == "" {
		return nil, fmt.Errorf("flaresolverr url is required")
	}

	cookies := parseCookieHeader(opts.Cookie)
	payload := flaresolverrRequest{
		Cmd:        "request.get",
		URL:        req.URL,
		Cookies:    cookies,
		MaxTimeout: int(opts.FlareSolverrWait.Milliseconds()),
	}

	client := &http.Client{Timeout: opts.FlareSolverrWait}
	endpoint := strings.TrimRight(opts.FlareSolverrURL, "/") + "/v1"

	decoded, err := doFlareSolverrRequest(ctx, client, endpoint, payload)
	if err != nil {
		return nil, err
	}
	if safeID := sehuatangAgeGateSafeID(req.URL, decoded.Solution.Response); safeID != "" {
		payload.Cookies = withFlareSolverrCookie(payload.Cookies, flaresolverrCookie{
			Name:  "_safe",
			Value: safeID,
		})
		decoded, err = doFlareSolverrRequest(ctx, client, endpoint, payload)
		if err != nil {
			return nil, err
		}
	}

	status := decoded.Solution.Status
	if status == 0 {
		status = http.StatusOK
	}
	finalURL := decoded.Solution.URL
	if finalURL == "" {
		finalURL = req.URL
	}
	return &Response{
		Status:   status,
		FinalURL: finalURL,
		Headers:  http.Header{},
		Body:     decoded.Solution.Response,
		Channel:  ChannelFlareSolver,
	}, nil
}

func doFlareSolverrRequest(ctx context.Context, client *http.Client, endpoint string, payload flaresolverrRequest) (*flaresolverrResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("flaresolverr returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var decoded flaresolverrResponse
	if err := json.Unmarshal(respBytes, &decoded); err != nil {
		return nil, err
	}
	if decoded.Status != "ok" {
		return nil, fmt.Errorf("flaresolverr status %q: %s", decoded.Status, decoded.Message)
	}
	if decoded.Solution == nil {
		return nil, fmt.Errorf("flaresolverr returned no solution")
	}
	return &decoded, nil
}

func sehuatangAgeGateSafeID(rawURL, body string) string {
	if !isSehuatangURL(rawURL) {
		return ""
	}
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "safeid=") {
		return ""
	}
	if !strings.Contains(lower, "enter-btn") &&
		!strings.Contains(lower, "static/safe/") &&
		!strings.Contains(lower, "if you are over 18") {
		return ""
	}
	matches := sehuatangSafeIDPattern.FindStringSubmatch(body)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func isSehuatangURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "sehuatang.org" || strings.HasSuffix(host, ".sehuatang.org")
}

func withFlareSolverrCookie(cookies []flaresolverrCookie, cookie flaresolverrCookie) []flaresolverrCookie {
	next := make([]flaresolverrCookie, 0, len(cookies)+1)
	for _, existing := range cookies {
		if strings.EqualFold(existing.Name, cookie.Name) {
			continue
		}
		next = append(next, existing)
	}
	next = append(next, cookie)
	return next
}

func parseCookieHeader(cookie string) []flaresolverrCookie {
	if strings.TrimSpace(cookie) == "" {
		return nil
	}
	parts := strings.Split(cookie, ";")
	cookies := make([]flaresolverrCookie, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		cookies = append(cookies, flaresolverrCookie{
			Name:  strings.TrimSpace(name),
			Value: strings.TrimSpace(value),
		})
	}
	return cookies
}
