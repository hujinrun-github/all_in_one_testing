package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ProfileTaskPollConfig struct {
	ControlPlaneURL string
	Token           string
	AgentID         string
}

type ProfileTaskProcessResult struct {
	Processed int `json:"processed"`
}

type profileTask struct {
	ID                      string   `json:"id"`
	RunID                   string   `json:"runId"`
	ScenarioID              string   `json:"scenarioId"`
	ScenarioName            string   `json:"scenarioName"`
	TargetID                string   `json:"targetId"`
	TargetName              string   `json:"targetName"`
	PprofBaseURL            string   `json:"pprofBaseUrl"`
	ProfileURL              string   `json:"profileUrl"`
	ProfileType             string   `json:"profileType"`
	ProfileSeconds          int      `json:"profileSeconds"`
	ProfileCommand          string   `json:"profileCommand"`
	ProfileCommandArgs      []string `json:"profileCommandArgs"`
	ProfileCommandOutput    string   `json:"profileCommandOutput"`
	ProfileCommandTimeoutMs int      `json:"profileCommandTimeoutMs"`
}

func ProcessProfileTasksOnce(ctx context.Context, client *http.Client, config ProfileTaskPollConfig) (ProfileTaskProcessResult, error) {
	config = normalizeProfileTaskPollConfig(config)
	if err := validateProfileTaskPollConfig(config); err != nil {
		return ProfileTaskProcessResult{}, err
	}
	if client == nil {
		client = http.DefaultClient
	}

	tasks, err := pollProfileTasks(ctx, client, config)
	if err != nil {
		return ProfileTaskProcessResult{}, err
	}

	result := ProfileTaskProcessResult{}
	for _, task := range tasks {
		uploadConfig := ProfileUploadConfig{
			ControlPlaneURL: config.ControlPlaneURL,
			Token:           config.Token,
			ProfileURL:      task.ProfileURL,
			PprofBaseURL:    task.PprofBaseURL,
			ProfileSeconds:  task.ProfileSeconds,
			RunID:           task.RunID,
			ScenarioID:      task.ScenarioID,
			ScenarioName:    task.ScenarioName,
			TargetID:        task.TargetID,
			TargetName:      task.TargetName,
			ProfileType:     task.ProfileType,
		}
		artifact, err := collectProfileTaskArtifact(ctx, client, task, uploadConfig)
		if err != nil {
			_ = completeProfileTask(ctx, client, config, task.ID, "", err.Error())
			return result, err
		}
		if err := completeProfileTask(ctx, client, config, task.ID, artifact.ID, ""); err != nil {
			return result, err
		}
		result.Processed++
	}

	return result, nil
}

func collectProfileTaskArtifact(ctx context.Context, client *http.Client, task profileTask, uploadConfig ProfileUploadConfig) (ProfileArtifactResponse, error) {
	if strings.TrimSpace(task.ProfileCommand) == "" {
		return CollectAndUploadProfile(ctx, client, uploadConfig)
	}
	return CollectAndUploadCommandProfile(ctx, client, CommandProfileConfig{
		ProfileUploadConfig: uploadConfig,
		Command:             task.ProfileCommand,
		Args:                task.ProfileCommandArgs,
		OutputPath:          task.ProfileCommandOutput,
		Timeout:             time.Duration(task.ProfileCommandTimeoutMs) * time.Millisecond,
	})
}

func normalizeProfileTaskPollConfig(config ProfileTaskPollConfig) ProfileTaskPollConfig {
	config.ControlPlaneURL = strings.TrimRight(strings.TrimSpace(config.ControlPlaneURL), "/")
	config.Token = strings.TrimSpace(config.Token)
	config.AgentID = strings.TrimSpace(config.AgentID)
	return config
}

func validateProfileTaskPollConfig(config ProfileTaskPollConfig) error {
	if config.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL is required")
	}
	if config.Token == "" {
		return fmt.Errorf("agent token is required")
	}
	if config.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}
	return nil
}

func pollProfileTasks(ctx context.Context, client *http.Client, config ProfileTaskPollConfig) ([]profileTask, error) {
	pollURL := config.ControlPlaneURL + "/agent/v1/profile-tasks?agentId=" + url.QueryEscape(config.AgentID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("profile task poll returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var tasks []profileTask
	if err := json.NewDecoder(response.Body).Decode(&tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func completeProfileTask(ctx context.Context, client *http.Client, config ProfileTaskPollConfig, taskID string, artifactID string, errorText string) error {
	payload := map[string]string{}
	if strings.TrimSpace(artifactID) != "" {
		payload["artifactId"] = strings.TrimSpace(artifactID)
	}
	if strings.TrimSpace(errorText) != "" {
		payload["error"] = strings.TrimSpace(errorText)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, config.ControlPlaneURL+"/agent/v1/profile-tasks/"+url.PathEscape(taskID)+"/complete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+config.Token)
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("profile task complete returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
