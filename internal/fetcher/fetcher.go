package fetcher

import (
	"context"
	"fmt"
)

const ageGateMaxAttempts = 1

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
	if postChallenge.Detected || !DetectAgeVerification(bypassResp.Body) {
		return &Result{Response: bypassResp, Challenge: postChallenge}, nil
	}

	gated := bypassResp
	for attempt := 0; attempt < ageGateMaxAttempts; attempt++ {
		safeID := ExtractSafeID(gated.Body)
		if safeID == "" {
			break
		}
		retryOpts := opts
		retryOpts.Cookie = mergeCookieHeaders("_safe="+safeID, mergeCookieHeaders(opts.Cookie, gated.Cookies))
		gateResp, err := FetchFlareSolverr(ctx, req, retryOpts)
		if err != nil {
			break
		}
		gateChallenge := DetectChallenge(gateResp.Status, gateResp.Headers, gateResp.Body)
		if gateChallenge.Detected || !DetectAgeVerification(gateResp.Body) {
			return &Result{Response: gateResp, Challenge: gateChallenge}, nil
		}
		gated = gateResp
	}
	return &Result{Response: gated, Challenge: ChallengeInfo{}}, nil
}
