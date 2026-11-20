package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/seunggabi/claude-dashboard/internal/config"
	"github.com/seunggabi/claude-dashboard/internal/conversation"
	"github.com/seunggabi/claude-dashboard/internal/monitor"
	"github.com/seunggabi/claude-dashboard/internal/session"
	"github.com/seunggabi/claude-dashboard/internal/styles"
	"github.com/seunggabi/claude-dashboard/internal/tmux"
	"github.com/seunggabi/claude-dashboard/internal/ui"
)

// validSessionName matches only safe tmux session name characters.
var validSessionName = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

// Version is set by main.go at build time.
var Version = "dev"

// View represents the current view.
type View int

const (
	ViewDashboard View = iota
	ViewLogs
	ViewDetail
	ViewCreate
	ViewHelp
	ViewOverview
)

// Model is the main Bubble Tea model.
type Model struct {
	// Core
	manager  *session.Manager
	sessions []session.Session
	cfg      *config.Config

	// UI state
	view         View
	cursor       int
	scrollOffset int
	width        int
	height       int
	err          error

	// Kill confirmation. killTargets holds the sessions named in confirmMsg;
	// it is non-nil only while the prompt is shown.
	confirmMsg  string
	killTargets []string

	// Sub-views
	logView    ui.LogView
	createForm ui.CreateForm
	filterText textinput.Model
	filtering  bool

	// Filter
	filterQuery string

	// Overview: conversation summaries by session name
	summaries map[string]conversation.Summary

	// Attach target (set when user wants to attach, triggers Quit)
	attachTarget string
}

// SessionsMsg carries refreshed session list.
type SessionsMsg struct {
	Sessions []session.Session
	Err      error
}

// KillMsg signals session was killed.
type KillMsg struct {
	Err error
}

// CreateMsg signals session was created.
type CreateMsg struct {
	Err error
}

// SummariesMsg carries conversation summaries for the overview.
type SummariesMsg map[string]conversation.Summary

// LogsMsg carries log content.
type LogsMsg struct {
	Content string
	Err     error
}

// New creates a new app model.
func New() (Model, error) {
	client, err := tmux.NewClient()
	if err != nil {
		return Model{}, fmt.Errorf("tmux is required: %w", err)
	}

	cfg := config.Load()
	mgr := session.NewManager(client)

	filterInput := textinput.New()
	filterInput.Placeholder = "filter..."
	filterInput.CharLimit = 50
	filterInput.Width = 30

	m := Model{
		manager:    mgr,
		cfg:        cfg,
		view:       ViewDashboard,
		filterText: filterInput,
	}

	return m, nil
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.refreshSessions,
		monitor.TickCmd(m.cfg.RefreshInterval),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.view == ViewLogs {
			m.logView.SetSize(m.width, m.height)
		}
		return m, nil

	case monitor.TickMsg:
		cmds := []tea.Cmd{m.refreshSessions, monitor.TickCmd(m.cfg.RefreshInterval)}
		if m.view == ViewOverview {
			cmds = append(cmds, m.loadSummaries())
		}
		return m, tea.Batch(cmds...)

	case SummariesMsg:
		m.summaries = msg
		return m, nil

	case SessionsMsg:
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.sessions = msg.Sessions
		}
		m.clampCursor()
		return m, nil

	case KillMsg:
		if msg.Err != nil {
			m.err = msg.Err
		}
		return m, m.refreshSessions

	case CreateMsg:
		if msg.Err != nil {
			m.createForm.Err = msg.Err.Error()
			return m, nil
		}
		m.view = ViewDashboard
		return m, m.refreshSessions

	case LogsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.logView.SetContent(msg.Content)
		return m, nil

	case tea.KeyMsg:
		m.err = nil // Clear error on any key press
		return m.handleKey(msg)
	}

	// Update sub-components
	return m.updateSubComponents(msg)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global quit
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Confirm mode
	if m.killTargets != nil {
		return m.handleConfirmKey(msg)
	}

	// Filter mode
	if m.filtering {
		return m.handleFilterKey(msg)
	}

	// View-specific
	switch m.view {
	case ViewDashboard:
		return m.handleDashboardKey(msg)
	case ViewLogs:
		return m.handleLogsKey(msg)
	case ViewDetail:
		return m.handleDetailKey(msg)
	case ViewCreate:
		return m.handleCreateKey(msg)
	case ViewHelp:
		return m.handleHelpKey(msg)
	case ViewOverview:
		return m.handleOverviewKey(msg)
	}

	return m, nil
}

