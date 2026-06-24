package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
)

const maxProfileArtifactBytes = 64 * 1024 * 1024

type ProfileUploadConfig struct {
	ControlPlaneURL string
	Token           string
	ProfileURL      string
	PprofBaseURL    string
	ProfileSeconds  int
	RunID           string
	ScenarioID      string
	ScenarioName    string
	TargetID        string
	TargetName      string
	ProfileType     string
	FileName        string
	SourceURL       string
	StartedAt       string
	FinishedAt      string
}

type ProfileArtifactResponse struct {
	ID          string `json:"id"`
	RunID       string `json:"runId"`
	ProfileType string `json:"profileType"`
	Status      string `json:"status"`
	FileName    string `json:"fileName"`
	SizeBytes   int64  `json:"sizeBytes"`
}

func CollectAndUploadProfile(ctx context.Context, client *http.Client, config ProfileUploadConfig) (ProfileArtifactResponse, error) {
	config = normalizeProfileUploadConfig(config)
	if err := validateProfileUploadConfig(config); err != nil {
		return ProfileArtifactResponse{}, err
	}
	if client == nil {
		client = http.DefaultClient
	}

	payload, contentType, err := fetchProfile(ctx, client, config.ProfileURL)
	if err != nil {
		return ProfileArtifactResponse{}, err
	}

	return uploadProfileArtifact(ctx, client, config, payload, contentType)
}

func normalizeProfileUploadConfig(config ProfileUploadConfig) ProfileUploadConfig {
	config.ControlPlaneURL = strings.TrimRight(strings.TrimSpace(config.ControlPlaneURL), "/")
	config.Token = strings.TrimSpace(config.Token)
	config.ProfileURL = strings.TrimSpace(config.ProfileURL)
	config.PprofBaseURL = strings.TrimRight(strings.TrimSpace(config.PprofBaseURL), "/")
	config.RunID = strings.TrimSpace(config.RunID)
	config.ScenarioID = strings.TrimSpace(config.ScenarioID)
	config.ScenarioName = strings.TrimSpace(config.ScenarioName)
	config.TargetID = strings.TrimSpace(config.TargetID)
	config.TargetName = strings.TrimSpace(config.TargetName)
	config.ProfileType = strings.ToLower(strings.TrimSpace(config.ProfileType))
	config.FileName = filepath.Base(strings.TrimSpace(config.FileName))
	config.SourceURL = strings.TrimSpace(config.SourceURL)
	config.StartedAt = strings.TrimSpace(config.StartedAt)
	config.FinishedAt = strings.TrimSpace(config.FinishedAt)
	if config.ProfileType == "" {
		config.ProfileType = "cpu"
	}
	if config.ProfileSeconds <= 0 {
		config.ProfileSeconds = 1
	}
	if config.ProfileURL == "" && config.PprofBaseURL != "" {
		config.ProfileURL = buildGoPprofProfileURL(config.PprofBaseURL, config.ProfileType, config.ProfileSeconds)
	}
	if config.SourceURL == "" {
		config.SourceURL = config.ProfileURL
	}
	if config.FileName == "." || config.FileName == "" {
		config.FileName = fmt.Sprintf("%s-%s.pprof", config.RunID, config.ProfileType)
	}
	return config
}

func buildGoPprofProfileURL(baseURL string, profileType string, seconds int) string {
	switch profileType {
	case "cpu":
		return fmt.Sprintf("%s/profile?seconds=%d", baseURL, seconds)
	case "heap", "goroutine", "mutex", "block", "allocs", "threadcreate":
		return baseURL + "/" + profileType
	default:
		return baseURL + "/" + strings.TrimLeft(profileType, "/")
	}
}

func validateProfileUploadConfig(config ProfileUploadConfig) error {
	if config.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL is required")
	}
	if config.Token == "" {
		return fmt.Errorf("agent token is required")
	}
	if config.ProfileURL == "" {
		return fmt.Errorf("profile URL or pprof base URL is required")
	}
	if config.RunID == "" {
		return fmt.Errorf("run ID is required")
	}
	return nil
}

func fetchProfile(ctx context.Context, client *http.Client, profileURL string) ([]byte, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, profileURL, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		return nil, response.Header.Get("Content-Type"), fmt.Errorf("profile endpoint returned HTTP %d", response.StatusCode)
	}

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxProfileArtifactBytes+1))
	if err != nil {
		return nil, response.Header.Get("Content-Type"), err
	}
	if len(payload) > maxProfileArtifactBytes {
		return nil, response.Header.Get("Content-Type"), fmt.Errorf("profile artifact exceeds %d bytes", maxProfileArtifactBytes)
	}
	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return payload, contentType, nil
}

func uploadProfileArtifact(ctx context.Context, client *http.Client, config ProfileUploadConfig, payload []byte, contentType string) (ProfileArtifactResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for field, value := range map[string]string{
		"runId":        config.RunID,
		"scenarioId":   config.ScenarioID,
		"scenarioName": config.ScenarioName,
		"targetId":     config.TargetID,
		"targetName":   config.TargetName,
		"profileType":  config.ProfileType,
		"sourceUrl":    config.SourceURL,
		"startedAt":    config.StartedAt,
		"finishedAt":   config.FinishedAt,
	} {
		if value == "" {
			continue
		}
		if err := writer.WriteField(field, value); err != nil {
			return ProfileArtifactResponse{}, err
		}
	}

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeMultipartFilename(config.FileName)))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return ProfileArtifactResponse{}, err
	}
	if _, err := part.Write(payload); err != nil {
		return ProfileArtifactResponse{}, err
	}
	if err := writer.Close(); err != nil {
		return ProfileArtifactResponse{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.ControlPlaneURL+"/agent/v1/profile-artifacts", body)
	if err != nil {
		return ProfileArtifactResponse{}, err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	response, err := client.Do(request)
	if err != nil {
		return ProfileArtifactResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return ProfileArtifactResponse{}, fmt.Errorf("artifact upload returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var result ProfileArtifactResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return ProfileArtifactResponse{}, err
	}
	return result, nil
}

func escapeMultipartFilename(fileName string) string {
	fileName = strings.ReplaceAll(fileName, `\`, `\\`)
	return strings.ReplaceAll(fileName, `"`, `\"`)
}
