package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	logging "github.com/ipfs/go-log/v2"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	ucanhttp "github.com/storacha/go-ucanto/transport/http"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
)

var log = logging.Logger("server")

type config struct {
	id principal.Signer
}

type Option func(*config)

// WithIdentity specifies the server DID.
func WithIdentity(s principal.Signer) Option {
	return func(c *config) {
		c.id = s
	}
}

// ListenAndServe creates a new indexing service HTTP server, and starts it up.
func ListenAndServe(addr string, opts ...Option) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: NewServer(opts...),
	}
	log.Infof("Listening on %s", addr)
	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// NewServer creates a new indexing service HTTP server.
func NewServer(opts ...Option) *http.ServeMux {
	c := &config{}
	for _, opt := range opts {
		opt(c)
	}

	if c.id == nil {
		log.Warn("Generating a server identity as one has not been set!")
		id, err := ed25519.Generate()
		if err != nil {
			panic(err)
		}
		c.id = id
	}

	if s, ok := c.id.(signer.WrappedSigner); ok {
		log.Infof("Server ID: %s (%s)", s.DID(), s.Unwrap().DID())
	} else {
		log.Infof("Server ID: %s", c.id.DID())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", getRootHandler(c.id))
	mux.HandleFunc("POST /claims", postClaimsHandler(c.id))
	mux.HandleFunc("GET /claims/{multihash}", getClaimsHandler())
	return mux
}

// getRootHandler displays version info when a GET request is sent to "/".
func getRootHandler(id principal.Signer) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("🔥 indexing-service v0.0.0\n"))
		w.Write([]byte("- https://github.com/storacha/indexing-service\n"))
		if s, ok := id.(signer.WrappedSigner); ok {
			w.Write([]byte(fmt.Sprintf("- %s (%s)", s.DID(), s.Unwrap().DID())))
		} else {
			w.Write([]byte(fmt.Sprintf("- %s", id.DID())))
		}
	}
}

// postClaimsHandler invokes the ucanto service when a POST request is sent to
// "/claims".
func postClaimsHandler(id principal.Signer) func(http.ResponseWriter, *http.Request) {
	server, err := contentclaims.NewServer(id)
	if err != nil {
		log.Fatalf("creating ucanto server: %s", err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		res, _ := server.Request(ucanhttp.NewHTTPRequest(r.Body, r.Header))

		for key, vals := range res.Headers() {
			for _, v := range vals {
				w.Header().Add(key, v)
			}
		}

		if res.Status() != 0 {
			w.WriteHeader(res.Status())
		}

		io.Copy(w, res.Body())
	}
}

// getClaimsHandler retrieves content claims when a GET request is sent to
// "/claims/{multihash}".
func getClaimsHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		_, bytes, err := multibase.Decode(r.PathValue("multihash"))
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("invalid multibase encoding"))
			return
		}

		mh, err := multihash.Decode(bytes)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("invalid multihash"))
			return
		}

		// TODO: implement me
		// Just echo it back for now...
		enc, _ := multihash.Encode(mh.Digest, mh.Code)
		str, _ := multibase.Encode(multibase.Base58BTC, enc)

		w.Write([]byte(str))
	}
}
