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

type playwrightRequest struct {
	Method    string            `json:"method,omitempty"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	Cookies   string            `json:"cookies,omitempty"`
	Timeout   int               `json:"timeout,omitempty"`
	Autoclick *AutoclickConfig  `json:"autoclick,omitempty"`
}

type playwrightResponse struct {
	Status   int               `json:"status"`
	FinalURL string            `json:"final_url"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     string            `json:"body"`
	Error    string            `json:"error,omitempty"`
}

func FetchPlaywright(ctx context.Context, req Request, opts RuntimeOptions) (*Response, error) {
	if strings.TrimSpace(opts.PlaywrightURL) == "" {
		return nil, fmt.Errorf("playwright url is required")
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	payload := playwrightRequest{
		Method:    method,
		URL:       req.URL,
		Headers:   req.Headers,
		Cookies:   opts.Cookie,
		Timeout:   int(opts.PlaywrightWait.Milliseconds()),
		Autoclick: opts.Autoclick,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: opts.PlaywrightWait}
	endpoint := strings.TrimRight(opts.PlaywrightURL, "/") + "/fetch"
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
		return nil, fmt.Errorf("playwright returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var decoded playwrightResponse
	if err := json.Unmarshal(respBytes, &decoded); err != nil {
		return nil, err
	}
	if decoded.Error != "" {
		return nil, fmt.Errorf("playwright error: %s", decoded.Error)
	}

	status := decoded.Status
	if status == 0 {
		status = http.StatusOK
	}
	finalURL := decoded.FinalURL
	if finalURL == "" {
		finalURL = req.URL
	}
	headers := http.Header{}
	for key, value := range decoded.Headers {
		headers.Set(key, value)
	}
	return &Response{
		Status:   status,
		FinalURL: finalURL,
		Headers:  headers,
		Body:     decoded.Body,
		Channel:  ChannelPlaywright,
	}, nil
}
