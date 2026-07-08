package fetcher

import (
	"context"
	"fmt"
)

type Result struct {
	Response  *Response
	Challenge ChallengeInfo
}

func Fetch(ctx context.Context, req Request, opts RuntimeOptions) (*Result, error) {
	if opts.PlaywrightURL != "" {
		resp, err := FetchPlaywright(ctx, req, opts)
		if err != nil {
			return nil, err
		}
		if opts.Challenge == ChallengeOff {
			return &Result{Response: resp}, nil
		}
		challenge := DetectChallenge(resp.Status, resp.Headers, resp.Body)
		if !challenge.Detected || opts.Challenge != ChallengeBypass || opts.FlareSolverrURL == "" {
			return &Result{Response: resp, Challenge: challenge}, nil
		}
		return bypassChallenge(ctx, req, opts, resp, challenge)
	}

	if opts.Challenge == ChallengeOff {
		resp, err := FetchHTTP(ctx, req, opts)
		if err != nil {
			return nil, err
		}
		return &Result{Response: resp}, nil
	}

	resp, err := FetchHTTP(ctx, req, opts)
	if err != nil {
		return nil, err
	}
	challenge := DetectChallenge(resp.Status, resp.Headers, resp.Body)
	if !challenge.Detected {
		return &Result{Response: resp}, nil
	}

	if opts.Challenge != ChallengeBypass {
		return &Result{Response: resp, Challenge: challenge}, nil
	}
	if opts.FlareSolverrURL == "" {
		return &Result{Response: resp, Challenge: challenge}, fmt.Errorf("challenge detected but --flaresolverr was not provided")
	}

	return bypassChallenge(ctx, req, opts, resp, challenge)
}

func bypassChallenge(ctx context.Context, req Request, opts RuntimeOptions, resp *Response, challenge ChallengeInfo) (*Result, error) {
	bypassResp, err := FetchFlareSolverr(ctx, req, opts)
	if err != nil {
		return &Result{Response: resp, Challenge: challenge}, err
	}
	postChallenge := DetectChallenge(bypassResp.Status, bypassResp.Headers, bypassResp.Body)
	if postChallenge.Detected || opts.PlaywrightURL == "" || !DetectAgeVerification(bypassResp.Body) {
		return &Result{Response: bypassResp, Challenge: postChallenge}, nil
	}

	retryOpts := opts
	retryOpts.Cookie = mergeCookieHeaders(opts.Cookie, bypassResp.Cookies)
	pwResp, err := FetchPlaywright(ctx, req, retryOpts)
	if err != nil {
		return &Result{Response: bypassResp, Challenge: postChallenge}, nil
	}
	pwChallenge := DetectChallenge(pwResp.Status, pwResp.Headers, pwResp.Body)
	return &Result{Response: pwResp, Challenge: pwChallenge}, nil
}
