package main

import (
	"fmt"
	"os"

	logging "github.com/ipfs/go-log/v2"
	"github.com/storacha/go-ucanto/did"
	ed25519 "github.com/storacha/go-ucanto/principal/ed25519/signer"
	"github.com/storacha/go-ucanto/principal/signer"
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
						},
						Action: func(cCtx *cli.Context) error {
							addr := fmt.Sprintf(":%d", cCtx.Int("port"))
							var opts []server.Option
							if cCtx.String("private-key") != "" {
								id, err := ed25519.Parse(cCtx.String("private-key"))
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
								opts = append(opts, server.WithIdentity(id))
							}
							return server.ListenAndServe(addr, opts...)
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