func (m Model) handleDashboardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.scrollOffset {
				m.scrollOffset = m.cursor
			}
		}
	case "down", "j":
		sessions := m.filteredSessions()
		if m.cursor < len(sessions)-1 {
			m.cursor++
			visibleRows := m.visibleSessionRows()
			if m.cursor >= m.scrollOffset+visibleRows {
				m.scrollOffset = m.cursor - visibleRows + 1
			}
		}
	case "enter":
		if s, ok := m.selected(); ok {
			if !s.Managed {
				m.err = errors.New("terminal sessions cannot be attached (not a tmux session)")
				return m, nil
			}
			if !validSessionName.MatchString(s.Name) {
				m.err = fmt.Errorf("invalid session name: %s", s.Name)
				return m, nil
			}
			// Quit Bubble Tea; the Run loop attaches, then restarts the dashboard.
			m.attachTarget = s.Name
			return m, tea.Quit
		}
	case "n":
		m.view = ViewCreate
		defaultDir := m.cfg.DefaultDir
		if defaultDir == "" {
			if cwd, err := os.Getwd(); err == nil {
				defaultDir = cwd
			} else {
				defaultDir, _ = os.UserHomeDir()
			}
		}
		m.createForm = ui.NewCreateForm(defaultDir)
		return m, m.createForm.NameInput.Focus()
	case "K":
		if s, ok := m.selected(); ok {
			m.confirmKill(s)
		}
	case "ctrl+k":
		var idle []string
		for _, s := range m.sessions {
			if s.Managed && s.Status == session.StatusIdle {
				idle = append(idle, s.Name)
			}
		}
		if len(idle) == 0 {
			m.err = errors.New("no idle sessions to kill")
			return m, nil
		}
		m.killTargets = idle
		m.confirmMsg = fmt.Sprintf("Kill %d idle session(s)? (y/n)", len(idle))
	case "l":
		if s, ok := m.selected(); ok {
			return m, m.openLogs(s)
		}
	case "d":
		if _, ok := m.selected(); ok {
			m.view = ViewDetail
		}
	case "/":
		m.filtering = true
		m.filterText.SetValue(m.filterQuery)
		return m, m.filterText.Focus()
	case "r":
		return m, m.refreshSessions
	case "?":
		m.view = ViewHelp
	case "o":
		m.view = ViewOverview
		return m, m.loadSummaries()
	}
	return m, nil
}

// handleOverviewKey reuses the dashboard keys for moving, attaching,
// refreshing and quitting; esc or o goes back to the dashboard.
func (m Model) handleOverviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "o":
		m.view = ViewDashboard
	case "up", "k", "down", "j", "enter", "q":
		return m.handleDashboardKey(msg)
	case "r":
		return m, tea.Batch(m.refreshSessions, m.loadSummaries())
	}
	return m, nil
}

func (m Model) handleLogsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = ViewDashboard
		return m, nil
	case "q":
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.logView.Viewport, cmd = m.logView.Viewport.Update(msg)
		return m, cmd
	}
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = ViewDashboard
	case "q":
		return m, tea.Quit
	case "l":
		if s, ok := m.selected(); ok {
			return m, m.openLogs(s)
		}
	case "K":
		if s, ok := m.selected(); ok {
			m.confirmKill(s)
		}
	}
	return m, nil
}

