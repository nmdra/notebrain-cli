package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nmdra/notebrain-cli/internal/store"
)

// ─── Dependencies ─────────────────────────────────────────────────

// SearchFunc is the function signature used to perform a vector search.
// It is decoupled from concrete types so tests can inject a stub.
type SearchFunc func(ctx context.Context, query string) ([]store.Result, error)

// ─── Messages ─────────────────────────────────────────────────────

// searchMsg is dispatched after the debounce timer fires.
type searchMsg struct {
	id    int64
	query string
}

// resultsMsg carries results (or an error) back to the Update loop.
type resultsMsg struct {
	id      int64
	results []store.Result
	err     error
}

// ─── Focus state ──────────────────────────────────────────────────

type focusState int

const (
	focusInput focusState = iota // keyboard goes to textinput
	focusList                    // keyboard goes to result list
)

// ─── Model ────────────────────────────────────────────────────────

// LiveSearchModel is a Bubble Tea model for interactive live semantic search.
// The user types a query; results update automatically after a short debounce.
//
// inflight is a *int64 pointer (heap-allocated) so that value-copies of the
// model (as Bubble Tea requires) do not violate sync/atomic noCopy rules.
type LiveSearchModel struct {
	input     textinput.Model
	list      list.Model
	searchFn  SearchFunc
	vaultName string
	useEditor bool
	focus     focusState

	// inflight is a pointer to an int64 counter used for debounce ID tracking.
	// Using a pointer keeps the model copyable (Bubble Tea's interface requirement)
	// while still allowing safe atomic operations.
	inflight *int64
	debounce time.Duration
	loading  bool
	status   string

	quitting bool
	width    int
	height   int
}

// ─── Styles ───────────────────────────────────────────────────────

var (
	lsInputBorderFocused = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#5DCAA5")).
				Padding(0, 1)

	lsInputBorderBlurred = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#555555")).
				Padding(0, 1)

	lsStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B4B2A9")).
			Italic(true)

	lsHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555555"))

	lsLoadingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5DCAA5"))

	lsTabActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#5DCAA5")).
				Bold(true)

	lsTabInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888780"))
)

// ─── Constructor ──────────────────────────────────────────────────

// NewLiveSearch creates a LiveSearchModel.
// searchFn wraps embed+query so the TUI has no direct dependency on the
// store/embedder packages.
func NewLiveSearch(searchFn SearchFunc, vaultName string, limit int, initialQuery string, useEditor bool) LiveSearchModel {
	ti := textinput.New()
	ti.Placeholder = "type to search your vault…"
	ti.Prompt = "  "
	ti.Focus()
	if initialQuery != "" {
		ti.SetValue(initialQuery)
	}

	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false) // we own the search; list filtering would interfere
	l.SetShowHelp(false)

	var inflightCounter int64
	return LiveSearchModel{
		input:     ti,
		list:      l,
		searchFn:  searchFn,
		vaultName: vaultName,
		useEditor: useEditor,
		focus:     focusInput,
		inflight:  &inflightCounter,
		debounce:  300 * time.Millisecond,
		status:    "start typing to search…",
	}
}

// ─── Init ─────────────────────────────────────────────────────────

func (m LiveSearchModel) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, 2)
	cmds = append(cmds, textinput.Blink)
	// If a pre-populated query was provided, trigger an immediate search.
	if q := m.input.Value(); q != "" {
		cmds = append(cmds, m.triggerSearch(q, 0))
	}
	return tea.Batch(cmds...)
}

// ─── Update ───────────────────────────────────────────────────────

