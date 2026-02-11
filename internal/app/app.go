package app

import (
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
	confirmMsg   string
	confirming   bool

	// Sub-views
	logView    ui.LogView
	createForm ui.CreateForm
	filterText textinput.Model
	filtering  bool

	// Filter
	filterQuery string

	// Attach target (set when user wants to attach, triggers Quit)
	attachTarget string
}

// SessionsMsg carries refreshed session list.
type SessionsMsg struct {
	Sessions []session.Session
	Err      error
}

// AttachMsg signals to attach to a session.
type AttachMsg struct {
	Name string
}

// KillMsg signals session was killed.
type KillMsg struct {
	Err error
}

// CreateMsg signals session was created.
type CreateMsg struct {
	Err error
}

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
		return m, tea.Batch(
			m.refreshSessions,
			monitor.TickCmd(m.cfg.RefreshInterval),
		)

	case SessionsMsg:
		if msg.Err != nil {
			m.err = msg.Err
		} else {
			m.sessions = msg.Sessions
			// Update resource info
			for i := range m.sessions {
				if m.sessions[i].PID != "" {
					info := monitor.GetChildProcessInfo(m.sessions[i].PID)
					m.sessions[i].CPU = info.CPU
					m.sessions[i].Memory = info.Memory
				}
			}
		}
		if m.cursor >= len(m.sessions) && m.cursor > 0 {
			m.cursor = len(m.sessions) - 1
		}
		return m, nil

	case KillMsg:
		if msg.Err != nil {
			m.err = msg.Err
		}
		m.confirming = false
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

	case AttachMsg:
		if !validSessionName.MatchString(msg.Name) {
			m.err = fmt.Errorf("invalid session name: %s", msg.Name)
			return m, nil
		}
		// Set attach target and quit Bubble Tea.
		// Run() loop will drain stdin, then run tmux attach, then restart.
		m.attachTarget = msg.Name
		return m, tea.Quit

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
	if m.confirming {
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
		sessions := m.filteredSessions()
		if len(sessions) > 0 && m.cursor < len(sessions) {
			if !sessions[m.cursor].Managed {
				m.err = fmt.Errorf("terminal sessions cannot be attached (not a tmux session)")
				return m, nil
			}
			return m, m.attachSession(sessions[m.cursor].Name)
		}
	case "n":
		m.view = ViewCreate
		m.createForm = ui.NewCreateForm(m.cfg.DefaultDir)
		return m, m.createForm.NameInput.Focus()
	case "K":
		sessions := m.filteredSessions()
		if len(sessions) > 0 && m.cursor < len(sessions) {
			if !sessions[m.cursor].Managed {
				m.err = fmt.Errorf("terminal sessions cannot be killed from dashboard")
				return m, nil
			}
			m.confirming = true
			m.confirmMsg = fmt.Sprintf("Kill session '%s'? (y/n)", sessions[m.cursor].Name)
		}
	case "l":
		sessions := m.filteredSessions()
		if len(sessions) > 0 && m.cursor < len(sessions) {
			s := sessions[m.cursor]
			m.view = ViewLogs
			m.logView = ui.NewLogView(s.Name, m.width, m.height)
			if s.Managed {
				return m, m.fetchLogs(s.Name)
			}
			return m, m.fetchConversation(s.Path)
		}
	case "d":
		sessions := m.filteredSessions()
		if len(sessions) > 0 && m.cursor < len(sessions) {
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
		sessions := m.filteredSessions()
		if m.cursor < len(sessions) {
			m.view = ViewLogs
			s := sessions[m.cursor]
			m.logView = ui.NewLogView(s.Name, m.width, m.height)
			return m, m.fetchLogs(s.Name)
		}
	case "K":
		sessions := m.filteredSessions()
		if m.cursor < len(sessions) {
			if !sessions[m.cursor].Managed {
				m.err = fmt.Errorf("terminal sessions cannot be killed from dashboard")
				return m, nil
			}
			m.confirming = true
			m.confirmMsg = fmt.Sprintf("Kill session '%s'? (y/n)", sessions[m.cursor].Name)
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
		sessions := m.filteredSessions()
		if m.cursor < len(sessions) {
			return m, m.killSession(sessions[m.cursor].Name)
		}
		m.confirming = false
	case "n", "N", "esc":
		m.confirming = false
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
		// Clamp scrollOffset
		if m.scrollOffset > len(sessions)-visibleRows {
			m.scrollOffset = len(sessions) - visibleRows
		}
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		content := ui.RenderDashboard(sessions, m.cursor, m.width, m.scrollOffset, visibleRows)
		b.WriteString(content)
		lines := strings.Count(content, "\n")
		for i := lines; i < contentHeight; i++ {
			b.WriteString("\n")
		}
	case ViewLogs:
		b.WriteString(ui.RenderLogView(m.logView, m.width))
	case ViewDetail:
		if m.cursor < len(sessions) {
			s := sessions[m.cursor]
			b.WriteString(ui.RenderDetail(&s, m.width))
		}
	case ViewCreate:
		b.WriteString(ui.RenderCreateForm(m.createForm, m.width))
	case ViewHelp:
		b.WriteString(ui.RenderHelp(m.width))
	}

	// Confirm overlay
	if m.confirming {
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
	b.WriteString(ui.HelpBar(m.width, viewName))

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
	default:
		return "dashboard"
	}
}

func (m Model) filteredSessions() []session.Session {
	return session.FilterSessions(m.sessions, m.filterQuery)
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
	sessions, err := m.manager.List()
	return SessionsMsg{Sessions: sessions, Err: err}
}

func (m Model) attachSession(name string) tea.Cmd {
	return func() tea.Msg {
		return AttachMsg{Name: name}
	}
}

func (m Model) killSession(name string) tea.Cmd {
	return func() tea.Msg {
		err := m.manager.Kill(name)
		return KillMsg{Err: err}
	}
}

func (m Model) createSession(name, dir string) tea.Cmd {
	return func() tea.Msg {
		err := m.manager.Create(name, dir, "")
		return CreateMsg{Err: err}
	}
}

func (m Model) fetchLogs(name string) tea.Cmd {
	return func() tea.Msg {
		content, err := m.manager.GetLogs(name, m.cfg.LogHistory)
		return LogsMsg{Content: content, Err: err}
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

		// Bubble Tea has fully exited alt screen.
		// Drain stdin to consume any DA1 response (?6c) from the terminal.
		DrainStdin()

		// Enable mouse scroll
		name := model.attachTarget
		_ = exec.Command("tmux", "set-option", "-t", name, "mouse", "on").Run()

		// Background: detect and clean DA1 residue (?6c) from pane
		go cleanDA1(name)

		// Run tmux attach with TERM=tmux-256color to prevent DA1 query
		cmd := exec.Command("tmux", "attach-session", "-t", name)
		cmd.Env = append(os.Environ(), "TERM=tmux-256color")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run()

		// User detached, loop back to dashboard
	}
}



// ExecAttach attaches to a tmux session (used by CLI `new` command).
func ExecAttach(name string) error {
	if !validSessionName.MatchString(name) {
		return fmt.Errorf("invalid session name: %s", name)
	}
	// Mouse mode is controlled globally via Ctrl+B m toggle
	// Don't override user's preference here
	// Drain stdin right before attach to consume any pending DA1 response
	DrainStdin()
	// Background: detect and clean DA1 residue (?6c) from pane
	go cleanDA1(name)
	proc := exec.Command("tmux", "attach-session", "-t", name)
	proc.Env = append(os.Environ(), "TERM=tmux-256color")
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	return proc.Run()
}

// CreateSession creates a new Claude session from CLI (non-TUI).
func CreateSession(name, projectDir, claudeArgs string) error {
	client, err := tmux.NewClient()
	if err != nil {
		return fmt.Errorf("tmux is required: %w", err)
	}
	mgr := session.NewManager(client)
	return mgr.Create(name, projectDir, claudeArgs)
}
