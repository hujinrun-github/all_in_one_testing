package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"all_in_one_testing/backend/internal/agent"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	configPath, err := configPathFromArgs(args)
	if err != nil {
		return err
	}
	fileConfig, err := loadAgentFileConfig(configPath)
	if err != nil {
		return err
	}
	heartbeatInterval, err := durationFromEnvConfig("HEARTBEAT_INTERVAL", fileConfig.HeartbeatInterval, 0)
	if err != nil {
		return err
	}
	metricsInterval, err := durationFromEnvConfig("METRICS_INTERVAL", fileConfig.MetricsInterval, 0)
	if err != nil {
		return err
	}
	profileTaskInterval, err := durationFromEnvConfig("PROFILE_TASK_INTERVAL", fileConfig.ProfileTaskInterval, 0)
	if err != nil {
		return err
	}
	targetHealthInterval, err := durationFromEnvConfig("TARGET_HEALTH_INTERVAL", fileConfig.TargetHealthInterval, 0)
	if err != nil {
		return err
	}
	commandTimeout, err := durationFromEnvConfig("PROFILE_COMMAND_TIMEOUT", fileConfig.ProfileCommandTimeout, 0)
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("all-in-one-agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	config := agent.ProfileUploadConfig{
		ControlPlaneURL: envOrConfig("CONTROL_PLANE_URL", fileConfig.ControlPlaneURL, "http://127.0.0.1:8080"),
		Token:           envOrConfig("AGENT_TOKEN", fileConfig.Token, ""),
		ProfileURL:      fileConfig.ProfileURL,
		PprofBaseURL:    fileConfig.PprofBaseURL,
		ProfileSeconds:  intOrDefault(fileConfig.ProfileSeconds, 1),
		RunID:           fileConfig.RunID,
		ScenarioID:      fileConfig.ScenarioID,
		ScenarioName:    fileConfig.ScenarioName,
		TargetID:        fileConfig.TargetID,
		TargetName:      fileConfig.TargetName,
		ProfileType:     stringOrDefault(fileConfig.ProfileType, "cpu"),
		FileName:        fileConfig.FileName,
		SourceURL:       fileConfig.SourceURL,
		StartedAt:       fileConfig.StartedAt,
		FinishedAt:      fileConfig.FinishedAt,
	}
	commandConfig := agent.CommandProfileConfig{
		Command:    fileConfig.ProfileCommand,
		OutputPath: fileConfig.ProfileCommandOutput,
		Timeout:    commandTimeout,
	}
	commandArgs := repeatableStrings(fileConfig.ProfileCommandArgs)
	daemonConfig := agent.DaemonConfig{
		Name:                 envOrConfig("AGENT_NAME", fileConfig.Name, ""),
		Hostname:             envOrConfig("AGENT_HOSTNAME", fileConfig.Hostname, ""),
		IP:                   envOrConfig("AGENT_IP", fileConfig.IP, ""),
		Version:              envOrConfig("AGENT_VERSION", fileConfig.Version, version),
		Labels:               copyStringMap(fileConfig.Labels),
		Capabilities:         append([]string(nil), fileConfig.Capabilities...),
		HeartbeatInterval:    heartbeatInterval,
		MetricsInterval:      metricsInterval,
		ProfileTaskInterval:  profileTaskInterval,
		TargetHealthInterval: targetHealthInterval,
	}
	agentID := envOrConfig("AGENT_ID", fileConfig.AgentID, "")
	pollOnce := false
	daemon := false
	buildInfo := false
	labels := envOrConfig("AGENT_LABELS", fileConfig.LabelsText, "")
	capabilities := envOrConfig("AGENT_CAPABILITIES", fileConfig.CapabilitiesText, "")
	flags.StringVar(&configPath, "config", configPath, "path to JSON agent configuration file")
	flags.StringVar(&config.ControlPlaneURL, "control-plane", config.ControlPlaneURL, "control plane base URL")
	flags.StringVar(&config.Token, "token", config.Token, "agent bearer token")
	flags.StringVar(&agentID, "agent-id", agentID, "agent id used when polling profile tasks")
	flags.BoolVar(&pollOnce, "poll-once", false, "poll one batch of profile tasks, process them, and exit")
	flags.BoolVar(&daemon, "daemon", false, "run as a long-lived agent daemon")
	flags.BoolVar(&buildInfo, "build-info", false, "print agent build metadata and exit")
	flags.StringVar(&daemonConfig.Name, "name", daemonConfig.Name, "agent display name")
	flags.StringVar(&daemonConfig.Hostname, "hostname", daemonConfig.Hostname, "agent hostname")
	flags.StringVar(&daemonConfig.IP, "ip", daemonConfig.IP, "agent IP address")
	flags.StringVar(&daemonConfig.Version, "version", daemonConfig.Version, "agent version")
	flags.StringVar(&labels, "labels", labels, "comma-separated labels, for example service=checkout,zone=shanghai-a")
	flags.StringVar(&capabilities, "capabilities", capabilities, "comma-separated capabilities")
	flags.DurationVar(&daemonConfig.HeartbeatInterval, "heartbeat-interval", daemonConfig.HeartbeatInterval, "daemon heartbeat interval")
	flags.DurationVar(&daemonConfig.MetricsInterval, "metrics-interval", daemonConfig.MetricsInterval, "daemon metrics interval")
	flags.DurationVar(&daemonConfig.ProfileTaskInterval, "profile-task-interval", daemonConfig.ProfileTaskInterval, "daemon profile task polling interval")
	flags.DurationVar(&daemonConfig.TargetHealthInterval, "target-health-interval", daemonConfig.TargetHealthInterval, "daemon target health check interval")
	flags.StringVar(&config.ProfileURL, "profile-url", config.ProfileURL, "source pprof/prof URL")
	flags.StringVar(&config.PprofBaseURL, "pprof-base-url", config.PprofBaseURL, "Go pprof base URL, for example http://127.0.0.1:6060/debug/pprof")
	flags.IntVar(&config.ProfileSeconds, "profile-seconds", config.ProfileSeconds, "CPU profile duration in seconds when profile-type is cpu")
	flags.StringVar(&config.RunID, "run-id", config.RunID, "run id to attach the artifact to")
	flags.StringVar(&config.ScenarioID, "scenario-id", config.ScenarioID, "scenario id metadata")
	flags.StringVar(&config.ScenarioName, "scenario-name", config.ScenarioName, "scenario name metadata")
	flags.StringVar(&config.TargetID, "target-id", config.TargetID, "target id metadata")
	flags.StringVar(&config.TargetName, "target-name", config.TargetName, "target name metadata")
	flags.StringVar(&config.ProfileType, "profile-type", config.ProfileType, "profile type, for example cpu or heap")
	flags.StringVar(&config.FileName, "file-name", config.FileName, "uploaded artifact filename")
	flags.StringVar(&config.SourceURL, "source-url", config.SourceURL, "source URL metadata; defaults to profile-url")
	flags.StringVar(&config.StartedAt, "started-at", config.StartedAt, "profile capture start time")
	flags.StringVar(&config.FinishedAt, "finished-at", config.FinishedAt, "profile capture finish time")
	flags.StringVar(&commandConfig.Command, "profile-command", commandConfig.Command, "controlled profiler command path; executes without a shell")
	flags.Var(&commandArgs, "profile-command-arg", "argument for the controlled profiler command; may be repeated and supports {{output}} and {{seconds}}")
	flags.StringVar(&commandConfig.OutputPath, "profile-command-output", commandConfig.OutputPath, "local output path written by the controlled profiler command")
	flags.DurationVar(&commandConfig.Timeout, "profile-command-timeout", commandConfig.Timeout, "controlled profiler command timeout")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if buildInfo {
		return json.NewEncoder(stdout).Encode(map[string]string{
			"version": version,
			"commit":  commit,
			"date":    date,
		})
	}

	client := &http.Client{Timeout: 30 * time.Second}
	if daemon {
		daemonConfig.ControlPlaneURL = config.ControlPlaneURL
		daemonConfig.Token = config.Token
		daemonConfig.AgentID = agentID
		if labels != "" {
			daemonConfig.Labels = parseLabelPairs(labels)
		}
		if capabilities != "" {
			daemonConfig.Capabilities = splitCSV(capabilities)
		}
		if err := agent.RunDaemon(ctx, client, daemonConfig); err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(map[string]string{"status": "stopped"})
	}
	if pollOnce {
		result, err := agent.ProcessProfileTasksOnce(ctx, client, agent.ProfileTaskPollConfig{
			ControlPlaneURL: config.ControlPlaneURL,
			Token:           config.Token,
			AgentID:         agentID,
		})
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(result)
	}
	if strings.TrimSpace(commandConfig.Command) != "" {
		commandConfig.ProfileUploadConfig = config
		commandConfig.Args = []string(commandArgs)
		result, err := agent.CollectAndUploadCommandProfile(ctx, client, commandConfig)
		if err != nil {
			return err
		}
		return json.NewEncoder(stdout).Encode(result)
	}

	result, err := agent.CollectAndUploadProfile(ctx, client, config)
	if err != nil {
		return err
	}
	return json.NewEncoder(stdout).Encode(result)
}

