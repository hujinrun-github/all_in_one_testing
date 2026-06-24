package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CommandProfileConfig struct {
	ProfileUploadConfig ProfileUploadConfig
	Command             string
	Args                []string
	Env                 []string
	OutputPath          string
	Timeout             time.Duration
}

func CollectAndUploadCommandProfile(ctx context.Context, client *http.Client, config CommandProfileConfig) (ProfileArtifactResponse, error) {
	uploadConfig := normalizeProfileUploadConfig(config.ProfileUploadConfig)
	command := strings.TrimSpace(config.Command)
	if command == "" {
		return ProfileArtifactResponse{}, fmt.Errorf("profile command is required")
	}
	if uploadConfig.ControlPlaneURL == "" {
		return ProfileArtifactResponse{}, fmt.Errorf("control plane URL is required")
	}
	if uploadConfig.Token == "" {
		return ProfileArtifactResponse{}, fmt.Errorf("agent token is required")
	}
	if uploadConfig.RunID == "" {
		return ProfileArtifactResponse{}, fmt.Errorf("run ID is required")
	}
	if uploadConfig.SourceURL == "" {
		uploadConfig.SourceURL = "command://" + command
	}
	if client == nil {
		client = http.DefaultClient
	}

	outputPath := strings.TrimSpace(config.OutputPath)
	cleanupOutput := false
	if outputPath == "" {
		outputFile, err := os.CreateTemp("", "ait-command-profile-*")
		if err != nil {
			return ProfileArtifactResponse{}, err
		}
		outputPath = outputFile.Name()
		if err := outputFile.Close(); err != nil {
			_ = os.Remove(outputPath)
			return ProfileArtifactResponse{}, err
		}
		cleanupOutput = true
	}
	if cleanupOutput {
		defer os.Remove(outputPath)
	}

	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := renderCommandProfileArgs(config.Args, outputPath, uploadConfig.ProfileSeconds)
	cmd := exec.CommandContext(commandCtx, command, args...)
	if len(config.Env) > 0 {
		cmd.Env = append(os.Environ(), config.Env...)
	}
	commandOutput, err := cmd.CombinedOutput()
	if commandCtx.Err() == context.DeadlineExceeded {
		return ProfileArtifactResponse{}, fmt.Errorf("profile command timed out after %s", timeout)
	}
	if err != nil {
		message := strings.TrimSpace(string(commandOutput))
		if message == "" {
			return ProfileArtifactResponse{}, fmt.Errorf("profile command failed: %w", err)
		}
		return ProfileArtifactResponse{}, fmt.Errorf("profile command failed: %w: %s", err, message)
	}

	payload, err := readCommandProfileOutput(outputPath)
	if err != nil {
		return ProfileArtifactResponse{}, err
	}
	return uploadProfileArtifact(ctx, client, uploadConfig, payload, "application/octet-stream")
}

func renderCommandProfileArgs(args []string, outputPath string, seconds int) []string {
	rendered := make([]string, 0, len(args))
	secondsValue := fmt.Sprintf("%d", seconds)
	for _, arg := range args {
		arg = strings.ReplaceAll(arg, "{{output}}", outputPath)
		arg = strings.ReplaceAll(arg, "{{seconds}}", secondsValue)
		rendered = append(rendered, arg)
	}
	return rendered
}

func readCommandProfileOutput(outputPath string) ([]byte, error) {
	file, err := os.Open(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read command profile output: %w", err)
	}
	defer file.Close()

	payload, err := io.ReadAll(io.LimitReader(file, maxProfileArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxProfileArtifactBytes {
		return nil, fmt.Errorf("profile artifact exceeds %d bytes", maxProfileArtifactBytes)
	}
	return payload, nil
}
