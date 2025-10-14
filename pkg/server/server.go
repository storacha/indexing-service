package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log/v2"
	"github.com/ipld/go-ipld-prime"
	"github.com/ipld/go-ipld-prime/datamodel"
	cidlink "github.com/ipld/go-ipld-prime/linking/cid"
	"github.com/ipni/go-libipni/dagsync/ipnisync/head"
	"github.com/ipni/go-libipni/find/model"
	"github.com/ipni/go-libipni/ingest/schema"
	"github.com/ipni/go-libipni/metadata"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
	"github.com/storacha/go-libstoracha/capabilities/assert"
	"github.com/storacha/go-libstoracha/ipnipublisher/store"
	"github.com/storacha/go-ucanto/core/car"
	"github.com/storacha/go-ucanto/core/dag/blockstore"
	"github.com/storacha/go-ucanto/core/delegation"
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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var log = logging.Logger("server")

type ipniConfig struct {
	provider peer.AddrInfo
	metadata []byte
}

type config struct {
	id                   principal.Signer
	contentClaimsOptions []server.Option
	enableTelemetry      bool
	ipniConfig           *ipniConfig
}

type Option func(*config) error

// WithIdentity specifies the server DID.
func WithIdentity(s principal.Signer) Option {
	return func(c *config) error {
		c.id = s
		return nil
	}
}

func WithContentClaimsOptions(options ...server.Option) Option {
	return func(c *config) error {
		c.contentClaimsOptions = options
		return nil
	}
}

func WithTelemetry() Option {
	return func(c *config) error {
		c.enableTelemetry = true
		return nil
	}
}

func WithIPNI(provider peer.AddrInfo, metadata metadata.Metadata) Option {
	return func(c *config) error {
		mb, err := metadata.MarshalBinary()
		if err != nil {
			return err
		}
		c.ipniConfig = &ipniConfig{
			provider: provider,
			metadata: mb,
		}
		return nil
	}
}

// ListenAndServe creates a new indexing service HTTP server, and starts it up.
func ListenAndServe(addr string, indexer types.Service, opts ...Option) error {
	mux, err := NewServer(indexer, opts...)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	log.Infof("Listening on %s", addr)
	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// NewServer creates a new indexing service HTTP server.
func NewServer(indexer types.Service, opts ...Option) (*http.ServeMux, error) {
	c := &config{}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, err
		}
	}

	if c.id == nil {
		log.Warn("Generating a server identity as one has not been set!")
		id, err := ed25519.Generate()
		if err != nil {
			return nil, fmt.Errorf("generating identity: %w", err)
		}
		c.id = id
	}

	if s, ok := c.id.(signer.WrappedSigner); ok {
		log.Infof("Server ID: %s (%s)", s.DID(), s.Unwrap().DID())
	} else {
		log.Infof("Server ID: %s", c.id.DID())
	}

	mux := http.NewServeMux()
	maybeInstrumentAndAdd(mux, "GET /", GetRootHandler(c.id), c.enableTelemetry)
	maybeInstrumentAndAdd(mux, "GET /claim/{claim}", GetClaimHandler(indexer), c.enableTelemetry)
	// temporary fix: post claims handler accessible at POST / too
	maybeInstrumentAndAdd(mux, "POST /", PostClaimsHandler(c.id, indexer, c.contentClaimsOptions...), c.enableTelemetry)
	maybeInstrumentAndAdd(mux, "POST /claims", PostClaimsHandler(c.id, indexer, c.contentClaimsOptions...), c.enableTelemetry)
	maybeInstrumentAndAdd(mux, "GET /claims", GetClaimsHandler(indexer), c.enableTelemetry)
	maybeInstrumentAndAdd(mux, "GET /.well-known/did.json", GetDIDDocument(c.id), c.enableTelemetry)
	if c.ipniConfig != nil {
		maybeInstrumentAndAdd(mux, "GET /cid/{cid}", GetIPNICIDHandler(indexer, c.ipniConfig), c.enableTelemetry)
	}
	return mux, nil
}

func maybeInstrumentAndAdd(mux *http.ServeMux, route string, handler http.HandlerFunc, enableTelemetry bool) {
	if enableTelemetry {
		mux.Handle(route, otelhttp.NewHandler(handler, route, otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents)))
	} else {
		mux.HandleFunc(route, handler)
	}
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
			log.Errorf("getting claim: %s", err)
			http.Error(w, "failed to get claim", http.StatusInternalServerError)
			return
		}

		_, err = io.Copy(w, dlg.Archive())
		if err != nil {
			log.Warnf("serving claim: %s: %s", c, err)
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
		res, _ := server.Request(r.Context(), ucanhttp.NewRequest(r.Body, r.Header))

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
			log.Errorf("sending UCAN response: %s", err)
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
			log.Errorf("sending claims response: %s", err)
		}
	}
}

