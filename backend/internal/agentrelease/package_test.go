package agentrelease

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageAgentBuildsLinuxAMD64BinaryAndChecksum(t *testing.T) {
	backendRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("expected backend root path, got %v", err)
	}
	outputDir := filepath.Join(t.TempDir(), "agents")

	artifact, err := PackageAgent(context.Background(), PackageConfig{
		BackendRoot: backendRoot,
		OutputDir:   outputDir,
		Version:     "test-version",
		Commit:      "test-commit",
		Date:        "2026-06-23T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("expected agent package to build, got %v", err)
	}
	if artifact.BinaryName != "all-in-one-agent-linux-amd64" {
		t.Fatalf("expected linux amd64 binary name, got %#v", artifact)
	}
	if artifact.Version != "test-version" || artifact.Commit != "test-commit" || artifact.Date != "2026-06-23T00:00:00Z" {
		t.Fatalf("expected build metadata to round-trip, got %#v", artifact)
	}

	payload, err := os.ReadFile(artifact.BinaryPath)
	if err != nil {
		t.Fatalf("expected packaged binary to be readable, got %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected packaged binary to be non-empty")
	}

	checksumPayload, err := os.ReadFile(artifact.ChecksumPath)
	if err != nil {
		t.Fatalf("expected checksum file to be readable, got %v", err)
	}
	sum := sha256.Sum256(payload)
	expectedChecksum := fmt.Sprintf("%x  %s\n", sum, artifact.BinaryName)
	if string(checksumPayload) != expectedChecksum {
		t.Fatalf("expected checksum %q, got %q", expectedChecksum, string(checksumPayload))
	}

	manifestPayload, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	if err != nil {
		t.Fatalf("expected release manifest to be readable, got %v", err)
	}
	var manifest struct {
		Artifacts []struct {
			Name        string `json:"name"`
			URL         string `json:"url"`
			ChecksumURL string `json:"checksumUrl"`
			SHA256      string `json:"sha256"`
			SizeBytes   int64  `json:"sizeBytes"`
			Version     string `json:"version"`
			Commit      string `json:"commit"`
			Date        string `json:"date"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(manifestPayload, &manifest); err != nil {
		t.Fatalf("expected release manifest JSON, got %v", err)
	}
	if len(manifest.Artifacts) != 1 {
		t.Fatalf("expected one release manifest artifact, got %#v", manifest)
	}
	release := manifest.Artifacts[0]
	if release.Name != artifact.BinaryName || release.URL != "/agent/binaries/"+artifact.BinaryName || release.ChecksumURL != "/agent/binaries/"+artifact.BinaryName+".sha256" {
		t.Fatalf("expected release manifest links, got %#v", release)
	}
	if release.SHA256 != fmt.Sprintf("%x", sum) || release.SizeBytes != int64(len(payload)) {
		t.Fatalf("expected release manifest checksum and size, got %#v", release)
	}
	if release.Version != "test-version" || release.Commit != "test-commit" || release.Date != "2026-06-23T00:00:00Z" {
		t.Fatalf("expected release manifest metadata, got %#v", release)
	}
}
