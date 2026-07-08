package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
	URL      string               `json:"url"`
	Status   int                  `json:"status"`
	Response string               `json:"response"`
	Cookies  []flaresolverrCookie `json:"cookies"`
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
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: opts.FlareSolverrWait}
	endpoint := strings.TrimRight(opts.FlareSolverrURL, "/") + "/v1"
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
		Cookies:  buildCookieHeader(decoded.Solution.Cookies),
	}, nil
}

func buildCookieHeader(cookies []flaresolverrCookie) string {
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		name := strings.TrimSpace(cookie.Name)
		if name == "" {
			continue
		}
		parts = append(parts, name+"="+strings.TrimSpace(cookie.Value))
	}
	return strings.Join(parts, "; ")
}

func mergeCookieHeaders(base, extra string) string {
	order := make([]string, 0)
	values := make(map[string]string)
	for _, header := range []string{base, extra} {
		for _, part := range strings.Split(header, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name, value, ok := strings.Cut(part, "=")
			name = strings.TrimSpace(name)
			if !ok || name == "" {
				continue
			}
			if _, seen := values[name]; !seen {
				order = append(order, name)
			}
			values[name] = strings.TrimSpace(value)
		}
	}
	parts := make([]string, 0, len(order))
	for _, name := range order {
		parts = append(parts, name+"="+values[name])
	}
	return strings.Join(parts, "; ")
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
