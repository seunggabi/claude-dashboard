package session

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/seunggabi/claude-dashboard/internal/conversation"
	"github.com/seunggabi/claude-dashboard/internal/tmux"
)

// Manager handles session CRUD operations.
type Manager struct {
	client   *tmux.Client
	detector *Detector
}

// NewManager creates a new session manager.
func NewManager(client *tmux.Client) *Manager {
	return &Manager{
		client:   client,
		detector: NewDetector(client),
	}
}

// List returns all Claude sessions.
func (m *Manager) List() ([]Session, error) {
	return m.detector.Detect()
}

// Create creates a new Claude session.
func (m *Manager) Create(name, projectDir string) error {
	sessionName := SessionPrefix + name
	command := "claude"

	err := m.client.NewSession(sessionName, projectDir, command)
	if err != nil {
		return fmt.Errorf("failed to create session %s: %w", sessionName, err)
	}
	return nil
}

// CreateWithArgs creates a new Claude session with additional claude arguments.
func (m *Manager) CreateWithArgs(name, projectDir, claudeArgs string) error {
	sessionName := SessionPrefix + name
	command := "claude"
	if claudeArgs != "" {
		command = "claude " + claudeArgs
	}

	err := m.client.NewSession(sessionName, projectDir, command)
	if err != nil {
		return fmt.Errorf("failed to create session %s: %w", sessionName, err)
	}
	return nil
}

// Kill terminates a session.
func (m *Manager) Kill(name string) error {
	err := m.client.KillSession(name)
	if err != nil {
		return fmt.Errorf("failed to kill session %s: %w", name, err)
	}
	return nil
}

// Attach attaches to a session (returns cmd to execute).
func (m *Manager) Attach(name string) *exec.Cmd {
	return m.client.AttachSession(name)
}

// GetLogs returns the captured pane content for a session.
func (m *Manager) GetLogs(name string, lines int) (string, error) {
	if lines <= 0 {
		lines = 1000
	}
	return m.client.CapturePaneContent(name, lines)
}

// GetConversation returns the formatted conversation log for a session.
func (m *Manager) GetConversation(path string, maxMessages int) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no working directory for session")
	}
	messages, err := conversation.ReadConversation(path, maxMessages)
	if err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "No conversation messages found.", nil
	}
	return conversation.FormatConversation(messages), nil
}

// SendCommand sends a command to a session.
func (m *Manager) SendCommand(name, command string) error {
	return m.client.SendKeys(name, command)
}

// Refresh re-detects all sessions and returns them.
func (m *Manager) Refresh() ([]Session, error) {
	return m.detector.Detect()
}

// FindByName finds a session by name.
func (m *Manager) FindByName(sessions []Session, name string) *Session {
	for i := range sessions {
		if sessions[i].Name == name {
			return &sessions[i]
		}
	}
	return nil
}

// FilterSessions filters sessions by query string.
func FilterSessions(sessions []Session, query string) []Session {
	if query == "" {
		return sessions
	}
	query = strings.ToLower(query)
	filtered := make([]Session, 0)
	for _, s := range sessions {
		if strings.Contains(strings.ToLower(s.Name), query) ||
			strings.Contains(strings.ToLower(s.Project), query) ||
			strings.Contains(strings.ToLower(string(s.Status)), query) ||
			strings.Contains(strings.ToLower(s.Path), query) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
