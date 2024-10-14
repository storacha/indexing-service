package server

import (
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/ipfs/go-cid"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/storacha/indexing-service/pkg/service/providerindex/publisher"
)

const (
	// IPNIPath is the path that the Publisher expects as the last port of the
	// HTTP request URL path. The sync client automatically adds this to the
	// request path.
	IPNIPath = "/ipni/v1/ad"
)

type Server struct {
	advertStore publisher.AdvertStore
	handlerPath string
	srv         *http.Server
}

func (s *Server) Start(ctx context.Context) error {
	go s.srv.ListenAndServe()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Close()
}

func NewServer(store publisher.AdvertStore, options ...Option) (*Server, error) {
	opts, err := getOpts(options)
	if err != nil {
		return nil, err
	}
	var handlerPath string
	opts.handlerPath = strings.TrimPrefix(opts.handlerPath, "/")
	if opts.handlerPath != "" {
		handlerPath = path.Join(opts.handlerPath, IPNIPath)
	} else {
		handlerPath = strings.TrimPrefix(IPNIPath, "/")
	}

	srv := &http.Server{
		Addr: opts.httpAddr,
	}

	s := &Server{advertStore: store, srv: srv, handlerPath: handlerPath}

	mux := http.NewServeMux()
	mux.Handle(handlerPath, s)
	s.srv.Handler = mux

	return s, nil

}

// ServeHTTP implements the http.Handler interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// If we expect publisher requests to have a prefix in the request path,
	// then check for the expected prefix.. This happens when using an external
	// server with this Publisher as the request handler.
	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	if s.handlerPath != "" {
		// A URL path from http will have a leading "/". A URL from libp2phttp will not.
		if !strings.HasPrefix(urlPath, s.handlerPath) {
			http.Error(w, "invalid request path: "+r.URL.Path, http.StatusBadRequest)
			return
		}
	} else if path.Dir(urlPath) != "." {
		http.Error(w, "invalid request path: "+r.URL.Path, http.StatusBadRequest)
		return
	}

	ask := path.Base(r.URL.Path)
	if ask == "head" {

		// Serve the head message.

		err := s.advertStore.EncodeHead(r.Context(), w)
		if err != nil {
			if publisher.IsNotFound(err) {
				http.Error(w, "", http.StatusNoContent)
				return
			}
			http.Error(w, "", http.StatusInternalServerError)
		}
		return
	}

	// Interpret `ask` as a CID to serve.
	c, err := cid.Parse(ask)
	if err != nil {
		http.Error(w, "invalid request: not a cid", http.StatusBadRequest)
		return
	}
	err = s.advertStore.Encode(r.Context(), cidlink.Link{Cid: c}, w)
	if err != nil {
		if publisher.IsNotFound(err) {
			http.Error(w, "cid not found", http.StatusNotFound)
			return
		}
		http.Error(w, "unable to load data for cid", http.StatusInternalServerError)
	}
}
