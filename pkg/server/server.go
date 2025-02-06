package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	"github.com/storacha/go-ucanto/server"
	ucanhttp "github.com/storacha/go-ucanto/transport/http"
	"github.com/storacha/indexing-service/pkg/build"
	"github.com/storacha/indexing-service/pkg/service/contentclaims"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/storacha/indexing-service/pkg/types"
)

var log = logging.Logger("server")

type config struct {
	id                   principal.Signer
	contentClaimsOptions []server.Option
}

type Option func(*config)

// WithIdentity specifies the server DID.
func WithIdentity(s principal.Signer) Option {
	return func(c *config) {
		c.id = s
	}
}

func WithContentClaimsOptions(options ...server.Option) Option {
	return func(c *config) {
		c.contentClaimsOptions = options
	}
}

// ListenAndServe creates a new indexing service HTTP server, and starts it up.
func ListenAndServe(addr string, indexer types.Service, opts ...Option) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: NewServer(indexer, opts...),
	}
	log.Infof("Listening on %s", addr)
	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// NewServer creates a new indexing service HTTP server.
func NewServer(indexer types.Service, opts ...Option) *http.ServeMux {
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
	mux.HandleFunc("GET /claim/{claim}", GetClaimHandler(indexer))
	mux.HandleFunc("POST /claims", PostClaimsHandler(c.id, indexer, c.contentClaimsOptions...))
	mux.HandleFunc("GET /claims", GetClaimsHandler(indexer))
	return mux
}

// GetRootHandler displays version info when a GET request is sent to "/".
func GetRootHandler(id principal.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fmt.Sprintf("🔥 indexing-service %s\n", build.Version)))
		w.Write([]byte("- https://github.com/storacha/indexing-service\n"))
		w.Write([]byte(fmt.Sprintf("- %s\n", id.DID())))
		if s, ok := id.(signer.WrappedSigner); ok {
			w.Write([]byte(fmt.Sprintf("- %s\n", s.Unwrap().DID())))
		}
	}
}

// GetClaimHandler retrieves a single content claim by it's root CID.
func GetClaimHandler(service types.Getter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		c, err := cid.Parse(parts[len(parts)-1])
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid CID: %s", err), http.StatusBadRequest)
			return
		}

		dlg, err := service.Get(r.Context(), cidlink.Link{Cid: c})
		if err != nil {
			if errors.Is(err, types.ErrKeyNotFound) {
				http.Error(w, fmt.Sprintf("not found: %s", c), http.StatusNotFound)
				return
			}
			log.Errorf("getting claim: %w", err)
			http.Error(w, "failed to get claim", http.StatusInternalServerError)
			return
		}

		_, err = io.Copy(w, dlg.Archive())
		if err != nil {
			log.Warnf("serving claim: %s: %w", c, err)
		}
	}
}

// PostClaimsHandler invokes the ucanto service when a POST request is sent to
// "/claims".
func PostClaimsHandler(id principal.Signer, service types.Publisher, options ...server.Option) http.HandlerFunc {
	server, err := contentclaims.NewUCANServer(id, service, options...)
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

		_, err := io.Copy(w, res.Body())
		if err != nil {
			log.Errorf("sending UCAN response: %w", err)
		}
	}
}

// GetClaimsHandler retrieves content claims when a GET request is sent to
// "/claims?multihash={multihash}".
func GetClaimsHandler(service types.Querier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, s := telemetry.StartSpan(r.Context(), "GetClaimsHandler")
		defer s.End()

		queryTypeParam := r.URL.Query()["type"]
		var queryType types.QueryType
		switch len(queryTypeParam) {
		case 0:
			queryType = types.QueryTypeStandard
		case 1:
			var err error
			queryType, err = types.ParseQueryType(queryTypeParam[0])
			if err != nil {
				http.Error(w, fmt.Sprint(err), http.StatusBadRequest)
				return
			}
		default:
			http.Error(w, fmt.Sprintf("only one 'type' parameter is allowed, but got %d", len(queryTypeParam)), http.StatusBadRequest)
			return
		}

		mhStrings := r.URL.Query()["multihash"]
		hashes := make([]multihash.Multihash, 0, len(mhStrings))
		for _, mhString := range mhStrings {
			_, bytes, err := multibase.Decode(mhString)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid multibase encoding: %s", err.Error()), http.StatusBadRequest)
				return
			}
			hashes = append(hashes, bytes)
		}
		if len(hashes) == 0 {
			http.Error(w, "missing digests", 400)
			return
		}

		spaceStrings := r.URL.Query()["spaces"]
		spaces := make([]did.DID, 0, len(spaceStrings))
		for _, spaceString := range spaceStrings {
			space, err := did.Parse(spaceString)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid did: %s", err.Error()), http.StatusBadRequest)
				return
			}
			spaces = append(spaces, space)
		}

		qr, err := service.Query(ctx, types.Query{
			Type:   queryType,
			Hashes: hashes,
			Match: types.Match{
				Subject: spaces,
			},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("processing query: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		body := car.Encode([]datamodel.Link{qr.Root().Link()}, qr.Blocks())
		w.WriteHeader(http.StatusOK)
		_, err = io.Copy(w, body)
		if err != nil {
			log.Errorf("sending claims response: %w", err)
		}
	}
}