type agentFileConfig struct {
	ControlPlaneURL       string            `json:"controlPlaneUrl"`
	Token                 string            `json:"token"`
	AgentID               string            `json:"agentId"`
	Name                  string            `json:"name"`
	Hostname              string            `json:"hostname"`
	IP                    string            `json:"ip"`
	Version               string            `json:"version"`
	Labels                map[string]string `json:"labels"`
	LabelsText            string            `json:"labelsText"`
	Capabilities          []string          `json:"capabilities"`
	CapabilitiesText      string            `json:"capabilitiesText"`
	HeartbeatInterval     string            `json:"heartbeatInterval"`
	MetricsInterval       string            `json:"metricsInterval"`
	ProfileTaskInterval   string            `json:"profileTaskInterval"`
	TargetHealthInterval  string            `json:"targetHealthInterval"`
	ProfileURL            string            `json:"profileUrl"`
	PprofBaseURL          string            `json:"pprofBaseUrl"`
	ProfileSeconds        int               `json:"profileSeconds"`
	RunID                 string            `json:"runId"`
	ScenarioID            string            `json:"scenarioId"`
	ScenarioName          string            `json:"scenarioName"`
	TargetID              string            `json:"targetId"`
	TargetName            string            `json:"targetName"`
	ProfileType           string            `json:"profileType"`
	FileName              string            `json:"fileName"`
	SourceURL             string            `json:"sourceUrl"`
	StartedAt             string            `json:"startedAt"`
	FinishedAt            string            `json:"finishedAt"`
	ProfileCommand        string            `json:"profileCommand"`
	ProfileCommandArgs    []string          `json:"profileCommandArgs"`
	ProfileCommandOutput  string            `json:"profileCommandOutput"`
	ProfileCommandTimeout string            `json:"profileCommandTimeout"`
}