func (m LiveSearchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.recalcLayout()

	case editorFinishedMsg:
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {

		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "esc":
			// If we're in list focus, escape returns to input focus
			if m.focus == focusList {
				m.focus = focusInput
				m.input.Focus()
				return m, textinput.Blink
			}
			// If we're in input focus, escape exits
			m.quitting = true
			return m, tea.Quit

		case "tab":
			m.focus = focusInput
			m.input.Focus()
			return m, textinput.Blink

		case "down":
			// Down from input moves focus to list
			if m.focus == focusInput && len(m.list.Items()) > 0 {
				m.focus = focusList
				m.input.Blur()
				return m, nil
			}
			if m.focus == focusList {
				var listCmd tea.Cmd
				m.list, listCmd = m.list.Update(msg)
				return m, listCmd
			}

		case "up":
			if m.focus == focusList {
				// If at top of list, move focus back to input
				if m.list.Index() == 0 {
					m.focus = focusInput
					m.input.Focus()
					return m, textinput.Blink
				}
				var listCmd tea.Cmd
				m.list, listCmd = m.list.Update(msg)
				return m, listCmd
			}

		case "enter":
			if m.focus == focusList {
				if item, ok := m.list.SelectedItem().(resultItem); ok {
					return m, openNote(m.vaultName, item.FilePath, m.useEditor)
				}
				return m, nil
			}

		case "o":
			if m.focus == focusList {
				if item, ok := m.list.SelectedItem().(resultItem); ok {
					return m, openNote(m.vaultName, item.FilePath, false)
				}
				return m, nil
			}
			fallthrough

		case "e":
			if m.focus == focusList {
				if item, ok := m.list.SelectedItem().(resultItem); ok {
					return m, openNote(m.vaultName, item.FilePath, true)
				}
				return m, nil
			}
			fallthrough

		default:
			// All other keys go to textinput when it has focus
			if m.focus == focusInput {
				prevVal := m.input.Value()
				var tiCmd tea.Cmd
				m.input, tiCmd = m.input.Update(msg)
				newVal := m.input.Value()

				var debounceCmd tea.Cmd
				if newVal != prevVal {
					id := atomic.AddInt64(m.inflight, 1)
					debounceCmd = m.triggerSearch(newVal, id)
					m.loading = true
					m.status = ""
				}
				return m, tea.Batch(tiCmd, debounceCmd)
			}
			// List focus: pass to list
			if m.focus == focusList {
				var listCmd tea.Cmd
				m.list, listCmd = m.list.Update(msg)
				return m, listCmd
			}
		}

	case searchMsg:
		// Only act if this ID is still current (guards against stale fires).
		if msg.id != atomic.LoadInt64(m.inflight) {
			return m, nil
		}
		if msg.query == "" {
			m.loading = false
			m.status = "start typing to search…"
			m.list.SetItems(nil)
			return m, nil
		}
		// Fire the actual search in a goroutine.
		searchID := msg.id
		searchFn := m.searchFn
		query := msg.query
		return m, func() tea.Msg {
			results, err := searchFn(context.Background(), query)
			return resultsMsg{id: searchID, results: results, err: err}
		}

	case resultsMsg:
		// Discard results from superseded searches.
		if msg.id != atomic.LoadInt64(m.inflight) {
			return m, nil
		}
		m.loading = false
		if msg.err != nil {
			m.status = lsStatusStyle.Render("⚠  " + msg.err.Error())
			return m, nil
		}
		items := make([]list.Item, len(msg.results))
		for i, r := range msg.results {
			items[i] = resultItem{r}
		}
		m.list.SetItems(items)
		switch len(msg.results) {
		case 0:
			m.status = lsStatusStyle.Render("no results")
		case 1:
			m.status = lsStatusStyle.Render("1 result")
		default:
			m.status = lsStatusStyle.Render(fmt.Sprintf("%d results", len(msg.results)))
		}
		return m, nil
	}

	// Default: pass through to textinput (e.g. cursor blink).
	if m.focus == focusInput {
		var tiCmd tea.Cmd
		m.input, tiCmd = m.input.Update(msg)
		return m, tiCmd
	}
	return m, nil
}

