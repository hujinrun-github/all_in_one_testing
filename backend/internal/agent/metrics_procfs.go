package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const procfsSectorSizeBytes = 512

type RuntimeMetricsSampler struct {
	mu     sync.Mutex
	procfs *ProcfsMetricsSampler
}

type ProcfsMetricsSampler struct {
	Root string
	now  func() time.Time

	mu       sync.Mutex
	previous procfsMetricsSnapshot
}

type procfsMetricsSnapshot struct {
	sampledAt    time.Time
	cpu          procfsCPU
	diskRead     uint64
	diskWrite    uint64
	networkRx    uint64
	networkTx    uint64
	processCPU   map[int]uint64
	hasBenchmark bool
}

type procfsCPU struct {
	total uint64
	idle  uint64
}

type procfsProcessSample struct {
	pid         int
	name        string
	cmdline     string
	cpuTicks    uint64
	rssBytes    int64
	fdCount     int
	threadCount int
}

func (sampler *RuntimeMetricsSampler) SampleMetrics(ctx context.Context, agentID string) (AgentMetrics, error) {
	root := strings.TrimSpace(os.Getenv("AIO_AGENT_PROC_ROOT"))
	if root == "" {
		root = "/proc"
	}
	if runtime.GOOS == "linux" || root != "/proc" {
		sampler.mu.Lock()
		if sampler.procfs == nil {
			sampler.procfs = &ProcfsMetricsSampler{Root: root}
		}
		procfs := sampler.procfs
		sampler.mu.Unlock()
		if metrics, err := procfs.SampleMetrics(ctx, agentID); err == nil {
			return metrics, nil
		}
	}
	return fallbackRuntimeMetrics(agentID), nil
}

func (sampler *ProcfsMetricsSampler) SampleMetrics(ctx context.Context, agentID string) (AgentMetrics, error) {
	if err := ctx.Err(); err != nil {
		return AgentMetrics{}, err
	}
	root := strings.TrimSpace(sampler.Root)
	if root == "" {
		root = "/proc"
	}
	now := time.Now
	if sampler.now != nil {
		now = sampler.now
	}
	sampledAt := now().UTC()

	cpu, err := parseProcfsCPU(readTextFile(filepath.Join(root, "stat")))
	if err != nil {
		return AgentMetrics{}, err
	}
	memoryUsage, err := parseProcfsMemoryUsage(readTextFile(filepath.Join(root, "meminfo")))
	if err != nil {
		return AgentMetrics{}, err
	}
	diskRead, diskWrite, err := parseProcfsDiskBytes(readTextFile(filepath.Join(root, "diskstats")))
	if err != nil {
		return AgentMetrics{}, err
	}
	networkRx, networkTx, err := parseProcfsNetworkBytes(readTextFile(filepath.Join(root, "net", "dev")))
	if err != nil {
		return AgentMetrics{}, err
	}
	processes, err := readProcfsProcesses(root)
	if err != nil {
		return AgentMetrics{}, err
	}

	current := procfsMetricsSnapshot{
		sampledAt:    sampledAt,
		cpu:          cpu,
		diskRead:     diskRead,
		diskWrite:    diskWrite,
		networkRx:    networkRx,
		networkTx:    networkTx,
		processCPU:   map[int]uint64{},
		hasBenchmark: true,
	}
	for _, process := range processes {
		current.processCPU[process.pid] = process.cpuTicks
	}

	sampler.mu.Lock()
	previous := sampler.previous
	sampler.previous = current
	sampler.mu.Unlock()

	elapsed := current.sampledAt.Sub(previous.sampledAt).Seconds()
	systemDelta := deltaUint64(current.cpu.total, previous.cpu.total)
	metrics := AgentMetrics{
		AgentID:             agentID,
		CollectedAt:         current.sampledAt.Format(time.RFC3339Nano),
		MemoryUsagePercent:  memoryUsage,
		Processes:           make([]AgentProcessMetric, 0, len(processes)),
		DiskReadBytesPerSec: 0,
	}
	if previous.hasBenchmark && elapsed > 0 {
		metrics.CPUUsagePercent = cpuUsagePercent(previous.cpu, current.cpu)
		metrics.DiskReadBytesPerSec = bytesPerSecond(previous.diskRead, current.diskRead, elapsed)
		metrics.DiskWriteBytesPerSec = bytesPerSecond(previous.diskWrite, current.diskWrite, elapsed)
		metrics.NetworkRxBytesPerSec = bytesPerSecond(previous.networkRx, current.networkRx, elapsed)
		metrics.NetworkTxBytesPerSec = bytesPerSecond(previous.networkTx, current.networkTx, elapsed)
	}

	for _, process := range processes {
		processMetric := AgentProcessMetric{
			PID:             process.pid,
			Name:            process.name,
			Cmdline:         process.cmdline,
			MemoryRSSBytes:  process.rssBytes,
			FDCount:         process.fdCount,
			ThreadCount:     process.threadCount,
			CPUUsagePercent: 0,
		}
		if previous.hasBenchmark && systemDelta > 0 {
			if previousProcessCPU, found := previous.processCPU[process.pid]; found {
				processMetric.CPUUsagePercent = float64(deltaUint64(process.cpuTicks, previousProcessCPU)) / float64(systemDelta) * 100
			}
		}
		metrics.Processes = append(metrics.Processes, processMetric)
	}
	sort.Slice(metrics.Processes, func(i, j int) bool {
		if metrics.Processes[i].CPUUsagePercent == metrics.Processes[j].CPUUsagePercent {
			return metrics.Processes[i].PID < metrics.Processes[j].PID
		}
		return metrics.Processes[i].CPUUsagePercent > metrics.Processes[j].CPUUsagePercent
	})
	if len(metrics.Processes) > 20 {
		metrics.Processes = metrics.Processes[:20]
	}
	return metrics, nil
}

