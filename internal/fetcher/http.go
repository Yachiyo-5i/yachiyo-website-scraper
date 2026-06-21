package fetcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/net/html/charset"
)

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func FetchHTTP(ctx context.Context, req Request, opts RuntimeOptions) (*Response, error) {
	client := &http.Client{Timeout: opts.Timeout}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", DefaultUserAgent)
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if opts.Cookie != "" {
		httpReq.Header.Set("Cookie", opts.Cookie)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	decoded, err := decodeBody(body, resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, err
	}

	return &Response{
		Status:   resp.StatusCode,
		FinalURL: resp.Request.URL.String(),
		Headers:  resp.Header.Clone(),
		Body:     decoded,
		Channel:  ChannelHTTP,
	}, nil
}

func decodeBody(body []byte, contentType string) (string, error) {
	reader, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return string(body), nil
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("decode response body: %w", err)
	}
	return string(decoded), nil
}
