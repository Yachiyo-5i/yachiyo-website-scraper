package fetcher

import (
	"net/http"
	"time"
)

type ChallengeMode string

const (
	ChallengeDetect ChallengeMode = "detect"
	ChallengeBypass ChallengeMode = "bypass"
	ChallengeOff    ChallengeMode = "off"
)

type Channel string

const (
	ChannelHTTP        Channel = "http"
	ChannelFlareSolver Channel = "flaresolverr"
)

type RuntimeOptions struct {
	Timeout          time.Duration
	Cookie           string
	Challenge        ChallengeMode
	FlareSolverrURL  string
	FlareSolverrWait time.Duration
	Debug            bool
}

type Request struct {
	Method  string
	URL     string
	Headers map[string]string
}

type Response struct {
	Status   int
	FinalURL string
	Headers  http.Header
	Body     string
	Channel  Channel
}

type ChallengeInfo struct {
	Detected bool     `json:"detected"`
	Reason   string   `json:"reason,omitempty"`
	Matched  []string `json:"matched,omitempty"`
}

func DefaultRuntimeOptions() RuntimeOptions {
	return RuntimeOptions{
		Timeout:          30 * time.Second,
		Challenge:        ChallengeDetect,
		FlareSolverrWait: 60 * time.Second,
	}
}
