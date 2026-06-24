package api

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

const agentBinaryLinuxAMD64 = "all-in-one-agent-linux-amd64"
const agentBinaryManifestName = "manifest.json"

const agentInstallScript = `#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  curl -fsSL http://control-plane:8080/agent/install.sh | sudo bash -s -- \
    --server http://control-plane:8080 \
    --token <AGENT_TOKEN> \
    [--agent-id agent-checkout-01] \
    [--name checkout-01] \
    [--labels env=prod,service=checkout] \
    [--binary /usr/local/bin/all-in-one-agent] \
    [--binary-url http://control-plane:8080/agent/binaries/all-in-one-agent-linux-amd64] \
    [--force-download]

The installer downloads the all-in-one-agent binary when it is missing locally.
Use --force-download or --upgrade to replace an existing binary during upgrades.
It writes /etc/all-in-one-agent/agent.json and installs a systemd service.
USAGE
}

require_value() {
  local flag="${1:-}"
  local value="${2:-}"
  if [[ -z "${value}" || "${value}" == --* ]]; then
    echo "${flag} requires a value" >&2
    usage >&2
    exit 2
  fi
}

slugify() {
  tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//'
}

download_agent_binary() {
  if [[ -z "${BINARY_URL}" ]]; then
    BINARY_URL="${SERVER_URL%/}/agent/binaries/all-in-one-agent-linux-amd64"
  fi
  CHECKSUM_URL="${BINARY_URL}.sha256"
  echo "downloading all-in-one-agent from ${BINARY_URL}"
  install -d -m 0755 "$(dirname "${BINARY_PATH}")"
  DOWNLOAD_PATH="$(mktemp "${BINARY_PATH}.download.XXXXXX")"
  trap 'rm -f "${DOWNLOAD_PATH}" "${DOWNLOAD_PATH}.sha256"' RETURN
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${BINARY_URL}" -o "${DOWNLOAD_PATH}"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "${DOWNLOAD_PATH}" "${BINARY_URL}"
  else
    echo "curl or wget is required to download ${BINARY_URL}" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    echo "downloading all-in-one-agent checksum from ${CHECKSUM_URL}"
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL "${CHECKSUM_URL}" -o "${DOWNLOAD_PATH}.sha256"
    elif command -v wget >/dev/null 2>&1; then
      wget -qO "${DOWNLOAD_PATH}.sha256" "${CHECKSUM_URL}"
    fi
    EXPECTED_SHA256="$(awk '{print $1}' "${DOWNLOAD_PATH}.sha256")"
    if [[ -z "${EXPECTED_SHA256}" ]]; then
      echo "checksum file is empty or invalid: ${CHECKSUM_URL}" >&2
      exit 1
    fi
    printf '%s  %s\n' "${EXPECTED_SHA256}" "${DOWNLOAD_PATH}" > "${DOWNLOAD_PATH}.sha256"
    sha256sum -c "${DOWNLOAD_PATH}.sha256"
  else
    echo "sha256sum is not available; skipping binary checksum verification" >&2
  fi
  install -m 0755 "${DOWNLOAD_PATH}" "${BINARY_PATH}"
}

SERVER_URL="${CONTROL_PLANE_URL:-}"
AGENT_TOKEN="${AGENT_TOKEN:-}"
AGENT_HOSTNAME="${AGENT_HOSTNAME:-$(hostname -f 2>/dev/null || hostname)}"
AGENT_ID="${AGENT_ID:-}"
AGENT_NAME="${AGENT_NAME:-${AGENT_HOSTNAME}}"
AGENT_LABELS="${AGENT_LABELS:-}"
AGENT_CAPABILITIES="${AGENT_CAPABILITIES:-host_metrics,process_metrics,pprof,profile_tasks}"
BINARY_PATH="${BINARY_PATH:-/usr/local/bin/all-in-one-agent}"
BINARY_URL="${BINARY_URL:-}"
FORCE_DOWNLOAD="${FORCE_DOWNLOAD:-0}"
HEARTBEAT_INTERVAL="${HEARTBEAT_INTERVAL:-10s}"
METRICS_INTERVAL="${METRICS_INTERVAL:-5s}"
PROFILE_TASK_INTERVAL="${PROFILE_TASK_INTERVAL:-5s}"
CONFIG_FILE="/etc/all-in-one-agent/agent.json"
SERVICE_FILE="/etc/systemd/system/all-in-one-agent.service"

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --server|--control-plane)
      require_value "$1" "${2:-}"
      SERVER_URL="$2"
      shift 2
      ;;
    --token)
      require_value "$1" "${2:-}"
      AGENT_TOKEN="$2"
      shift 2
      ;;
    --agent-id)
      require_value "$1" "${2:-}"
      AGENT_ID="$2"
      shift 2
      ;;
    --name)
      require_value "$1" "${2:-}"
      AGENT_NAME="$2"
      shift 2
      ;;
    --hostname)
      require_value "$1" "${2:-}"
      AGENT_HOSTNAME="$2"
      shift 2
      ;;
    --labels)
      require_value "$1" "${2:-}"
      AGENT_LABELS="$2"
      shift 2
      ;;
    --capabilities)
      require_value "$1" "${2:-}"
      AGENT_CAPABILITIES="$2"
      shift 2
      ;;
    --binary)
      require_value "$1" "${2:-}"
      BINARY_PATH="$2"
      shift 2
      ;;
    --binary-url)
      require_value "$1" "${2:-}"
      BINARY_URL="$2"
      shift 2
      ;;
    --force-download|--upgrade)
      FORCE_DOWNLOAD="1"
      shift
      ;;
    --heartbeat-interval)
      require_value "$1" "${2:-}"
      HEARTBEAT_INTERVAL="$2"
      shift 2
      ;;
    --metrics-interval)
      require_value "$1" "${2:-}"
      METRICS_INTERVAL="$2"
      shift 2
      ;;
    --profile-task-interval)
      require_value "$1" "${2:-}"
      PROFILE_TASK_INTERVAL="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ "${EUID}" -ne 0 ]]; then
  echo "run this installer with sudo or as root" >&2
  exit 1
fi
if [[ -z "${SERVER_URL}" ]]; then
  echo "--server is required" >&2
  exit 2
fi
if [[ -z "${AGENT_TOKEN}" ]]; then
  echo "--token is required" >&2
  exit 2
fi
if [[ -z "${AGENT_ID}" ]]; then
  AGENT_ID="agent-$(printf '%s' "${AGENT_HOSTNAME}" | slugify)"
fi
if [[ "${FORCE_DOWNLOAD}" == "1" || ! -x "${BINARY_PATH}" ]]; then
  download_agent_binary
fi
if [[ ! -x "${BINARY_PATH}" ]]; then
  echo "agent binary not found or not executable after download: ${BINARY_PATH}" >&2
  exit 1
fi
if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl is required for service installation" >&2
  exit 1
fi

install -d -m 0750 /etc/all-in-one-agent
umask 077
cat > "${CONFIG_FILE}" <<CONFIG
{
  "controlPlaneUrl": "${SERVER_URL}",
  "token": "${AGENT_TOKEN}",
  "agentId": "${AGENT_ID}",
  "name": "${AGENT_NAME}",
  "hostname": "${AGENT_HOSTNAME}",
  "labelsText": "${AGENT_LABELS}",
  "capabilitiesText": "${AGENT_CAPABILITIES}",
  "heartbeatInterval": "${HEARTBEAT_INTERVAL}",
  "metricsInterval": "${METRICS_INTERVAL}",
  "profileTaskInterval": "${PROFILE_TASK_INTERVAL}"
}
CONFIG
chmod 0600 "${CONFIG_FILE}"

cat > "${SERVICE_FILE}" <<SERVICE
[Unit]
Description=All-in-One Testing Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BINARY_PATH} --daemon --config ${CONFIG_FILE}
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
systemctl enable --now all-in-one-agent.service
if [[ "${FORCE_DOWNLOAD}" == "1" ]]; then
  systemctl restart all-in-one-agent.service
fi
systemctl --no-pager --full status all-in-one-agent.service || true
echo "all-in-one-agent installed and started as systemd service"
`

func handleAgentInstallScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(agentInstallScript))
}

func handleAgentBinaryDownload(w http.ResponseWriter, r *http.Request) {
	binaryName := chi.URLParam(r, "name")
	if binaryName == agentBinaryManifestName {
		handleAgentBinaryManifest(w, r)
		return
	}
	isChecksum := binaryName == agentBinaryLinuxAMD64+".sha256"
	if binaryName != agentBinaryLinuxAMD64 && !isChecksum {
		http.NotFound(w, r)
		return
	}

	binaryPath := filepath.Join(agentBinaryDir(), binaryName)
	file, err := os.Open(binaryPath)
	if err != nil {
		if isChecksum {
			serveGeneratedAgentBinaryChecksum(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	if isChecksum {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, binaryName))
	}
	http.ServeContent(w, r, binaryName, info.ModTime(), file)
}

func serveGeneratedAgentBinaryChecksum(w http.ResponseWriter, r *http.Request) {
	binaryPath := filepath.Join(agentBinaryDir(), agentBinaryLinuxAMD64)
	sum, err := sha256ForFile(binaryPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "%s  %s\n", sum, agentBinaryLinuxAMD64)
}

type agentBinaryManifest struct {
	Artifacts []agentBinaryManifestArtifact `json:"artifacts"`
}

type agentBinaryManifestArtifact struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	ChecksumURL string `json:"checksumUrl"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"sizeBytes"`
	Version     string `json:"version,omitempty"`
	Commit      string `json:"commit,omitempty"`
	Date        string `json:"date,omitempty"`
}

