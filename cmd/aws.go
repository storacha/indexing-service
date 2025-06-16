package main

import (
	"fmt"

	userver "github.com/storacha/go-ucanto/server"
	"github.com/storacha/indexing-service/pkg/aws"
	"github.com/storacha/indexing-service/pkg/principalresolver"
	"github.com/storacha/indexing-service/pkg/server"
	"github.com/storacha/indexing-service/pkg/telemetry"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/sdk/trace"
)

var awsCmd = &cli.Command{
	Name:  "aws",
	Usage: "Run the indexing service as a containerized server in AWS",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Value:   8080,
			Usage:   "Port to bind the server to",
		},
	},
	Action: func(cCtx *cli.Context) error {
		addr := fmt.Sprintf(":%d", cCtx.Int("port"))
		cfg := aws.FromEnv(cCtx.Context)
		srvOpts := []server.Option{
			server.WithIdentity(cfg.Signer),
		}

		presolv, err := principalresolver.New(cfg.PrincipalMapping)
		if err != nil {
			return fmt.Errorf("creating principal resolver: %w", err)
		}

		srvOpts = append(
			srvOpts,
			server.WithContentClaimsOptions(
				userver.WithPrincipalResolver(presolv.ResolveDIDKey),
			),
		)

		// an empty API key disables instrumentation
		if cfg.HoneycombAPIKey != "" {
			var telemetryOpts []telemetry.TelemetryOption
			if cfg.BaseTraceSampleRatio < 1.0 {
				telemetryOpts = append(telemetryOpts, telemetry.WithBaseSampler(trace.TraceIDRatioBased(cfg.BaseTraceSampleRatio)))
			}
			telemetryShutdown, err := telemetry.SetupTelemetry(cCtx.Context, &cfg.Config, telemetryOpts...)
			if err != nil {
				panic(err)
			}
			defer telemetryShutdown(cCtx.Context)
		}

		indexer, err := aws.Construct(cfg)
		if err != nil {
			return err
		}

		return server.ListenAndServe(addr, indexer, srvOpts...)
	},
}
