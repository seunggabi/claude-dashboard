package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/seunggabi/claude-dashboard/internal/styles"
)

// CreateForm holds the new session form state.
type CreateForm struct {
	NameInput textinput.Model
	DirInput  textinput.Model
	FocusIdx  int
	Err       string
}

// NewCreateForm creates a new session creation form.
func NewCreateForm(defaultDir string) CreateForm {
	nameInput := textinput.New()
	nameInput.Placeholder = "session-name"
	nameInput.CharLimit = 40
	nameInput.Width = 40
	nameInput.Focus()

	dirInput := textinput.New()
	dirInput.Placeholder = "/absolute/path/to/project"
	dirInput.CharLimit = 200
	dirInput.Width = 60
	if defaultDir != "" {
		dirInput.SetValue(defaultDir)
	}

	return CreateForm{
		NameInput: nameInput,
		DirInput:  dirInput,
		FocusIdx:  0,
	}
}

// FocusNext moves focus to the next input field.
func (f *CreateForm) FocusNext() {
	if f.FocusIdx == 0 {
		f.FocusIdx = 1
		f.NameInput.Blur()
		f.DirInput.Focus()
	} else {
		f.FocusIdx = 0
		f.DirInput.Blur()
		f.NameInput.Focus()
	}
}

// Values returns the form values.
func (f *CreateForm) Values() (name, dir string) {
	return strings.TrimSpace(f.NameInput.Value()), strings.TrimSpace(f.DirInput.Value())
}

// Validate checks if the form values are valid.
func (f *CreateForm) Validate() error {
	name, dir := f.Values()
	if name == "" {
		return fmt.Errorf("session name is required")
	}
	if strings.Contains(name, " ") {
		return fmt.Errorf("session name cannot contain spaces")
	}
	if dir == "" {
		return fmt.Errorf("project directory is required")
	}
	return nil
}

// RenderCreateForm renders the new session form.
func RenderCreateForm(form CreateForm, width int) string {
	var b strings.Builder

	title := styles.Title.Render(" New Session ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", width))
	b.WriteString("\n\n")

	// Name field
	nameLabel := styles.DetailLabel.Render("Name:")
	if form.FocusIdx == 0 {
		nameLabel = styles.StatusKey.Render("▸ Name:")
	}
	b.WriteString(fmt.Sprintf("  %s  %s\n", nameLabel, form.NameInput.View()))
	b.WriteString("\n")

	// Dir field
	dirLabel := styles.DetailLabel.Render("Directory:")
	if form.FocusIdx == 1 {
		dirLabel = styles.StatusKey.Render("▸ Directory:")
	}
	b.WriteString(fmt.Sprintf("  %s  %s\n", dirLabel, form.DirInput.View()))
	b.WriteString("\n")

	if form.Err != "" {
		b.WriteString(fmt.Sprintf("  %s\n", styles.Error.Render(form.Err)))
		b.WriteString("\n")
	}

	b.WriteString(strings.Repeat("─", width))
	b.WriteString("\n")
	b.WriteString(styles.Help.Render("  Session will run: claude in the specified directory"))
	b.WriteString("\n")
	b.WriteString(styles.Help.Render(fmt.Sprintf("  tmux session name: cd-%s", form.NameInput.Value())))
	b.WriteString("\n")

	return b.String()
}