func handleAgentBinaryManifest(w http.ResponseWriter, r *http.Request) {
	manifest, found, err := loadAgentBinaryManifestFile()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if found {
		writeJSON(w, http.StatusOK, manifest)
		return
	}

	binaryPath := filepath.Join(agentBinaryDir(), agentBinaryLinuxAMD64)
	info, err := os.Stat(binaryPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	sum, err := sha256ForFile(binaryPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, agentBinaryManifest{
		Artifacts: []agentBinaryManifestArtifact{
			{
				Name:        agentBinaryLinuxAMD64,
				URL:         "/agent/binaries/" + agentBinaryLinuxAMD64,
				ChecksumURL: "/agent/binaries/" + agentBinaryLinuxAMD64 + ".sha256",
				SHA256:      sum,
				SizeBytes:   info.Size(),
			},
		},
	})
}

func sha256ForFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func latestAgentReleaseVersion() (string, error) {
	manifest, found, err := loadAgentBinaryManifestFile()
	if err != nil || !found {
		return "", err
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Name == agentBinaryLinuxAMD64 {
			return strings.TrimSpace(artifact.Version), nil
		}
	}
	return "", nil
}

func loadAgentBinaryManifestFile() (agentBinaryManifest, bool, error) {
	path := filepath.Join(agentBinaryDir(), agentBinaryManifestName)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return agentBinaryManifest{}, false, nil
	}
	if err != nil {
		return agentBinaryManifest{}, false, err
	}
	defer file.Close()

	var manifest agentBinaryManifest
	if err := json.NewDecoder(file).Decode(&manifest); err != nil {
		return agentBinaryManifest{}, false, err
	}
	return manifest, true, nil
}

func agentBinaryDir() string {
	if dir := os.Getenv("AGENT_BINARY_DIR"); dir != "" {
		return dir
	}
	return filepath.Join("dist", "agents")
}
