package googleplay

import (
	"errors"
	"net/http"
	"time"
)

const defaultRequestTimeout = 60 * time.Second

func clientWithRedirectPolicy(base *http.Client, allowed func(*http.Request) bool, message string) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	clone := *base
	if clone.Timeout == 0 {
		clone.Timeout = defaultRequestTimeout
	}
	prior := clone.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !allowed(req) {
			return errors.New(message)
		}
		if prior != nil {
			return prior(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &clone
}
