package appstoreconnect

import (
	"errors"
	"net/http"
)

func clientWithRedirectPolicy(base *http.Client, destinationAllowed func(*http.Request) bool, message string) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	clone := *base
	prior := clone.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !destinationAllowed(req) {
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
