package agent

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProcfsMetricsSamplerComputesHostDeltasAndProcessMetrics(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	sampler := &ProcfsMetricsSampler{
		Root: root,
		now: func() time.Time {
			return now
		},
	}

	writeProcfsSnapshot(t, root, procfsSnapshot{
		stat:      "cpu  100 0 50 850 0 0 0 0 0 0\n",
		meminfo:   "MemTotal:       100000 kB\nMemAvailable:    25000 kB\n",
		diskstats: "   8       0 sda 10 0 1000 0 20 0 2000 0 0 0 0 0 0 0 0 0 0\n",
		netdev: `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 100000 0 0 0 0 0 0 0 200000 0 0 0 0 0 0 0
`,
		processes: []procfsProcessSnapshot{
			{
				pid:     "1234",
				stat:    "1234 (checkout) S 1 1 1 0 -1 4194560 100 0 0 0 20 10 0 0 20 0 7 0 1000 1000000 2048\n",
				status:  "Name:\tcheckout\nVmRSS:\t2048 kB\nThreads:\t7\n",
				cmdline: "checkout\x00--config=/etc/checkout/prod.yaml\x00--port=8080\x00",
				fd:      3,
			},
		},
	})
	if _, err := sampler.SampleMetrics(context.Background(), "agent-linux-01"); err != nil {
		t.Fatalf("expected first baseline sample to succeed, got %v", err)
	}

	now = now.Add(2 * time.Second)
	writeProcfsSnapshot(t, root, procfsSnapshot{
		stat:      "cpu  130 0 70 900 0 0 0 0 0 0\n",
		meminfo:   "MemTotal:       100000 kB\nMemAvailable:    25000 kB\n",
		diskstats: "   8       0 sda 10 0 3000 0 20 0 5000 0 0 0 0 0 0 0 0 0 0\n",
		netdev: `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 104000 0 0 0 0 0 0 0 206000 0 0 0 0 0 0 0
`,
		processes: []procfsProcessSnapshot{
			{
				pid:     "1234",
				stat:    "1234 (checkout) S 1 1 1 0 -1 4194560 100 0 0 0 35 15 0 0 20 0 7 0 1000 1000000 2048\n",
				status:  "Name:\tcheckout\nVmRSS:\t2048 kB\nThreads:\t7\n",
				cmdline: "checkout\x00--config=/etc/checkout/prod.yaml\x00--port=8080\x00",
				fd:      3,
			},
		},
	})

	metrics, err := sampler.SampleMetrics(context.Background(), "agent-linux-01")
	if err != nil {
		t.Fatalf("expected second sample to succeed, got %v", err)
	}
	if metrics.AgentID != "agent-linux-01" || metrics.CollectedAt == "" {
		t.Fatalf("expected agent id and collected time, got %#v", metrics)
	}
	assertFloatNear(t, "cpu usage", metrics.CPUUsagePercent, 50)
	assertFloatNear(t, "memory usage", metrics.MemoryUsagePercent, 75)
	assertFloatNear(t, "disk read bytes/sec", metrics.DiskReadBytesPerSec, 512000)
	assertFloatNear(t, "disk write bytes/sec", metrics.DiskWriteBytesPerSec, 768000)
	assertFloatNear(t, "network rx bytes/sec", metrics.NetworkRxBytesPerSec, 2000)
	assertFloatNear(t, "network tx bytes/sec", metrics.NetworkTxBytesPerSec, 3000)
	if len(metrics.Processes) != 1 {
		t.Fatalf("expected one process metric, got %#v", metrics.Processes)
	}
	process := metrics.Processes[0]
	if process.PID != 1234 || process.Name != "checkout" {
		t.Fatalf("expected checkout process, got %#v", process)
	}
	assertFloatNear(t, "process cpu usage", process.CPUUsagePercent, 20)
	if process.MemoryRSSBytes != 2048*1024 || process.FDCount != 3 || process.ThreadCount != 7 {
		t.Fatalf("expected process rss/fd/thread metrics, got %#v", process)
	}
	var marshaledProcesses []struct {
		Cmdline string `json:"cmdline"`
	}
	processJSON, err := json.Marshal(metrics.Processes)
	if err != nil {
		t.Fatalf("expected process metrics to marshal, got %v", err)
	}
	if err := json.Unmarshal(processJSON, &marshaledProcesses); err != nil {
		t.Fatalf("expected process JSON to decode, got %v", err)
	}
	if len(marshaledProcesses) != 1 || marshaledProcesses[0].Cmdline != "checkout --config=/etc/checkout/prod.yaml --port=8080" {
		t.Fatalf("expected process cmdline to be sampled and normalized, got %#v", marshaledProcesses)
	}
}

