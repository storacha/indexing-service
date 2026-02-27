package main

import (
	crypto_ed25519 "crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/url"
	"os"

	logging "github.com/ipfs/go-log/v2"
	"github.com/ipni/go-libipni/maurl"
	"github.com/ipni/go-libipni/metadata"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	goredis "github.com/redis/go-redis/v9"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	userver "github.com/storacha/go-ucanto/server"
	"github.com/storacha/go-ucanto/validator"
	"github.com/urfave/cli/v2"

	"github.com/storacha/indexing-service/pkg/construct"
	"github.com/storacha/indexing-service/pkg/presets"
	"github.com/storacha/indexing-service/pkg/principalresolver"
	"github.com/storacha/indexing-service/pkg/redis"
	"github.com/storacha/indexing-service/pkg/server"
)

var serverCmd = &cli.Command{
	Name:  "server",
	Usage: "HTTP server interface to the indexing service",
	Subcommands: []*cli.Command{
		{
			Name:  "start",
			Usage: "start an indexing service HTTP server",
			Flags: []cli.Flag{
				&cli.IntFlag{
					Name:    "port",
					Aliases: []string{"p"},
					Value:   9000,
					Usage:   "port to bind the server to",
				},
				&cli.StringFlag{
					Name:    "private-key",
					Aliases: []string{"pk"},
					Usage:   "base64 encoded private key identity for the server",
				},
				&cli.StringFlag{
					Name:    "key-file",
					Aliases: []string{"kf"},
					Usage:   "path to PEM-encoded Ed25519 private key file",
				},
				&cli.StringFlag{
					Name:  "did",
					Usage: "DID of the server (only needs to be set if different from what is derived from the private key i.e. a did:web DID)",
				},
				&cli.StringFlag{
					Name:    "redis-url",
					Aliases: []string{"redis"},
					EnvVars: []string{"REDIS_URL"},
					Usage:   "url for a running redis database",
				},
				&cli.StringFlag{
					Name:    "redis-passwd",
					Aliases: []string{"rp"},
					EnvVars: []string{"REDIS_PASSWD"},
					Usage:   "passwd for redis",
				},
				&cli.StringFlag{
					Name:        "ipni-endpoint",
					Aliases:     []string{"ipni"},
					DefaultText: "Defaults to https://cid.contact",
					Value:       "https://cid.contact",
					Usage:       "HTTP endpoint of the IPNI instance used to discover providers.",
				},
				&cli.StringFlag{
					Name:  "ipni-announce-urls",
					Value: `["https://cid.contact/announce"]`,
					Usage: "JSON array of IPNI node URLs to announce chain updates to.",
				},
				&cli.StringFlag{
					Name:  "ipni-format-peer-id",
					Usage: "Peer ID of the IPNI node to use for format announcements (enables endpoint mimicking IPNI).",
				},
				&cli.StringFlag{
					Name:  "ipni-format-endpoint",
					Usage: "HTTP endpoint of the IPNI node to use for format announcements (enables endpoint mimicking IPNI).",
				},
				&cli.StringSliceFlag{
					Name:    "public-url",
					EnvVars: []string{"PUBLIC_URL"},
					Usage:   "Public URL(s) where the indexing service can be reached (for claim fetching). Can be specified multiple times or comma-separated in env var.",
				},
				&cli.StringSliceFlag{
					Name:    "resolve-did-web",
					EnvVars: []string{"RESOLVE_DID_WEB"},
					Usage:   "did:web DIDs to resolve via HTTP (fetches /.well-known/did.json). Can be specified multiple times or comma-separated in env var.",
				},
				&cli.BoolFlag{
					Name:    "insecure-did-resolution",
					EnvVars: []string{"INSECURE_DID_RESOLUTION"},
					Usage:   "Use HTTP instead of HTTPS for did:web resolution (for local development)",
				},
			},
			Action: func(cCtx *cli.Context) error {
				addr := fmt.Sprintf(":%d", cCtx.Int("port"))
				var id principal.Signer
				var err error
				var opts []server.Option

				if cCtx.IsSet("key-file") {
					// load from PEM file
					id, err = signerFromPEMFile(cCtx.String("key-file"))
					if err != nil {
						return fmt.Errorf("loading key from PEM file: %w", err)
					}
				} else if cCtx.IsSet("private-key") {
					id, err = ed25519.Parse(cCtx.String("private-key"))
					if err != nil {
						return fmt.Errorf("parsing server private key: %w", err)
					}
				} else {
					// generate a new private key if one is not provided
					id, err = ed25519.Generate()
					if err != nil {
						return fmt.Errorf("generating server private key: %w", err)
					}
				}

				// wrap with custom DID if specified
				if cCtx.String("did") != "" {
					customDID, err := did.Parse(cCtx.String("did"))
					if err != nil {
						return fmt.Errorf("parsing server DID: %w", err)
					}
					id, err = signer.Wrap(id, customDID)
					if err != nil {
						return fmt.Errorf("wrapping server DID: %w", err)
					}
				}

				opts = append(opts, server.WithIdentity(id))

				// Create principal resolver
				var presolv validator.PrincipalResolver
				resolveDIDs := cCtx.StringSlice("resolve-did-web")
				if len(resolveDIDs) > 0 {
					// Use HTTP-based resolution for specified DIDs
					var webDIDs []did.DID
					for _, d := range resolveDIDs {
						parsed, err := did.Parse(d)
						if err != nil {
							return fmt.Errorf("parsing resolve-did-web DID %s: %w", d, err)
						}
						webDIDs = append(webDIDs, parsed)
					}

					var httpOpts []principalresolver.HTTPOption
					if cCtx.Bool("insecure-did-resolution") {
						httpOpts = append(httpOpts, principalresolver.InsecureHTTPResolution())
					}

					httpResolver, err := principalresolver.NewHTTPResolver(webDIDs, httpOpts...)
					if err != nil {
						return fmt.Errorf("creating HTTP principal resolver: %w", err)
					}
					presolv = httpResolver
				} else {
					// Fall back to static mapping from presets
					staticResolver, err := principalresolver.New(presets.PrincipalMapping)
					if err != nil {
						return fmt.Errorf("creating principal resolver: %w", err)
					}
					presolv = staticResolver
				}
				opts = append(
					opts,
					server.WithContentClaimsOptions(
						userver.WithPrincipalResolver(presolv.ResolveDIDKey),
					),
				)

				ipniSrvOpts, err := ipniOpts(cCtx.String("ipni-format-peer-id"), cCtx.String("ipni-format-endpoint"))
				if err != nil {
					return fmt.Errorf("setting up IPNI options: %w", err)
				}
				opts = append(opts, ipniSrvOpts...)
				var sc construct.ServiceConfig
				sc.ID = id
				sc.IPNIFindURL = cCtx.String("ipni-endpoint")
				sc.PublicURL = cCtx.StringSlice("public-url")

				// Create standalone Redis client for local development
				redisOpts := &goredis.Options{
					Addr:     cCtx.String("redis-url"),
					Password: cCtx.String("redis-passwd"),
				}
				redisClient := goredis.NewClient(redisOpts)
				clientAdapter := redis.NewClientAdapter(redisClient)

				if cCtx.String("ipni-announce-urls") != "" {
					var urls []string
					err := json.Unmarshal([]byte(cCtx.String("ipni-announce-urls")), &urls)
					if err != nil {
						return fmt.Errorf("parsing IPNI announce URLs JSON: %w", err)
					}
					sc.IPNIDirectAnnounceURLs = urls
				} else {
					sc.IPNIDirectAnnounceURLs = presets.IPNIAnnounceURLs
				}

				privKey, err := crypto.UnmarshalEd25519PrivateKey(id.Raw())
				if err != nil {
					return fmt.Errorf("unmarshaling private key: %w", err)
				}
				sc.PrivateKey = privKey

				logging.SetAllLoggers(logging.LevelInfo)
				indexer, err := construct.Construct(sc,
					construct.WithProvidersClient(clientAdapter),
					construct.WithNoProvidersClient(redisClient),
					construct.WithClaimsClient(redisClient),
					construct.WithIndexesClient(redisClient),
				)
				if err != nil {
					return err
				}
				err = indexer.Startup(cCtx.Context)
				if err != nil {
					return err
				}
				defer func() {
					indexer.Shutdown(cCtx.Context)
				}()
				return server.ListenAndServe(addr, indexer, opts...)
			},
		},
	},
}

