package main

import (
	"fmt"
	"os"

	logging "github.com/ipfs/go-log/v2"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/redis/go-redis/v9"
	"github.com/storacha/go-ucanto/did"
	"github.com/storacha/go-ucanto/principal"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
	ucanserver "github.com/storacha/go-ucanto/server"
	"github.com/storacha/indexing-service/cmd/config"
	"github.com/storacha/indexing-service/pkg/construct"
	"github.com/storacha/indexing-service/pkg/principalresolver"
	"github.com/storacha/indexing-service/pkg/server"
	"github.com/urfave/cli/v2"
)

var log = logging.Logger("cmd")

func main() {
	logging.SetLogLevel("*", "info")

	app := &cli.App{
		Name:  "indexing-service",
		Usage: "Manage running the indexing service.",
		Commands: []*cli.Command{
			{
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
							&cli.IntFlag{
								Name:    "provider-redis-db",
								Aliases: []string{"prd"},
								Usage:   "database number for providers index",
								Value:   0,
							},
							&cli.IntFlag{
								Name:    "claims-redis-db",
								Aliases: []string{"c"},
								Usage:   "database number for claims",
								Value:   1,
							},
							&cli.IntFlag{
								Name:    "indexes-redis-db",
								Aliases: []string{"i"},
								Usage:   "database number for indexes cache",
								Value:   2,
							},
							&cli.StringFlag{
								Name:        "ipni-endpoint",
								Aliases:     []string{"ipni"},
								DefaultText: "Defaults to https://cid.contact",
								Value:       "https://cid.contact",
								Usage:       "HTTP endpoint of the IPNI instance used to discover providers.",
							},
						},
						Action: func(cCtx *cli.Context) error {
							addr := fmt.Sprintf(":%d", cCtx.Int("port"))
							var id principal.Signer
							var err error
							var opts []server.Option

							if !cCtx.IsSet("private-key") {
								// generate a new private key if one is not provided
								id, err = ed25519.Generate()
								if err != nil {
									return fmt.Errorf("generating server private key: %w", err)
								}
							} else {
								id, err = ed25519.Parse(cCtx.String("private-key"))
								if err != nil {
									return fmt.Errorf("parsing server private key: %w", err)
								}
								if cCtx.String("did") != "" {
									did, err := did.Parse(cCtx.String("did"))
									if err != nil {
										return fmt.Errorf("parsing server DID: %w", err)
									}
									id, err = signer.Wrap(id, did)
									if err != nil {
										return fmt.Errorf("wrapping server DID: %w", err)
									}
								}
							}

							opts = append(opts, server.WithIdentity(id))

							presolv, err := principalresolver.New(config.PrincipalMapping)
							if err != nil {
								return fmt.Errorf("creating principal resolver: %w", err)
							}
							opts = append(
								opts,
								server.WithContentClaimsOptions(
									ucanserver.WithPrincipalResolver(presolv.ResolveDIDKey),
								),
							)

							var sc construct.ServiceConfig
							sc.ProvidersRedis = redis.Options{
								Addr:     cCtx.String("redis-url"),
								Password: cCtx.String("redis-passwd"),
								DB:       cCtx.Int("providers-redis-db"),
							}
							sc.ClaimsRedis = redis.Options{
								Addr:     cCtx.String("redis-url"),
								Password: cCtx.String("redis-passwd"),
								DB:       cCtx.Int("claims-redis-db"),
							}
							sc.IndexesRedis = redis.Options{
								Addr:     cCtx.String("redis-url"),
								Password: cCtx.String("redis-passwd"),
								DB:       cCtx.Int("indexes-redis-db"),
							}
							sc.IndexerURL = cCtx.String("ipni-endpoint")

							privKey, err := crypto.UnmarshalEd25519PrivateKey(id.Raw())
							if err != nil {
								return fmt.Errorf("unmarshaling private key: %w", err)
							}
							sc.PrivateKey = privKey

							indexer, err := construct.Construct(sc)
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
			},
			queryCmd,
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