func (m Model) handleCreateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.view = ViewDashboard
		return m, nil
	case "tab":
		m.createForm.FocusNext()
		return m, nil
	case "enter":
		if err := m.createForm.Validate(); err != nil {
			m.createForm.Err = err.Error()
			return m, nil
		}
		name, dir := m.createForm.Values()
		return m, m.createSession(name, dir)
	}

	// Update the focused input
	var cmd tea.Cmd
	if m.createForm.FocusIdx == 0 {
		m.createForm.NameInput, cmd = m.createForm.NameInput.Update(msg)
	} else {
		m.createForm.DirInput, cmd = m.createForm.DirInput.Update(msg)
	}
	return m, cmd
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "?", "q":
		m.view = ViewDashboard
	}
	return m, nil
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		targets := m.killTargets
		m.killTargets = nil
		return m, m.killSessions(targets)
	case "n", "N", "esc":
		m.killTargets = nil
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filterQuery = m.filterText.Value()
		m.filtering = false
		m.cursor = 0
		return m, nil
	case "esc":
		m.filterQuery = ""
		m.filtering = false
		m.cursor = 0
		return m, nil
	}

	var cmd tea.Cmd
	m.filterText, cmd = m.filterText.Update(msg)
	m.filterQuery = m.filterText.Value()
	m.clampCursor()
	return m, cmd
}

