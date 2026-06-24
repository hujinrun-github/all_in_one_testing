package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"all_in_one_testing/backend/internal/agentrelease"
)

func TestRunPackagesAgentFromFlags(t *testing.T) {
	var captured agentrelease.PackageConfig
	packager := func(_ context.Context, config agentrelease.PackageConfig) (agentrelease.PackageArtifact, error) {
		captured = config
		return agentrelease.PackageArtifact{
			BinaryName:   "all-in-one-agent-linux-amd64",
			BinaryPath:   "D:/out/all-in-one-agent-linux-amd64",
			ChecksumPath: "D:/out/all-in-one-agent-linux-amd64.sha256",
			Version:      config.Version,
			Commit:       config.Commit,
			Date:         config.Date,
		}, nil
	}

	output := &bytes.Buffer{}
	err := run(context.Background(), []string{
		"--backend-root", "D:/repo/backend",
		"--output-dir", "D:/out",
		"--version", "v0.4.0",
		"--commit", "abc123",
		"--date", "2026-06-23T00:00:00Z",
	}, output, packager)
	if err != nil {
		t.Fatalf("expected package command to succeed, got %v", err)
	}

	if captured.BackendRoot != "D:/repo/backend" || captured.OutputDir != "D:/out" {
		t.Fatalf("expected package paths from flags, got %#v", captured)
	}
	if captured.Version != "v0.4.0" || captured.Commit != "abc123" || captured.Date != "2026-06-23T00:00:00Z" {
		t.Fatalf("expected package metadata from flags, got %#v", captured)
	}

	var artifact agentrelease.PackageArtifact
	if err := json.NewDecoder(output).Decode(&artifact); err != nil {
		t.Fatalf("expected package JSON output, got %v", err)
	}
	if artifact.BinaryName != "all-in-one-agent-linux-amd64" || artifact.Version != "v0.4.0" {
		t.Fatalf("expected package artifact JSON, got %#v", artifact)
	}
}