func TestProcfsMetricsSamplerDoesNotInventCPUForNewProcess(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 14, 10, 30, 0, 0, time.UTC)
	sampler := &ProcfsMetricsSampler{
		Root: root,
		now: func() time.Time {
			return now
		},
	}

	writeProcfsSnapshot(t, root, procfsSnapshot{
		stat:      "cpu  100 0 0 900 0 0 0 0 0 0\n",
		meminfo:   "MemTotal:       100000 kB\nMemAvailable:    50000 kB\n",
		diskstats: "   8       0 sda 10 0 1000 0 20 0 2000 0 0 0 0 0 0 0 0 0 0\n",
		netdev: `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 100000 0 0 0 0 0 0 0 200000 0 0 0 0 0 0 0
`,
	})
	if _, err := sampler.SampleMetrics(context.Background(), "agent-linux-01"); err != nil {
		t.Fatalf("expected first baseline sample to succeed, got %v", err)
	}

	now = now.Add(2 * time.Second)
	writeProcfsSnapshot(t, root, procfsSnapshot{
		stat:      "cpu  150 0 0 950 0 0 0 0 0 0\n",
		meminfo:   "MemTotal:       100000 kB\nMemAvailable:    50000 kB\n",
		diskstats: "   8       0 sda 10 0 1000 0 20 0 2000 0 0 0 0 0 0 0 0 0 0\n",
		netdev: `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
  eth0: 100000 0 0 0 0 0 0 0 200000 0 0 0 0 0 0 0
`,
		processes: []procfsProcessSnapshot{
			{
				pid:    "5678",
				stat:   "5678 (worker) S 1 1 1 0 -1 4194560 100 0 0 0 50 10 0 0 20 0 4 0 1000 1000000 1024\n",
				status: "Name:\tworker\nVmRSS:\t1024 kB\nThreads:\t4\n",
				fd:     1,
			},
		},
	})

	metrics, err := sampler.SampleMetrics(context.Background(), "agent-linux-01")
	if err != nil {
		t.Fatalf("expected second sample to succeed, got %v", err)
	}
	if len(metrics.Processes) != 1 {
		t.Fatalf("expected one process metric, got %#v", metrics.Processes)
	}
	assertFloatNear(t, "new process cpu usage", metrics.Processes[0].CPUUsagePercent, 0)
}

type procfsSnapshot struct {
	stat      string
	meminfo   string
	diskstats string
	netdev    string
	processes []procfsProcessSnapshot
}

type procfsProcessSnapshot struct {
	pid     string
	stat    string
	status  string
	cmdline string
	fd      int
}

func writeProcfsSnapshot(t *testing.T, root string, snapshot procfsSnapshot) {
	t.Helper()

	writeFile(t, filepath.Join(root, "stat"), snapshot.stat)
	writeFile(t, filepath.Join(root, "meminfo"), snapshot.meminfo)
	writeFile(t, filepath.Join(root, "diskstats"), snapshot.diskstats)
	writeFile(t, filepath.Join(root, "net", "dev"), snapshot.netdev)
	for _, process := range snapshot.processes {
		processRoot := filepath.Join(root, process.pid)
		writeFile(t, filepath.Join(processRoot, "stat"), process.stat)
		writeFile(t, filepath.Join(processRoot, "status"), process.status)
		if process.cmdline != "" {
			writeFile(t, filepath.Join(processRoot, "cmdline"), process.cmdline)
		}
		fdRoot := filepath.Join(processRoot, "fd")
		if err := os.RemoveAll(fdRoot); err != nil {
			t.Fatalf("expected fd directory cleanup to succeed, got %v", err)
		}
		if err := os.MkdirAll(fdRoot, 0o755); err != nil {
			t.Fatalf("expected fd directory to be created, got %v", err)
		}
		for index := 0; index < process.fd; index++ {
			writeFile(t, filepath.Join(fdRoot, string(rune('0'+index))), "")
		}
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("expected directory for %s to be created, got %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("expected %s to be written, got %v", path, err)
	}
}

func assertFloatNear(t *testing.T, name string, actual float64, expected float64) {
	t.Helper()
	if math.Abs(actual-expected) > 0.001 {
		t.Fatalf("expected %s %.3f, got %.3f", name, expected, actual)
	}
}
