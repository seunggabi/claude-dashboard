package monitor

import (
	"os/exec"
	"strconv"
	"strings"
)

// ProcessInfo holds CPU and memory usage for a process.
type ProcessInfo struct {
	PID    string
	CPU    float64
	Memory float64
}

// ProcessTableEntry holds a single row from the process table.
type ProcessTableEntry struct {
	PID  string
	PPID string
	CPU  float64
	Mem  float64
	Args string
}

// ProcessTable is a map from PID to ProcessTableEntry.
type ProcessTable map[string]ProcessTableEntry

// GetProcessTable runs ps once and returns a full process table.
func GetProcessTable() ProcessTable {
	cmd := exec.Command("ps", "-eo", "pid,ppid,%cpu,%mem,args")
	out, err := cmd.Output()
	if err != nil {
		return ProcessTable{}
	}

	table := make(ProcessTable)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		cpu, _ := strconv.ParseFloat(fields[2], 64)
		mem, _ := strconv.ParseFloat(fields[3], 64)
		entry := ProcessTableEntry{
			PID:  fields[0],
			PPID: fields[1],
			CPU:  cpu,
			Mem:  mem,
			Args: strings.Join(fields[4:], " "),
		}
		table[entry.PID] = entry
	}
	return table
}

// GetChildProcessInfo returns aggregated CPU/memory for a PID and all children
// using a pre-built process table to avoid spawning multiple ps calls.
func GetChildProcessInfo(pid string, table ProcessTable) ProcessInfo {
	info := ProcessInfo{PID: pid}
	if pid == "" {
		return info
	}

	// Build children map from the process table.
	childrenOf := make(map[string][]string)
	for _, entry := range table {
		childrenOf[entry.PPID] = append(childrenOf[entry.PPID], entry.PID)
	}

	// BFS to aggregate CPU and memory for pid and all descendants.
	queue := []string{pid}
	visited := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true

		if entry, ok := table[current]; ok {
			info.CPU += entry.CPU
			info.Memory += entry.Mem
		}
		queue = append(queue, childrenOf[current]...)
	}

	return info
}
