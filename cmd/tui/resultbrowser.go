package tui

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// resultItem adapts store.Result to the list.Item interface.
type resultItem struct {
	store.Result
}

func (i resultItem) Title() string { return i.Result.Title }
func (i resultItem) Description() string {
	if i.Extra != "" {
		return fmt.Sprintf("score=%.3f  %s", i.Score, i.Extra)
	}
	return fmt.Sprintf("score=%.3f", i.Score)
}
func (i resultItem) FilterValue() string { return i.Result.Title }

// Model is the Bubble Tea model for the interactive result browser.
type Model struct {
	list      list.Model
	vaultName string
	useEditor bool
	quitting  bool
}

// NewResultBrowser creates a new interactive search results browser.
func NewResultBrowser(header, vaultName string, results []store.Result, useEditor bool) Model {
	items := make([]list.Item, len(results))
	for i, r := range results {
		items[i] = resultItem{r}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = header
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true) // "/" to fuzzy-filter results live

	return Model{list: l, vaultName: vaultName, useEditor: useEditor}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)

	case editorFinishedMsg:
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			if item, ok := m.list.SelectedItem().(resultItem); ok {
				return m, openNote(m.vaultName, item.FilePath, m.useEditor)
			}
			return m, nil

		case "o":
			if item, ok := m.list.SelectedItem().(resultItem); ok {
				return m, openNote(m.vaultName, item.FilePath, false)
			}
			return m, nil

		case "e":
			if item, ok := m.list.SelectedItem().(resultItem); ok {
				return m, openNote(m.vaultName, item.FilePath, true)
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	help := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888780")).
		Render("\n  ↑/↓ navigate · / filter · enter open · o obsidian · e editor · q quit")
	v := tea.NewView(m.list.View() + help)
	v.AltScreen = true
	return v
}
