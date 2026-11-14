package session

import (
	"context"
	"os/exec"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/seunggabi/claude-dashboard/internal/monitor"
	"github.com/seunggabi/claude-dashboard/internal/tmux"
)

// cwdCacheEntry holds a cached CWD result with expiry.
type cwdCacheEntry struct {
	path    string
	expires time.Time
}

var (
	cwdCache   = make(map[string]cwdCacheEntry)
	cwdCacheMu sync.Mutex
	cwdCacheTTL = 10 * time.Second
)

// Detector discovers Claude Code sessions from tmux.
type Detector struct {
	client *tmux.Client
}

// NewDetector creates a new session detector.
func NewDetector(client *tmux.Client) *Detector {
	return &Detector{client: client}
}

// Detect finds all Claude sessions: tmux sessions first, then Claude processes
// running in plain terminals.
func (d *Detector) Detect(ctx context.Context) ([]Session, error) {
	// One process table per refresh, shared by every step below.
	procTable := monitor.GetProcessTable()

	var sessions []Session

	// A tmux failure (e.g. no server running) still leaves terminal sessions to report.
	if output, err := d.client.ListSessions(ctx, tmux.SessionFormat); err == nil && output != "" {
		procChildren := buildProcChildren(procTable)

		for _, raw := range tmux.ParseSessions(output) {
			// Include sessions with the managed prefix or that contain claude in the name
			isNameMatch := strings.HasPrefix(raw.Name, SessionPrefix) || strings.Contains(strings.ToLower(raw.Name), "claude")
			if !isNameMatch && !d.client.HasClaudeProcess(ctx, raw.Name, procChildren) {
				continue
			}

			s := Session{
				Name:      raw.Name,
				Project:   extractProject(raw.Name, raw.Path),
				Status:    d.detectStatus(ctx, raw.Name, raw.Activity),
				StartedAt: raw.Created,
				Activity:  raw.Activity,
				Attached:  raw.Attached,
				Path:      raw.Path,
				Managed:   true,
			}
			if pid, err := d.client.GetSessionPID(ctx, raw.Name); err == nil {
				s.PID = pid
			}
			sessions = append(sessions, s)
		}
	}

	// Pane PIDs of the tmux sessions above, so their processes are not listed twice.
	panePIDs := make(map[string]bool)
	for _, s := range sessions {
		if s.PID != "" {
			panePIDs[s.PID] = true
		}
	}
	sessions = append(sessions, terminalSessions(procTable, panePIDs)...)

	for i := range sessions {
		if sessions[i].PID != "" {
			info := monitor.GetChildProcessInfo(sessions[i].PID, procTable)
			sessions[i].CPU = info.CPU
			sessions[i].Memory = info.Memory
		}
	}

	return sessions, nil
}

// terminalSessions finds Claude processes running outside the tmux sessions
// identified by panePIDs.
func terminalSessions(table monitor.ProcessTable, panePIDs map[string]bool) []Session {
	var sessions []Session
	for _, p := range table {
		// Only match processes where the executable base name is "claude"
		exe, _, _ := strings.Cut(p.Args, " ")
		if path.Base(exe) != "claude" {
			continue
		}

		// Skip background/detached processes (no TTY)
		if p.TTY == "??" || p.TTY == "?" {
			continue
		}

		// Skip Claude running in a tracked tmux pane, directly or under its shell
		if hasAncestorIn(p.PID, table, panePIDs) {
			continue
		}

		// Get working directory via lsof
		cwd := getProcessCWD(p.PID)
		project := ""
		if cwd != "" {
			project = path.Base(cwd)
		}

		sessions = append(sessions, Session{
			Name:    "terminal/" + p.TTY,
			Project: project,
			Status:  StatusTerminal,
			PID:     p.PID,
			Path:    cwd,
			Managed: false,
		})
	}

	// The process table is a map; sort so rows keep their place between refreshes.
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].Name != sessions[j].Name {
			return sessions[i].Name < sessions[j].Name
		}
		return sessions[i].PID < sessions[j].PID
	})

	return sessions
}

// hasAncestorIn reports whether pid, or any of its ancestors, is in set.
func hasAncestorIn(pid string, table monitor.ProcessTable, set map[string]bool) bool {
	// The depth cap guards against a PPID cycle in a malformed table.
	for depth := 0; pid != "" && depth < 64; depth++ {
		if set[pid] {
			return true
		}
		pid = table[pid].PPID
	}
	return false
}

// getProcessCWD gets the current working directory of a process.
// Results are cached with a 10-second TTL to avoid repeated lsof calls.
func getProcessCWD(pid string) string {
	cwdCacheMu.Lock()
	if entry, ok := cwdCache[pid]; ok && time.Now().Before(entry.expires) {
		cwdCacheMu.Unlock()
		return entry.path
	}
	cwdCacheMu.Unlock()

	cmd := exec.Command("lsof", "-a", "-p", pid, "-d", "cwd", "-Fn")
	out, err := cmd.Output()
	result := ""
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "n/") {
				result = line[1:]
				break
			}
		}
	}

	cwdCacheMu.Lock()
	cwdCache[pid] = cwdCacheEntry{path: result, expires: time.Now().Add(cwdCacheTTL)}
	cwdCacheMu.Unlock()

	return result
}

// detectStatus determines session status by examining activity timestamp and pane content:
// recent output is active, a visible question is waiting, anything else is idle.
func (d *Detector) detectStatus(ctx context.Context, name string, lastActivity time.Time) Status {
	// If activity is very recent (within 2 seconds), consider it active
	// This handles cases where output is streaming but prompt is not visible yet
	idleThreshold := 2 * time.Second
	if !lastActivity.IsZero() && time.Since(lastActivity) < idleThreshold {
		return StatusActive
	}

	// If no recent activity, check pane content to distinguish idle vs waiting
	content, err := d.client.CapturePaneContent(ctx, name, 20)
	if err != nil {
		return StatusIdle
	}

	// Check the last 20 lines for a question or confirmation prompt. The capture
	// is padded with blank lines up to the pane height; drop those first.
	lines := strings.Split(strings.TrimRight(content, " \t\n"), "\n")
	for i := len(lines) - 1; i >= 0 && i >= len(lines)-20; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasSuffix(line, "?") ||
			strings.Contains(line, "Y/n") || strings.Contains(line, "y/N") ||
			strings.Contains(line, "(y/n)") {
			return StatusWaiting
		}
	}

	return StatusIdle
}

// buildProcChildren converts a monitor.ProcessTable into the children map
// format expected by tmux.Client.HasClaudeProcess.
func buildProcChildren(table monitor.ProcessTable) map[string][]tmux.ProcEntry {
	entries := make([]struct{ PID, PPID, Args string }, 0, len(table))
	for _, e := range table {
		entries = append(entries, struct{ PID, PPID, Args string }{
			PID:  e.PID,
			PPID: e.PPID,
			Args: e.Args,
		})
	}
	return tmux.BuildProcChildren(entries)
}

// extractProject derives project name from session name or path.
func extractProject(name, path string) string {
	// If session has cd- prefix, use the rest as project name
	if strings.HasPrefix(name, SessionPrefix) {
		return strings.TrimPrefix(name, SessionPrefix)
	}

	// Use last directory component of path
	if path != "" {
		parts := strings.Split(strings.TrimRight(path, "/"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	return name
}
