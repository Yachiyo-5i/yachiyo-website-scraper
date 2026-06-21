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

	bypassResp, err := FetchFlareSolverr(ctx, req, opts)
	if err != nil {
		return &Result{Response: resp, Challenge: challenge}, err
	}
	postChallenge := DetectChallenge(bypassResp.Status, bypassResp.Headers, bypassResp.Body)
	return &Result{Response: bypassResp, Challenge: postChallenge}, nil
}
