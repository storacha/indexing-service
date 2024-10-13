package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime/datamodel"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/delegation"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	ucanhttp "github.com/storacha/go-ucanto/transport/http"
	"github.com/storacha/indexing-service/pkg/service"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/service/queryresult"
)

var log = logging.Logger("server")

type Service interface {
	CacheClaim(ctx context.Context, claim delegation.Delegation) error
	PublishClaim(ctx context.Context, claim delegation.Delegation) error
	Query(ctx context.Context, q service.Query) (queryresult.QueryResult, error)
}

type config struct {
	id      principal.Signer
	service Service
}

type Option func(*config)

// WithIdentity specifies the server DID.
func WithIdentity(s principal.Signer) Option {
	return func(c *config) {
		c.id = s
	}
}

func WithService(service Service) Option {
	return func(c *config) {
		c.service = service
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
	mux.HandleFunc("GET /", GetRootHandler(c.id))
	mux.HandleFunc("POST /claims", PostClaimsHandler(c.id))
	mux.HandleFunc("GET /claims", GetClaimsHandler(c.service))
	return mux
}

// GetRootHandler displays version info when a GET request is sent to "/".
func GetRootHandler(id principal.Signer) func(http.ResponseWriter, *http.Request) {
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

// PostClaimsHandler invokes the ucanto service when a POST request is sent to
// "/claims".
func PostClaimsHandler(id principal.Signer) func(http.ResponseWriter, *http.Request) {
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

// GetClaimsHandler retrieves content claims when a GET request is sent to
// "/claims/{multihash}".
func GetClaimsHandler(s Service) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		mhStrings := r.URL.Query()["multihash"]
		hashes := make([]multihash.Multihash, 0, len(mhStrings))
		for _, mhString := range mhStrings {
			_, bytes, err := multibase.Decode(mhString)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid multibase encoding: %s", err.Error()), 400)
				return
			}
			hashes = append(hashes, bytes)
		}
		spaceStrings := r.URL.Query()["spaces"]
		spaces := make([]did.DID, 0, len(spaceStrings))
		for _, spaceString := range spaceStrings {
			space, err := did.Parse(spaceString)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid did: %s", err.Error()), 400)
				return
			}
			spaces = append(spaces, space)
		}

		qr, err := s.Query(r.Context(), service.Query{
			Hashes: hashes,
			Match: service.Match{
				Subject: spaces,
			},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("processing queury: %s", err.Error()), 400)
		}

		body := car.Encode([]datamodel.Link{qr.Root().Link()}, qr.Blocks())
		w.WriteHeader(http.StatusOK)
		io.Copy(w, body)
	}
}
