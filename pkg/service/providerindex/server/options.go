package server

import (
	"fmt"
)

// config contains all options for configuring Publisher.
type config struct {
	handlerPath string
	httpAddr    string
}

// Option is a function that sets a value in a config.
type Option func(*config) error

// getPubOpts creates a pubConfig and applies Options to it.
func getOpts(opts []Option) (config, error) {
	cfg := config{}
	for i, opt := range opts {
		if err := opt(&cfg); err != nil {
			return config{}, fmt.Errorf("option %d failed: %s", i, err)
		}
	}
	return cfg, nil
}

// WithHTTPListenAddrs sets the HTTP addresse to listen on. These are in
// addresses:port format
//
// Setting HTTP listen addresses is optional when a stream host is provided by
// the WithStreamHost option.
func WithHTTPListenAddrs(addr string) Option {
	return func(c *config) error {
		c.httpAddr = addr
		return nil
	}
}

// WithHandlerPath sets the path used to handle requests to this publisher.
// This specifies the portion of the path before the implicit /ipni/v1/ad/ part
// of the path. Calling WithHandlerPath("/foo/bar") configures the publisher to
// handle HTTP requests on the path "/foo/bar/ipni/v1/ad/".
func WithHandlerPath(urlPath string) Option {
	return func(c *config) error {
		c.handlerPath = urlPath
		return nil
	}
}
