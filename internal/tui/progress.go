package tui

import (
	"fmt"
	"io"
	"os"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
)

var (
	hasDarkBG = lipgloss.HasDarkBackground(os.Stdin, os.Stdout)
	lightDark = lipgloss.LightDark(hasDarkBG)

	colorGood  = lightDark(lipgloss.Color("#0F6E56"), lipgloss.Color("#5DCAA5")) // teal
	colorMuted = lightDark(lipgloss.Color("#888780"), lipgloss.Color("#B4B2A9")) // gray
)

type ProgressModel struct {
	bar      progress.Model
	total    int
	done     int
	current  string // currently processing note title
	updates  <-chan ProgressUpdate
	finished bool
}

type ProgressUpdate struct {
	Done    int
	Total   int
	Current string
	Final   bool
}

func NewProgressModel(total int, updates <-chan ProgressUpdate) ProgressModel {
	return ProgressModel{
		bar:     progress.New(),
		total:   total,
		updates: updates,
	}
}

func (m ProgressModel) Init() tea.Cmd {
	return waitForUpdate(m.updates)
}

func waitForUpdate(ch <-chan ProgressUpdate) tea.Cmd {
	return func() tea.Msg {
		u, ok := <-ch
		if !ok {
			return ProgressUpdate{Final: true}
		}
		return u
	}
}

func (m ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch u := msg.(type) {
	case tea.KeyMsg:
		switch u.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		}

	case ProgressUpdate:
		if u.Final {
			m.finished = true
			return m, tea.Quit
		}
		m.done, m.total, m.current = u.Done, u.Total, u.Current
		cmd := m.bar.SetPercent(float64(m.done) / float64(m.total))
		return m, tea.Batch(cmd, waitForUpdate(m.updates))

	case progress.FrameMsg:
		newModel, cmd := m.bar.Update(msg)
		m.bar = newModel
		return m, cmd
	}
	return m, nil
}

func (m ProgressModel) View() tea.View {
	if m.finished {
		return tea.NewView(lipgloss.NewStyle().Foreground(colorGood).Render("✓ Ingest complete\n"))
	}
	label := lipgloss.NewStyle().Foreground(colorMuted).Render(m.current)
	return tea.NewView(fmt.Sprintf("%s\n%s  (%d/%d)\n", m.bar.View(), label, m.done, m.total))
}

func RunProgress(stdin io.Reader, stdout io.Writer, totalFiles int, progressCh <-chan ProgressUpdate) error {
	isTTY := false
	if f, ok := stdout.(*os.File); ok {
		isTTY = term.IsTerminal(f.Fd())
	}
	if !isTTY || os.Getenv("TERM") == "dumb" {
		for u := range progressCh {
			if u.Final {
				break
			}
		}
		_, _ = fmt.Fprint(stdout, "✓ Ingest complete\n\n")
		return nil
	}

	pUI := tea.NewProgram(
		NewProgressModel(totalFiles, progressCh),
		tea.WithInput(stdin),
		tea.WithOutput(stdout),
	)
	_, err := pUI.Run()
	return err
}