func (m Model) updateSubComponents(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.view == ViewLogs {
		var cmd tea.Cmd
		m.logView.Viewport, cmd = m.logView.Viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	sessions := m.filteredSessions()
	var b strings.Builder

	// Title bar
	title := styles.Title.Render(" claude-dashboard ")
	ver := lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(Version)
	b.WriteString(title + " " + ver + "\n")

	// Error
	if m.err != nil {
		b.WriteString(styles.Error.Render(fmt.Sprintf("  Error: %v", m.err)))
		b.WriteString("\n")
	}

	// Main content
	contentHeight := m.height - 4 // title + status + help
	switch m.view {
	case ViewDashboard:
		visibleRows := m.visibleSessionRows()
		content := ui.RenderDashboard(sessions, m.cursor, m.width, m.scrollOffset, visibleRows)
		b.WriteString(content)
		lines := strings.Count(content, "\n")
		for i := lines; i < contentHeight; i++ {
			b.WriteString("\n")
		}
	case ViewLogs:
		b.WriteString(ui.RenderLogView(m.logView, m.width))
	case ViewDetail:
		if s, ok := m.selected(); ok {
			b.WriteString(ui.RenderDetail(&s, m.width))
		}
	case ViewCreate:
		b.WriteString(ui.RenderCreateForm(m.createForm, m.width))
	case ViewHelp:
		b.WriteString(ui.RenderHelp(m.width))
	case ViewOverview:
		content := ui.RenderOverview(sessions, m.summaries, m.cursor, m.width, contentHeight)
		b.WriteString(content)
		b.WriteString(strings.Repeat("
", max(0, contentHeight-strings.Count(content, "
"))))
	}

	// Confirm overlay
	if m.killTargets != nil {
		b.WriteString("\n")
		b.WriteString(styles.Confirm.Render("  " + m.confirmMsg))
	}

	// Filter bar
	if m.filtering {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  / %s", m.filterText.View()))
	}

	// Status bar
	viewName := m.viewName()
	b.WriteString("\n")
	b.WriteString(ui.StatusBar(m.width, len(sessions), viewName, m.filterQuery))
	b.WriteString("\n")
	helpContext := viewName
	switch {
	case m.killTargets != nil:
		helpContext = "confirm"
	case m.filtering:
		helpContext = "filter"
	}
	b.WriteString(ui.HelpBar(m.width, helpContext))

	return b.String()
}

func (m Model) viewName() string {
	switch m.view {
	case ViewDashboard:
		return "dashboard"
	case ViewLogs:
		return "logs"
	case ViewDetail:
		return "detail"
	case ViewCreate:
		return "create"
	case ViewHelp:
		return "help"
	case ViewOverview:
		return "overview"
	default:
		return "dashboard"
	}
}

func (m Model) filteredSessions() []session.Session {
	return session.FilterSessions(m.sessions, m.filterQuery)
}

// selected returns the session under the cursor, if any.
func (m Model) selected() (session.Session, bool) {
	sessions := m.filteredSessions()
	if m.cursor < 0 || m.cursor >= len(sessions) {
		return session.Session{}, false
	}
	return sessions[m.cursor], true
}

// clampCursor keeps the cursor and scroll offset inside the filtered list.
// Call it whenever the session list or the filter changes.
func (m *Model) clampCursor() {
	n := len(m.filteredSessions())
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if maxOffset := n - m.visibleSessionRows(); m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	if m.scrollOffset > m.cursor {
		m.scrollOffset = m.cursor
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

// confirmKill opens the kill prompt for one session. The name is stored now,
// so a list refresh before the answer cannot change which session is killed.
func (m *Model) confirmKill(s session.Session) {
	if !s.Managed {
		m.err = errors.New("terminal sessions cannot be killed from dashboard")
		return
	}
	m.killTargets = []string{s.Name}
	m.confirmMsg = fmt.Sprintf("Kill session '%s'? (y/n)", s.Name)
}

// openLogs switches to the log view: pane capture for tmux sessions,
// conversation history for terminal sessions.
func (m *Model) openLogs(s session.Session) tea.Cmd {
	m.view = ViewLogs
	m.logView = ui.NewLogView(s.Name, m.width, m.height)
	if s.Managed {
		return m.fetchLogs(s.Name)
	}
	return m.fetchConversation(s.Path)
}

// visibleSessionRows returns how many session rows fit in the content area.
// Subtracts: title(1) + error(1) + header(1) + status(1) + help(1) + padding(1) = 6
func (m Model) visibleSessionRows() int {
	rows := m.height - 6
	if rows < 1 {
		rows = 1
	}
	return rows
}

// Commands

func (m Model) refreshSessions() tea.Msg {
	sessions, err := m.manager.List(context.Background())
	return SessionsMsg{Sessions: sessions, Err: err}
}

func (m Model) killSessions(names []string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var failed []string
		for _, name := range names {
			if err := m.manager.Kill(ctx, name); err != nil {
				failed = append(failed, err.Error())
			}
		}
		if len(failed) > 0 {
			return KillMsg{Err: errors.New(strings.Join(failed, "; "))}
		}
		return KillMsg{}
	}
}

func (m Model) createSession(name, dir string) tea.Cmd {
	return func() tea.Msg {
		err := m.manager.Create(context.Background(), name, dir, "")
		return CreateMsg{Err: err}
	}
}

func (m Model) fetchLogs(name string) tea.Cmd {
	return func() tea.Msg {
		content, err := m.manager.GetLogs(context.Background(), name, m.cfg.LogHistory)
		return LogsMsg{Content: content, Err: err}
	}
}

// loadSummaries reads the conversation summary of every session with a
// working directory.
func (m Model) loadSummaries() tea.Cmd {
	sessions := m.sessions
	return func() tea.Msg {
		out := SummariesMsg{}
		for _, s := range sessions {
			if s.Path == "" {
				continue
			}
			if sum, err := conversation.ReadSummary(s.Path); err == nil {
				out[s.Name] = sum
			}
		}
		return out
	}
}

func (m Model) fetchConversation(path string) tea.Cmd {
	return func() tea.Msg {
		content, err := m.manager.GetConversation(path, 50)
		return LogsMsg{Content: content, Err: err}
	}
}

// cleanDA1 detects and removes DA1 residue (?6c) from a tmux pane.
// It polls every 100ms for up to 2 seconds, cleaning immediately when found.
func cleanDA1(name string) {
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		out, err := exec.Command("tmux", "capture-pane", "-t", name, "-p").Output()
		if err != nil {
			continue
		}
		content := string(out)
		if strings.Contains(content, "[?6c") {
			_ = exec.Command("tmux", "send-keys", "-t", name,
				"BSpace", "BSpace", "BSpace", "BSpace").Run()
			_ = exec.Command("tmux", "refresh-client").Run()
			return
		}
		if strings.Contains(content, "?6c") {
			_ = exec.Command("tmux", "send-keys", "-t", name,
				"BSpace", "BSpace", "BSpace").Run()
			_ = exec.Command("tmux", "refresh-client").Run()
			return
		}
	}
}

// DrainStdin reads and discards any pending data on stdin (e.g. DA1 response).
// Exported so main.go can call it at startup.
func DrainStdin() {
	fd := int(os.Stdin.Fd())
	_ = syscall.SetNonblock(fd, true)
	buf := make([]byte, 1024)
	os.Stdin.Read(buf)
	time.Sleep(50 * time.Millisecond)
	os.Stdin.Read(buf)
	_ = syscall.SetNonblock(fd, false)
}

// Run starts the TUI application.
func Run() error {
	for {
		// Drain any pending DA1 responses before starting TUI
		DrainStdin()

		m, err := New()
		if err != nil {
			return err
		}

		p := tea.NewProgram(m,
			tea.WithAltScreen(),
		)

		result, err := p.Run()
		if err != nil {
			return err
		}

		model := result.(Model)
		if model.attachTarget == "" {
			return nil // Normal quit
		}

		// Bubble Tea has fully exited alt screen. Enable mouse scroll, attach,
		// and loop back to the dashboard once the user detaches.
		name := model.attachTarget
		_ = exec.Command("tmux", "set-option", "-t", name, "mouse", "on").Run()
		_ = attach(name)

		// Inside tmux the client has moved to the session; there is no detach to
		// return from, and exiting closes the popup.
		if insideTmux() {
			return nil
		}
	}
}

// insideTmux reports whether claude-dashboard runs inside a tmux client.
func insideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// attach hands the terminal to tmux until the user detaches.
func attach(name string) error {
	// Show the session name in its dashboard color. Session options only, so
	// the global tmux config and other sessions are left alone.
	left := fmt.Sprintf("#[bg=colour%s,fg=black,bold] #S #[default] ", styles.SessionColor(name))
	_ = exec.Command("tmux",
		"set-option", "-t", name, "status-left", left, ";",
		"set-option", "-t", name, "status-left-length", "40").Run()

	// Inside tmux (e.g. the Ctrl+B g popup) attach-session would nest, so move
	// this client to the session instead.
	// ponytail: switch-client picks tmux's current client; pass -c if several
	// clients share one session and the wrong one moves.
	if insideTmux() {
		return exec.Command("tmux", "switch-client", "-t", name).Run()
	}


	// Drain stdin right before attach to consume any pending DA1 response.
	DrainStdin()
	// Background: detect and clean DA1 residue (?6c) from pane
	go cleanDA1(name)
	// TERM=tmux-256color prevents the DA1 query.
	cmd := exec.Command("tmux", "attach-session", "-t", name)
	cmd.Env = append(os.Environ(), "TERM=tmux-256color")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ExecAttach attaches to a tmux session (used by the CLI attach and new
// commands). Unlike the dashboard it leaves the session's mouse mode alone.
func ExecAttach(name string) error {
	if !validSessionName.MatchString(name) {
		return fmt.Errorf("invalid session name: %s", name)
	}
	return attach(name)
}

// StatusSummary returns the waiting and active session counts as tmux status
// bar text, or "" when there is nothing to report or tmux is unavailable.
func StatusSummary() string {
	client, err := tmux.NewClient()
	if err != nil {
		return ""
	}
	sessions, err := session.NewManager(client).List(context.Background())
	if err != nil {
		return ""
	}
	return formatStatusSummary(sessions)
}

// formatStatusSummary renders counts with tmux style codes. Waiting is a badge
// with its own background so it shows on any status bar color. The trailing
// separator keeps status-right tidy when the summary is empty.
func formatStatusSummary(sessions []session.Session) string {
	var waiting, active int
	for _, s := range sessions {
		switch s.Status {
		case session.StatusWaiting:
			waiting++
		case session.StatusActive:
			active++
		}
	}

	var parts []string
	if waiting > 0 {
		parts = append(parts, fmt.Sprintf("#[fg=black,bg=yellow,bold] ◎ %d waiting #[default]", waiting))
	}
	if active > 0 {
		parts = append(parts, fmt.Sprintf("● %d active", active))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ") + " | "
}

// CreateSession creates a new Claude session from CLI (non-TUI).
func CreateSession(name, projectDir, claudeArgs string) error {
	client, err := tmux.NewClient()
	if err != nil {
		return fmt.Errorf("tmux is required: %w", err)
	}
	mgr := session.NewManager(client)
	return mgr.Create(context.Background(), name, projectDir, claudeArgs)
}
