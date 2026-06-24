package agentrelease

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const AgentLinuxAMD64BinaryName = "all-in-one-agent-linux-amd64"

type PackageConfig struct {
	BackendRoot string
	OutputDir   string
	Version     string
	Commit      string
	Date        string
}

type PackageArtifact struct {
	BinaryName   string
	BinaryPath   string
	ChecksumPath string
	Version      string
	Commit       string
	Date         string
}

func PackageAgent(ctx context.Context, config PackageConfig) (PackageArtifact, error) {
	config = normalizePackageConfig(config)
	if err := os.MkdirAll(config.OutputDir, 0o755); err != nil {
		return PackageArtifact{}, err
	}

	binaryPath := filepath.Join(config.OutputDir, AgentLinuxAMD64BinaryName)
	command := exec.CommandContext(ctx, "go", "build",
		"-trimpath",
		"-ldflags", agentBuildLDFlags(config),
		"-o", binaryPath,
		"./cmd/agent",
	)
	command.Dir = config.BackendRoot
	command.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		return PackageArtifact{}, fmt.Errorf("build linux amd64 agent: %w: %s", err, strings.TrimSpace(string(output)))
	}

	checksumPath := binaryPath + ".sha256"
	sum, sizeBytes, err := writeSHA256File(binaryPath, checksumPath, AgentLinuxAMD64BinaryName)
	if err != nil {
		return PackageArtifact{}, err
	}
	if err := writeManifestFile(config, filepath.Join(config.OutputDir, "manifest.json"), sum, sizeBytes); err != nil {
		return PackageArtifact{}, err
	}

	return PackageArtifact{
		BinaryName:   AgentLinuxAMD64BinaryName,
		BinaryPath:   binaryPath,
		ChecksumPath: checksumPath,
		Version:      config.Version,
		Commit:       config.Commit,
		Date:         config.Date,
	}, nil
}

func normalizePackageConfig(config PackageConfig) PackageConfig {
	config.BackendRoot = strings.TrimSpace(config.BackendRoot)
	if config.BackendRoot == "" {
		config.BackendRoot = "."
	}
	config.OutputDir = strings.TrimSpace(config.OutputDir)
	if config.OutputDir == "" {
		config.OutputDir = filepath.Join(config.BackendRoot, "dist", "agents")
	}
	config.Version = stringOrFallback(config.Version, "dev")
	config.Commit = stringOrFallback(config.Commit, "unknown")
	config.Date = stringOrFallback(config.Date, time.Now().UTC().Format(time.RFC3339))
	return config
}

func agentBuildLDFlags(config PackageConfig) string {
	return fmt.Sprintf("-s -w -X main.version=%s -X main.commit=%s -X main.date=%s", config.Version, config.Commit, config.Date)
}

func writeSHA256File(binaryPath string, checksumPath string, binaryName string) (string, int64, error) {
	file, err := os.Open(binaryPath)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()

	hash := sha256.New()
	sizeBytes, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	sum := fmt.Sprintf("%x", hash.Sum(nil))
	payload := fmt.Sprintf("%s  %s\n", sum, binaryName)
	return sum, sizeBytes, os.WriteFile(checksumPath, []byte(payload), 0o644)
}

func writeManifestFile(config PackageConfig, manifestPath string, sum string, sizeBytes int64) error {
	manifest := struct {
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
	}{
		Artifacts: []struct {
			Name        string `json:"name"`
			URL         string `json:"url"`
			ChecksumURL string `json:"checksumUrl"`
			SHA256      string `json:"sha256"`
			SizeBytes   int64  `json:"sizeBytes"`
			Version     string `json:"version"`
			Commit      string `json:"commit"`
			Date        string `json:"date"`
		}{
			{
				Name:        AgentLinuxAMD64BinaryName,
				URL:         "/agent/binaries/" + AgentLinuxAMD64BinaryName,
				ChecksumURL: "/agent/binaries/" + AgentLinuxAMD64BinaryName + ".sha256",
				SHA256:      sum,
				SizeBytes:   sizeBytes,
				Version:     config.Version,
				Commit:      config.Commit,
				Date:        config.Date,
			},
		},
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(manifestPath, payload, 0o644)
}

func stringOrFallback(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