func configPathFromArgs(args []string) (string, error) {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--config" {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return "", fmt.Errorf("--config requires a value")
			}
			return args[index+1], nil
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, "--config=")), nil
		}
	}
	return "", nil
}

func loadAgentFileConfig(path string) (agentFileConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return agentFileConfig{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return agentFileConfig{}, fmt.Errorf("read agent config: %w", err)
	}
	defer file.Close()

	var config agentFileConfig
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return agentFileConfig{}, fmt.Errorf("parse agent config: %w", err)
	}
	return config, nil
}

func envOrConfig(name string, configValue string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	if strings.TrimSpace(configValue) != "" {
		return configValue
	}
	return fallback
}

func envOrDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func durationFromEnvConfig(name string, configValue string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if strings.TrimSpace(value) == "" {
		value = configValue
	}
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	return parsed, nil
}

func intOrDefault(value int, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func stringOrDefault(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func copyStringMap(values map[string]string) map[string]string {
	copied := map[string]string{}
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func parseLabelPairs(labels string) map[string]string {
	parsed := map[string]string{}
	for _, pair := range strings.Split(labels, ",") {
		key, value, found := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			continue
		}
		parsed[key] = strings.TrimSpace(value)
	}
	return parsed
}

func splitCSV(values string) []string {
	parts := []string{}
	for _, value := range strings.Split(values, ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			parts = append(parts, value)
		}
	}
	return parts
}

type repeatableStrings []string

func (values *repeatableStrings) String() string {
	if values == nil {
		return ""
	}
	return strings.Join(*values, ",")
}

func (values *repeatableStrings) Set(value string) error {
	*values = append(*values, value)
	return nil
}
