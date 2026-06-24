package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"all_in_one_testing/backend/internal/agentrelease"
)

type packageAgentFunc func(context.Context, agentrelease.PackageConfig) (agentrelease.PackageArtifact, error)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, agentrelease.PackageAgent); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer, packager packageAgentFunc) error {
	flags := flag.NewFlagSet("package-agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	config := agentrelease.PackageConfig{}
	flags.StringVar(&config.BackendRoot, "backend-root", ".", "backend module root containing go.mod")
	flags.StringVar(&config.OutputDir, "output-dir", "", "output directory for agent artifacts; defaults to <backend-root>/dist/agents")
	flags.StringVar(&config.Version, "version", "", "agent version metadata")
	flags.StringVar(&config.Commit, "commit", "", "source commit metadata")
	flags.StringVar(&config.Date, "date", "", "build date metadata")
	if err := flags.Parse(args); err != nil {
		return err
	}

	artifact, err := packager(ctx, config)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(artifact)
}