func fallbackRuntimeMetrics(agentID string) AgentMetrics {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return AgentMetrics{
		AgentID:     agentID,
		CollectedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Processes: []AgentProcessMetric{
			{
				PID:            os.Getpid(),
				Name:           filepath.Base(os.Args[0]),
				Cmdline:        strings.Join(os.Args, " "),
				MemoryRSSBytes: int64(memory.Sys),
				ThreadCount:    runtime.NumGoroutine(),
			},
		},
	}
}

func readTextFile(path string) (string, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func parseProcfsCPU(text string, err error) (procfsCPU, error) {
	if err != nil {
		return procfsCPU{}, err
	}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}
		var cpu procfsCPU
		for index, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return procfsCPU{}, err
			}
			cpu.total += value
			if index == 3 || index == 4 {
				cpu.idle += value
			}
		}
		return cpu, nil
	}
	return procfsCPU{}, fmt.Errorf("procfs stat missing cpu line")
}

func parseProcfsMemoryUsage(text string, err error) (float64, error) {
	if err != nil {
		return 0, err
	}
	var totalKB uint64
	var availableKB uint64
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, err
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			totalKB = value
		case "MemAvailable":
			availableKB = value
		case "MemFree":
			if availableKB == 0 {
				availableKB = value
			}
		}
	}
	if totalKB == 0 {
		return 0, fmt.Errorf("procfs meminfo missing MemTotal")
	}
	if availableKB > totalKB {
		availableKB = totalKB
	}
	return float64(totalKB-availableKB) / float64(totalKB) * 100, nil
}

func parseProcfsDiskBytes(text string, err error) (uint64, uint64, error) {
	if err != nil {
		return 0, 0, err
	}
	var readBytes uint64
	var writeBytes uint64
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		name := fields[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") {
			continue
		}
		readSectors, err := strconv.ParseUint(fields[5], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		writeSectors, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		readBytes += readSectors * procfsSectorSizeBytes
		writeBytes += writeSectors * procfsSectorSizeBytes
	}
	return readBytes, writeBytes, nil
}

func parseProcfsNetworkBytes(text string, err error) (uint64, uint64, error) {
	if err != nil {
		return 0, 0, err
	}
	var rxBytes uint64
	var txBytes uint64
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		name, data, _ := strings.Cut(line, ":")
		if strings.TrimSpace(name) == "lo" {
			continue
		}
		fields := strings.Fields(data)
		if len(fields) < 16 {
			continue
		}
		rx, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		tx, err := strconv.ParseUint(fields[8], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		rxBytes += rx
		txBytes += tx
	}
	return rxBytes, txBytes, nil
}

func readProcfsProcesses(root string) ([]procfsProcessSample, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	processes := []procfsProcessSample{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		processRoot := filepath.Join(root, entry.Name())
		status, err := parseProcfsProcessStatus(readTextFile(filepath.Join(processRoot, "status")))
		if err != nil {
			continue
		}
		cpuTicks, err := parseProcfsProcessCPU(readTextFile(filepath.Join(processRoot, "stat")))
		if err != nil {
			continue
		}
		status.pid = pid
		status.cpuTicks = cpuTicks
		status.cmdline = normalizeProcfsCmdline(readTextFile(filepath.Join(processRoot, "cmdline")))
		status.fdCount = countProcfsFDs(filepath.Join(processRoot, "fd"))
		processes = append(processes, status)
	}
	return processes, nil
}

func normalizeProcfsCmdline(text string, err error) string {
	if err != nil {
		return ""
	}
	parts := strings.Split(text, "\x00")
	args := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			args = append(args, part)
		}
	}
	return strings.Join(args, " ")
}

func parseProcfsProcessStatus(text string, err error) (procfsProcessSample, error) {
	if err != nil {
		return procfsProcessSample{}, err
	}
	var process procfsProcessSample
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "Name":
			process.name = fields[1]
		case "VmRSS":
			value, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				return procfsProcessSample{}, err
			}
			process.rssBytes = value * 1024
		case "Threads":
			value, err := strconv.Atoi(fields[1])
			if err != nil {
				return procfsProcessSample{}, err
			}
			process.threadCount = value
		}
	}
	if process.name == "" {
		return procfsProcessSample{}, fmt.Errorf("process status missing Name")
	}
	return process, nil
}

func parseProcfsProcessCPU(text string, err error) (uint64, error) {
	if err != nil {
		return 0, err
	}
	closeParen := strings.LastIndex(text, ")")
	if closeParen < 0 || closeParen+2 >= len(text) {
		return 0, fmt.Errorf("process stat has invalid comm field")
	}
	fields := strings.Fields(text[closeParen+2:])
	if len(fields) < 13 {
		return 0, fmt.Errorf("process stat missing cpu fields")
	}
	utime, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, err
	}
	stime, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, err
	}
	return utime + stime, nil
}

func countProcfsFDs(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	return len(entries)
}

func cpuUsagePercent(previous procfsCPU, current procfsCPU) float64 {
	totalDelta := deltaUint64(current.total, previous.total)
	if totalDelta == 0 {
		return 0
	}
	idleDelta := deltaUint64(current.idle, previous.idle)
	if idleDelta > totalDelta {
		return 0
	}
	return float64(totalDelta-idleDelta) / float64(totalDelta) * 100
}

func bytesPerSecond(previous uint64, current uint64, elapsedSeconds float64) float64 {
	if elapsedSeconds <= 0 {
		return 0
	}
	return float64(deltaUint64(current, previous)) / elapsedSeconds
}

func deltaUint64(current uint64, previous uint64) uint64 {
	if current < previous {
		return 0
	}
	return current - previous
}