func ipniOpts(ipniFormatPeerID string, ipniFormatEndpoint string) ([]server.Option, error) {
	if ipniFormatEndpoint == "" || ipniFormatPeerID == "" {
		return nil, nil
	}
	peerID, err := peer.Decode(ipniFormatPeerID)
	if err != nil {
		return nil, fmt.Errorf("decoding IPNI format peer ID: %w", err)
	}
	url, err := url.Parse(ipniFormatEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parsing IPNI format endpoint URL: %w", err)
	}
	ma, err := maurl.FromURL(url)
	if err != nil {
		return nil, fmt.Errorf("converting IPNI format endpoint URL to multiaddr: %w", err)
	}
	return []server.Option{
		server.WithIPNI(peer.AddrInfo{ID: peerID, Addrs: []multiaddr.Multiaddr{ma}}, metadata.Default.New(metadata.IpfsGatewayHttp{})),
	}, nil
}

// signerFromPEMFile loads an Ed25519 private key from a PEM file.
func signerFromPEMFile(path string) (principal.Signer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pemData, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading private key: %w", err)
	}

	var privateKey *crypto_ed25519.PrivateKey
	rest := pemData

	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining

		if block.Type == "PRIVATE KEY" {
			parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse PKCS#8 private key: %w", err)
			}

			key, ok := parsedKey.(crypto_ed25519.PrivateKey)
			if !ok {
				return nil, fmt.Errorf("the parsed key is not an ED25519 private key")
			}
			privateKey = &key
			break
		}
	}

	if privateKey == nil {
		return nil, fmt.Errorf("could not find a PRIVATE KEY block in the PEM file")
	}

	return ed25519.FromRaw(*privateKey)
}