// recalcLayout recomputes input width and list height based on current terminal size.
func (m LiveSearchModel) recalcLayout() LiveSearchModel {
	if m.width == 0 {
		return m
	}
	// Input width: terminal width minus border (2) minus padding (2 each side = 4)
	inputWidth := max(m.width-6, 10)
	m.input.SetWidth(inputWidth)

	const (
		inputH  = 3 // rounded border = 2 lines + 1 content = 3
		statusH = 1
		helpH   = 1
		padding = 1
	)
	listH := max(m.height-inputH-statusH-helpH-padding, 3)
	m.list.SetSize(m.width, listH)
	return m
}

// triggerSearch returns a Cmd that waits debounce duration then sends a searchMsg.
func (m LiveSearchModel) triggerSearch(query string, id int64) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(m.debounce)
		return searchMsg{id: id, query: query}
	}
}

// ─── View ─────────────────────────────────────────────────────────

func (m LiveSearchModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	// Input box — border colour changes based on focus
	borderStyle := lsInputBorderFocused
	if m.focus == focusList {
		borderStyle = lsInputBorderBlurred
	}
	inputRendered := borderStyle.Render(m.input.View())

	// Status / loading line
	var statusLine string
	if m.loading {
		statusLine = lsLoadingStyle.Render("  ◌ searching…")
	} else {
		statusLine = "  " + m.status
	}

	// Focus hint tabs
	searchTab := lsTabInactiveStyle.Render("[ search ]")
	listTab := lsTabInactiveStyle.Render("[ results ]")
	if m.focus == focusInput {
		searchTab = lsTabActiveStyle.Render("[ search ]")
	} else {
		listTab = lsTabActiveStyle.Render("[ results ]")
	}
	tabs := "  " + searchTab + "  " + listTab

	// Help bar
	var help string
	if m.focus == focusInput {
		help = lsHelpStyle.Render("  ↓ → results · esc quit")
	} else {
		help = lsHelpStyle.Render("  ↑/↓ navigate · enter open · o obsidian · e editor · tab/↑ top → search · esc quit")
	}

	content := inputRendered + "\n" + statusLine + "\n" + tabs + "\n" + m.list.View() + "\n" + help
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// ─── Helpers ──────────────────────────────────────────────────────

// SuppressStderr redirects stderr to /dev/null for the duration of fn,
// then restores it. Used to silence ChromaDB/hnswlib integrity-check noise
// that would otherwise bleed into the TUI.
func SuppressStderr(fn func()) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		fn()
		return
	}
	defer func() { _ = devNull.Close() }()

	orig := os.Stderr
	os.Stderr = devNull
	// Also silence the C-library side via a dup2 if needed — redirect fd 2
	savedFd2, err2 := dupFd(2)
	if err2 == nil {
		_ = dup2(int(devNull.Fd()), 2)
		defer func() {
			_ = dup2(savedFd2, 2)
			_ = closeFd(savedFd2)
		}()
	}
	defer func() { os.Stderr = orig }()
	fn()
}

// SuppressOutputs redirects both stdout and stderr to /dev/null for the duration of fn,
// then restores them. Used to silence ChromaDB/hnswlib integrity-check and stats noise.
func SuppressOutputs(fn func()) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		fn()
		return
	}
	defer func() { _ = devNull.Close() }()

	origStderr := os.Stderr
	origStdout := os.Stdout
	os.Stderr = devNull
	os.Stdout = devNull

	// Silence C-library side via dup2 for stdout (1) and stderr (2)
	savedFd1, err1 := dupFd(1)
	savedFd2, err2 := dupFd(2)

	if err1 == nil {
		_ = dup2(int(devNull.Fd()), 1)
		defer func() {
			_ = dup2(savedFd1, 1)
			_ = closeFd(savedFd1)
		}()
	}
	if err2 == nil {
		_ = dup2(int(devNull.Fd()), 2)
		defer func() {
			_ = dup2(savedFd2, 2)
			_ = closeFd(savedFd2)
		}()
	}

	defer func() {
		os.Stderr = origStderr
		os.Stdout = origStdout
	}()

	fn()
}

// NullWriter returns an io.Writer that discards everything.
func NullWriter() io.Writer { return io.Discard }