func GetIPNICIDHandler(service types.Querier, config *ipniConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, s := telemetry.StartSpan(r.Context(), "GetClaimsHandler")
		defer s.End()
		if config == nil {
			http.Error(w, "IPNI config not available", http.StatusInternalServerError)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		c, err := cid.Parse(parts[len(parts)-1])
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid CID: %s", err), http.StatusBadRequest)
			return
		}
		mh := c.Hash()
		qr, err := service.Query(ctx, types.Query{
			Type:   types.QueryTypeStandard,
			Hashes: []multihash.Multihash{mh},
			Match: types.Match{
				Subject: []did.DID{},
			},
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("processing query: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		if len(qr.Claims()) == 0 {
			http.Error(w, fmt.Sprintf("no claims found for CID: %s", c), http.StatusNotFound)
			return
		}
		blocks, err := blockstore.NewBlockReader(blockstore.WithBlocksIterator(qr.Blocks()))
		if err != nil {
			http.Error(w, fmt.Sprintf("reading blocks from query result: %s", err), http.StatusInternalServerError)
			return
		}

		// iterate over all claims to see if there are location claims, return preset peer if found
		for _, root := range qr.Claims() {
			claim, err := delegation.NewDelegationView(root, blocks)
			if err != nil {
				http.Error(w, fmt.Sprintf("decoding delegation: %s", err), http.StatusInternalServerError)
				return
			}

			switch claim.Capabilities()[0].Can() {
			case assert.LocationAbility:
				data, err := model.MarshalFindResponse(&model.FindResponse{
					MultihashResults: []model.MultihashResult{{
						Multihash: mh,
						ProviderResults: []model.ProviderResult{{
							ContextID: mh,
							Metadata:  config.metadata,
							Provider:  &config.provider,
						}},
					}},
				})

				if err != nil {
					http.Error(w, fmt.Sprintf("marshalling find response: %s", err), http.StatusInternalServerError)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err = w.Write(data)
				if err != nil {
					log.Errorf("sending find response: %s", err)
				}
				return
			}
		}
		http.Error(w, fmt.Sprintf("no claims found for CID: %s", c), http.StatusNotFound)
	}
}

// Document is a did document that describes a did subject.
// See https://www.w3.org/TR/did-core/#dfn-did-documents.
type Document struct {
	Context            []string             `json:"@context"` // https://w3id.org/did/v1
	ID                 string               `json:"id"`
	Controller         []string             `json:"controller,omitempty"`
	VerificationMethod []VerificationMethod `json:"verificationMethod,omitempty"`
	Authentication     []string             `json:"authentication,omitempty"`
	AssertionMethod    []string             `json:"assertionMethod,omitempty"`
}

// VerificationMethod describes how to authenticate or authorize interactions
// with a did subject.
// See https://www.w3.org/TR/did-core/#dfn-verification-method.
type VerificationMethod struct {
	ID                 string `json:"id,omitempty"`
	Type               string `json:"type,omitempty"`
	Controller         string `json:"controller,omitempty"`
	PublicKeyMultibase string `json:"publicKeyMultibase,omitempty"`
}

// GetDIDDocument returns the DID document for did:web resolution.
func GetDIDDocument(id principal.Signer) http.HandlerFunc {
	doc := Document{
		Context: []string{"https://w3id.org/did/v1"},
		ID:      id.DID().String(),
	}
	if s, ok := id.(signer.WrappedSigner); ok {
		vid := fmt.Sprintf("%s#owner", s.DID())
		doc.VerificationMethod = []VerificationMethod{
			{
				ID:                 vid,
				Type:               "Ed25519VerificationKey2020",
				Controller:         s.DID().String(),
				PublicKeyMultibase: strings.TrimPrefix(s.Unwrap().DID().String(), "did:key:"),
			},
		}
		doc.Authentication = []string{vid}
		doc.AssertionMethod = []string{vid}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		bytes, err := json.Marshal(doc)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write(bytes)
	}
}

func PostAdHandler(sk crypto.PrivKey, store store.PublisherStore) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ad, err := decodeAdvert(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("decoding advert: %s", err.Error()), http.StatusBadRequest)
			return
		}

		if err := validateAdvertSig(sk, ad); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		adlink, err := publishAdvert(r.Context(), sk, store, ad)
		if err != nil {
			http.Error(w, fmt.Sprintf("publishing advert: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		out, err := json.Marshal(adlink)
		if err != nil {
			http.Error(w, fmt.Sprintf("marshaling JSON: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		w.Write(out)
	})
}

// ensures the advert came from this node originally
func validateAdvertSig(sk crypto.PrivKey, ad schema.Advertisement) error {
	sigBytes := ad.Signature
	err := ad.Sign(sk)
	if err != nil {
		return fmt.Errorf("signing advert: %w", err)
	}
	if !bytes.Equal(sigBytes, ad.Signature) {
		return errors.New("advert was not created by this node")
	}
	return nil
}

// assumed in DAG-JSON encoding
func decodeAdvert(r io.Reader) (schema.Advertisement, error) {
	advBytes, err := io.ReadAll(r)
	if err != nil {
		return schema.Advertisement{}, err
	}

	adLink, err := cid.V1Builder{
		Codec:  cid.DagJSON,
		MhType: multihash.SHA2_256,
	}.Sum(advBytes)
	if err != nil {
		return schema.Advertisement{}, err
	}

	return schema.BytesToAdvertisement(adLink, advBytes)
}

func publishAdvert(ctx context.Context, sk crypto.PrivKey, store store.PublisherStore, ad schema.Advertisement) (ipld.Link, error) {
	prevHead, err := store.Head(ctx)
	if err != nil {
		return nil, err
	}

	ad.PreviousID = prevHead.Head

	if err = ad.Sign(sk); err != nil {
		return nil, fmt.Errorf("signing advert: %w", err)
	}

	if err := ad.Validate(); err != nil {
		return nil, fmt.Errorf("validating advert: %w", err)
	}

	link, err := store.PutAdvert(ctx, ad)
	if err != nil {
		return nil, fmt.Errorf("putting advert: %w", err)
	}

	head, err := head.NewSignedHead(link.(cidlink.Link).Cid, "/indexer/ingest/mainnet", sk)
	if err != nil {
		return nil, fmt.Errorf("signing head: %w", err)
	}
	if _, err := store.ReplaceHead(ctx, prevHead, head); err != nil {
		return nil, fmt.Errorf("replacing head: %w", err)
	}

	return link, nil
}
