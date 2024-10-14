package publisher

import (
	"net/url"

	"github.com/multiformats/go-multiaddr"
)

// Option is an option configuring a publisher.
type Option func(cfg *options) error

type options struct {
	pubHTTPAnnounceAddrs []multiaddr.Multiaddr
	topic                string
	announceURLs         []*url.URL
}

// WithDirectAnnounce sets indexer URLs to send direct HTTP announcements to.
func WithDirectAnnounce(announceURLs ...string) Option {
	return func(o *options) error {
		for _, urlStr := range announceURLs {
			u, err := url.Parse(urlStr)
			if err != nil {
				return err
			}
			o.announceURLs = append(o.announceURLs, u)
		}
		return nil
	}
}

// WithAnnounceAddrs configures the multiaddr(s) of IPNI nodes that new
// advertisements should be announced to.
func WithAnnounceAddrs(addrs ...string) Option {
	return func(opts *options) error {
		for _, addr := range addrs {
			if addr != "" {
				maddr, err := multiaddr.NewMultiaddr(addr)
				if err != nil {
					return err
				}
				opts.pubHTTPAnnounceAddrs = append(opts.pubHTTPAnnounceAddrs, maddr)
			}
		}
		return nil
	}
}

func WithTopic(topic string) Option {
	return func(opts *options) error {
		opts.topic = topic
		return nil
	}
}
