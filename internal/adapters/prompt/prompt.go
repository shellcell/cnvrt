package promptadapter

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
	"golang.org/x/term"
)

type Prompt struct {
	source io.Reader
	in     *bufio.Reader
	out    io.Writer

	titleStyle  lipgloss.Style
	numberStyle lipgloss.Style
	hintStyle   lipgloss.Style
	badgeStyle  lipgloss.Style
	promptStyle lipgloss.Style
}

func New(in io.Reader, out io.Writer) *Prompt {
	return &Prompt{
		source:      in,
		in:          bufio.NewReader(in),
		out:         out,
		titleStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		numberStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		hintStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		badgeStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1),
		promptStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")),
	}
}

func (p *Prompt) SelectFiles(ctx context.Context, files []domain.FileRef) ([]domain.FileRef, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to select")
	}
	if p.hasTerminal() {
		return p.selectFilesTerminal(ctx, files)
	}

	return p.selectFilesFallback(ctx, files)
}

func (p *Prompt) SelectFormat(ctx context.Context, choices []ports.FormatChoice) (domain.Format, error) {
	if len(choices) == 0 {
		return "", fmt.Errorf("no formats to select")
	}
	if p.hasTerminal() {
		return p.selectFormatTerminal(ctx, choices)
	}

	return p.selectFormatFallback(ctx, choices)
}

func (p *Prompt) SelectOutputLocation(ctx context.Context, currentDir string) (ports.OutputLocation, error) {
	if p.hasTerminal() {
		var selected ports.OutputLocation
		field := huh.NewSelect[ports.OutputLocation]().
			Title("Save outputs").
			Description("Current directory: "+currentDir).
			Options(
				huh.NewOption("Current directory", ports.OutputLocationCurrent),
				huh.NewOption("Beside each source", ports.OutputLocationSource),
			).
			Value(&selected)

		if err := p.runField(ctx, field); err != nil {
			return "", err
		}
		return selected, nil
	}

	fmt.Fprintln(p.out, p.titleStyle.Render("Save outputs"))
	fmt.Fprintf(p.out, "  %s Current directory %s\n", p.numberStyle.Render("1."), p.hintStyle.Render(currentDir))
	fmt.Fprintf(p.out, "  %s Beside each source\n", p.numberStyle.Render("2."))
	fmt.Fprint(p.out, p.promptStyle.Render("Location: "))

	index, err := p.readSingleIndex(ctx, 2)
	if err != nil {
		return "", err
	}
	if index == 1 {
		return ports.OutputLocationSource, nil
	}
	return ports.OutputLocationCurrent, nil
}

func (p *Prompt) SelectArchiveAction(ctx context.Context, file domain.FileRef) (domain.ArchiveAction, error) {
	if p.hasTerminal() {
		var selected domain.ArchiveAction
		field := huh.NewSelect[domain.ArchiveAction]().
			Title("Archive detected").
			Description(file.Name+" ["+file.Format.String()+"]").
			Options(
				huh.NewOption("Extract archive", domain.ArchiveActionExtract),
				huh.NewOption("Choose output format", domain.ArchiveActionConvert),
				huh.NewOption("Cancel", domain.ArchiveActionCancel),
			).
			Value(&selected)

		if err := p.runField(ctx, field); err != nil {
			return "", err
		}
		return selected, nil
	}

	fmt.Fprintln(p.out, p.titleStyle.Render("Archive detected"))
	fmt.Fprintf(p.out, "  %s %s %s\n", p.numberStyle.Render("1."), "Extract archive", p.badgeStyle.Render(file.Format.String()))
	fmt.Fprintf(p.out, "  %s %s\n", p.numberStyle.Render("2."), "Choose output format")
	fmt.Fprintf(p.out, "  %s %s\n", p.numberStyle.Render("3."), "Cancel")
	fmt.Fprint(p.out, p.promptStyle.Render("Action: "))

	index, err := p.readSingleIndex(ctx, 3)
	if err != nil {
		return "", err
	}

	return archiveActionFromIndex(index), nil
}

func (p *Prompt) SelectSameFormatAction(ctx context.Context, format domain.Format) (domain.TransformAction, error) {
	if p.hasTerminal() {
		var selected domain.TransformAction
		field := huh.NewSelect[domain.TransformAction]().
			Title("Input and output format match").
			Description("Choose what to do with "+format.String()+" files.").
			Options(
				huh.NewOption("Compress", domain.ActionCompress),
				huh.NewOption("Resize", domain.ActionResize),
				huh.NewOption("Convert/copy", domain.ActionConvert),
			).
			Value(&selected)

		if err := p.runField(ctx, field); err != nil {
			return "", err
		}
		return selected, nil
	}

	fmt.Fprintln(p.out, p.titleStyle.Render("Input and output format match"))
	fmt.Fprintf(p.out, "  %s %s %s\n", p.numberStyle.Render("1."), "Compress", p.badgeStyle.Render(format.String()))
	fmt.Fprintf(p.out, "  %s %s\n", p.numberStyle.Render("2."), "Resize")
	fmt.Fprintf(p.out, "  %s %s\n", p.numberStyle.Render("3."), "Convert/copy")
	fmt.Fprint(p.out, p.promptStyle.Render("Action: "))

	index, err := p.readSingleIndex(ctx, 3)
	if err != nil {
		return "", err
	}

	return sameFormatActionFromIndex(index), nil
}

func (p *Prompt) AskResize(ctx context.Context) (string, error) {
	if p.hasTerminal() {
		var value string
		field := huh.NewInput().
			Title("Resize").
			Description("Examples: 800x, x600, 1280x720, 50%").
			Placeholder("800x").
			Validate(func(value string) error {
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("resize value is required")
				}
				return nil
			}).
			Value(&value)

		if err := p.runField(ctx, field); err != nil {
			return "", err
		}
		return strings.TrimSpace(value), nil
	}

	fmt.Fprintln(p.out, p.hintStyle.Render("Examples: 800x, x600, 1280x720, 50%"))
	fmt.Fprint(p.out, p.promptStyle.Render("Resize: "))
	value, err := p.readLine(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("resize value is required")
	}
	return value, nil
}

func (p *Prompt) AskQuality(ctx context.Context, defaultQuality int) (int, error) {
	if p.hasTerminal() {
		value := strconv.Itoa(defaultQuality)
		field := huh.NewInput().
			Title("Quality").
			Description("Enter quality from 1 to 100.").
			Placeholder(strconv.Itoa(defaultQuality)).
			Validate(func(value string) error {
				if strings.TrimSpace(value) == "" {
					return nil
				}
				quality, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("invalid quality: %s", value)
				}
				if quality < 1 || quality > 100 {
					return fmt.Errorf("quality must be between 1 and 100")
				}
				return nil
			}).
			Value(&value)

		if err := p.runField(ctx, field); err != nil {
			return 0, err
		}
		if strings.TrimSpace(value) == "" {
			return defaultQuality, nil
		}
		return strconv.Atoi(value)
	}

	fmt.Fprintf(p.out, "%s %s\n", p.hintStyle.Render("Enter quality from 1 to 100."), p.hintStyle.Render(fmt.Sprintf("Default: %d", defaultQuality)))
	fmt.Fprint(p.out, p.promptStyle.Render("Quality: "))
	value, err := p.readLine(ctx)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(value) == "" {
		return defaultQuality, nil
	}

	quality, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid quality: %s", value)
	}
	if quality < 1 || quality > 100 {
		return 0, fmt.Errorf("quality must be between 1 and 100")
	}
	return quality, nil
}

func (p *Prompt) selectFilesTerminal(ctx context.Context, files []domain.FileRef) ([]domain.FileRef, error) {
	root := filepath.Dir(files[0].Path)
	model := newFilePickerModel(p, root, files)
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(p.source), tea.WithOutput(p.out))
	result, err := program.Run()
	if err != nil {
		return nil, err
	}
	finished, ok := result.(*filePickerModel)
	if !ok {
		return nil, fmt.Errorf("unexpected file picker result")
	}
	if finished.aborted {
		return nil, ports.ErrUserAborted
	}
	if len(finished.result) == 0 {
		return nil, fmt.Errorf("select at least one file")
	}
	return finished.result, nil
}

func (p *Prompt) selectFormatTerminal(ctx context.Context, choices []ports.FormatChoice) (domain.Format, error) {
	model := newFormatPickerModel(p, choices)
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(p.source), tea.WithOutput(p.out))
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	finished, ok := result.(*formatPickerModel)
	if !ok {
		return "", fmt.Errorf("unexpected format picker result")
	}
	if finished.aborted {
		return "", ports.ErrUserAborted
	}
	if finished.result == "" {
		return "", fmt.Errorf("select an output format")
	}
	return finished.result, nil
}

type filePickerEntry struct {
	file   domain.FileRef
	parent bool
}

type filePickerPosition struct {
	cursor int
	offset int
	filter string
}

type filePickerModel struct {
	in            io.Reader
	out           io.Writer
	startDir      string
	currentDir    string
	entries       []filePickerEntry
	selected      map[string]domain.FileRef
	positions     map[string]filePickerPosition
	cursor        int
	offset        int
	height        int
	width         int
	filter        string
	filtering     bool
	pendingG      bool
	err           string
	aborted       bool
	result        []domain.FileRef
	titleStyle    lipgloss.Style
	numberStyle   lipgloss.Style
	hintStyle     lipgloss.Style
	badgeStyle    lipgloss.Style
	promptStyle   lipgloss.Style
	selectedStyle lipgloss.Style
	dimStyle      lipgloss.Style
}

func newFilePickerModel(p *Prompt, root string, files []domain.FileRef) *filePickerModel {
	model := &filePickerModel{
		in:            p.source,
		out:           p.out,
		startDir:      filepath.Clean(root),
		currentDir:    filepath.Clean(root),
		selected:      map[string]domain.FileRef{},
		positions:     map[string]filePickerPosition{},
		height:        p.listHeight(8),
		width:         p.terminalWidth(90),
		titleStyle:    p.titleStyle,
		numberStyle:   p.numberStyle,
		hintStyle:     p.hintStyle,
		badgeStyle:    p.badgeStyle,
		promptStyle:   p.promptStyle,
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
		dimStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	}
	model.entries = entriesFromFiles(files, false)
	return model
}

func (m *filePickerModel) Init() tea.Cmd { return nil }

func (m *filePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = maxInt(4, msg.Height-8)
		m.ensureCursorVisible()
	case tea.KeyPressMsg:
		keyName := msg.String()
		if keyName == "ctrl+c" || keyName == "q" {
			m.aborted = true
			return m, tea.Quit
		}
		if m.filtering {
			m.updateFilter(msg)
			return m, nil
		}

		switch keyName {
		case "/":
			m.pendingG = false
			m.filtering = true
			m.err = ""
		case "up", "k":
			m.pendingG = false
			m.move(-1)
		case "down", "j":
			m.pendingG = false
			m.move(1)
		case "right", "l":
			m.pendingG = false
			m.enterDirectory()
		case "left", "h":
			m.pendingG = false
			m.openParentDir()
		case "pgup", "pageup", "ctrl+up", "alt+up", "option+up", "ctrl+u":
			m.pendingG = false
			m.move(-m.height)
		case "pgdown", "pagedown", "ctrl+down", "alt+down", "option+down", "ctrl+d":
			m.pendingG = false
			m.move(m.height)
		case "g":
			if m.pendingG {
				m.goStart()
				m.pendingG = false
			} else {
				m.pendingG = true
			}
		case "home", "ctrl+a":
			m.pendingG = false
			m.goStart()
		case "end", "G", "shift+g", "ctrl+e":
			m.pendingG = false
			m.goEnd()
		case "space", "x":
			m.pendingG = false
			m.toggleCurrent()
		case "a", "A":
			m.pendingG = false
			m.selectAllVisible()
		case "c", "C":
			m.pendingG = false
			m.clearSelection()
		case "enter":
			m.pendingG = false
			m.enterCurrent()
			if len(m.result) > 0 {
				return m, tea.Quit
			}
		case "tab":
			m.pendingG = false
			m.submit()
			if len(m.result) > 0 {
				return m, tea.Quit
			}
		default:
			m.pendingG = false
		}
	}
	return m, nil
}

func (m *filePickerModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("Select input files"))
	b.WriteString("\n")
	b.WriteString(m.hintStyle.Render("enter/right opens dirs, left goes up, space/x selects, a selects filtered, c clears selection, / filters, esc clears, q quits"))
	b.WriteString("\n")
	b.WriteString(m.dimStyle.Render(filepath.Clean(m.currentDir)))
	b.WriteString("\n")
	if m.filtering || m.filter != "" {
		b.WriteString(m.promptStyle.Render("Filter: "))
		b.WriteString(m.filter)
		b.WriteString("\n")
	}

	visible := m.visible()
	if len(visible) == 0 {
		b.WriteString(m.dimStyle.Render("  No matching files or directories."))
		b.WriteString("\n")
	} else {
		end := minInt(len(visible), m.offset+m.height)
		for i := m.offset; i < end; i++ {
			entry := visible[i]
			cursor := "  "
			if i == m.cursor {
				cursor = m.numberStyle.Render("› ")
			}
			mark := "[ ]"
			if _, ok := m.selected[entry.file.Path]; ok {
				mark = m.selectedStyle.Render("[x]")
			}
			label := entry.file.Name
			if entry.file.Format == domain.FormatDir && !strings.HasSuffix(label, "/") {
				label += "/"
			}
			if entry.parent {
				mark = "   "
			}
			b.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, mark, formatLabel(entry.file.Format), label))
		}
		if len(visible) > m.height {
			b.WriteString(m.dimStyle.Render(fmt.Sprintf("  Showing %d-%d of %d", m.offset+1, end, len(visible))))
			b.WriteString("\n")
		}
	}
	if len(m.selected) > 0 {
		b.WriteString(m.hintStyle.Render(fmt.Sprintf("Selected: %d", len(m.selected))))
		b.WriteString("\n")
	}
	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.err))
		b.WriteString("\n")
	}
	return tea.NewView(b.String())
}

func (m *filePickerModel) updateFilter(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		m.filter = ""
		m.filtering = false
		m.cursor = 0
		m.offset = 0
	case "enter":
		m.filtering = false
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.cursor = 0
			m.offset = 0
		}
	default:
		if text := msg.Key().Text; text != "" {
			m.filter += text
			m.cursor = 0
			m.offset = 0
		}
	}
}

func (m *filePickerModel) visible() []filePickerEntry {
	if strings.TrimSpace(m.filter) == "" {
		return m.entries
	}
	needle := strings.ToLower(strings.TrimSpace(m.filter))
	var result []filePickerEntry
	for _, entry := range m.entries {
		label := strings.ToLower(entry.file.Name + " " + entry.file.Format.String())
		if strings.Contains(label, needle) {
			result = append(result, entry)
		}
	}
	return result
}

func (m *filePickerModel) move(delta int) {
	visible := m.visible()
	if len(visible) == 0 {
		return
	}
	m.cursor = clampInt(m.cursor+delta, 0, len(visible)-1)
	m.ensureCursorVisible()
	m.err = ""
}

func (m *filePickerModel) goStart() {
	m.cursor = 0
	m.ensureCursorVisible()
	m.err = ""
}

func (m *filePickerModel) goEnd() {
	m.cursor = maxInt(0, len(m.visible())-1)
	m.ensureCursorVisible()
	m.err = ""
}

func (m *filePickerModel) ensureCursorVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *filePickerModel) current() (filePickerEntry, bool) {
	visible := m.visible()
	if len(visible) == 0 || m.cursor < 0 || m.cursor >= len(visible) {
		return filePickerEntry{}, false
	}
	return visible[m.cursor], true
}

func (m *filePickerModel) toggleCurrent() {
	entry, ok := m.current()
	if !ok || entry.parent {
		return
	}
	if _, selected := m.selected[entry.file.Path]; selected {
		delete(m.selected, entry.file.Path)
	} else {
		m.selected[entry.file.Path] = entry.file
	}
	m.err = ""
}

func (m *filePickerModel) selectAllVisible() {
	count := 0
	for _, entry := range m.visible() {
		if entry.parent {
			continue
		}
		m.selected[entry.file.Path] = entry.file
		count++
	}
	if count == 0 {
		m.err = "no filtered files or directories to select"
		return
	}
	m.err = fmt.Sprintf("selected %d filtered item(s)", count)
}

func (m *filePickerModel) clearSelection() {
	if len(m.selected) == 0 {
		m.err = "selection is already empty"
		return
	}
	m.selected = map[string]domain.FileRef{}
	m.err = "selection cleared"
}

func (m *filePickerModel) enterCurrent() {
	entry, ok := m.current()
	if !ok {
		return
	}
	_, selected := m.selected[entry.file.Path]
	if entry.file.Format == domain.FormatDir && !selected {
		m.openDir(entry.file.Path)
		return
	}
	m.submit()
}

func (m *filePickerModel) enterDirectory() {
	entry, ok := m.current()
	if !ok {
		return
	}
	if entry.file.Format != domain.FormatDir {
		return
	}
	m.openDir(entry.file.Path)
}

func (m *filePickerModel) openParentDir() {
	if !canShowParent(m.currentDir, m.startDir) {
		return
	}
	m.openDir(filepath.Dir(filepath.Clean(m.currentDir)))
}

func (m *filePickerModel) submit() {
	if len(m.selected) == 0 {
		m.err = "select at least one file or directory"
		return
	}
	m.result = make([]domain.FileRef, 0, len(m.selected))
	for _, file := range m.selected {
		m.result = append(m.result, file)
	}
	sort.Slice(m.result, func(i, j int) bool { return m.result[i].Path < m.result[j].Path })
}

func (m *filePickerModel) openDir(path string) {
	target := filepath.Clean(path)
	entries, err := readPickerDir(target, m.startDir)
	if err != nil {
		m.err = err.Error()
		return
	}
	m.savePosition()
	m.currentDir = target
	m.entries = entries
	m.restorePosition(target)
	m.filtering = false
	m.err = ""
}

func (m *filePickerModel) savePosition() {
	m.positions[pickerDirKey(m.currentDir)] = filePickerPosition{
		cursor: m.cursor,
		offset: m.offset,
		filter: m.filter,
	}
}

func (m *filePickerModel) restorePosition(path string) {
	position, ok := m.positions[pickerDirKey(path)]
	if ok {
		m.cursor = position.cursor
		m.offset = position.offset
		m.filter = position.filter
	} else {
		m.cursor = 0
		m.offset = 0
		m.filter = ""
	}
	m.clampPosition()
}

func (m *filePickerModel) clampPosition() {
	visible := m.visible()
	if len(visible) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	m.cursor = clampInt(m.cursor, 0, len(visible)-1)
	maxOffset := maxInt(0, len(visible)-m.height)
	m.offset = clampInt(m.offset, 0, maxOffset)
	m.ensureCursorVisible()
}

func pickerDirKey(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

type formatPickerModel struct {
	choices       []ports.FormatChoice
	cursor        int
	offset        int
	height        int
	filter        string
	filtering     bool
	pendingG      bool
	err           string
	aborted       bool
	result        domain.Format
	titleStyle    lipgloss.Style
	numberStyle   lipgloss.Style
	hintStyle     lipgloss.Style
	available     lipgloss.Style
	unavailable   lipgloss.Style
	selectedStyle lipgloss.Style
}

func newFormatPickerModel(p *Prompt, choices []ports.FormatChoice) *formatPickerModel {
	return &formatPickerModel{
		choices:       choices,
		height:        p.listHeight(6),
		titleStyle:    p.titleStyle,
		numberStyle:   p.numberStyle,
		hintStyle:     p.hintStyle,
		available:     lipgloss.NewStyle(),
		unavailable:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		selectedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
	}
}

func (m *formatPickerModel) Init() tea.Cmd { return nil }

func (m *formatPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = maxInt(4, msg.Height-6)
		m.ensureCursorVisible()
	case tea.KeyPressMsg:
		keyName := msg.String()
		if keyName == "ctrl+c" || keyName == "q" {
			m.aborted = true
			return m, tea.Quit
		}
		if m.filtering {
			m.updateFilter(msg)
			return m, nil
		}
		switch keyName {
		case "/":
			m.pendingG = false
			m.filtering = true
			m.err = ""
		case "up", "k":
			m.pendingG = false
			m.move(-1)
		case "down", "j":
			m.pendingG = false
			m.move(1)
		case "pgup", "pageup", "ctrl+up", "alt+up", "option+up", "ctrl+u":
			m.pendingG = false
			m.move(-m.height)
		case "pgdown", "pagedown", "ctrl+down", "alt+down", "option+down", "ctrl+d":
			m.pendingG = false
			m.move(m.height)
		case "g":
			if m.pendingG {
				m.goStart()
				m.pendingG = false
			} else {
				m.pendingG = true
			}
		case "home", "ctrl+a":
			m.pendingG = false
			m.goStart()
		case "end", "G", "shift+g", "ctrl+e":
			m.pendingG = false
			m.goEnd()
		case "enter", "space", "x":
			m.pendingG = false
			m.selectCurrent()
			if m.result != "" {
				return m, tea.Quit
			}
		default:
			m.pendingG = false
		}
	}
	return m, nil
}

func (m *formatPickerModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("Select output format"))
	b.WriteString("\n")
	b.WriteString(m.hintStyle.Render("space/x/enter selects, dimmed formats need install, / filters, esc clears, gg/ctrl+a top, G/ctrl+e end, q quits"))
	b.WriteString("\n")
	if m.filtering || m.filter != "" {
		b.WriteString(m.hintStyle.Render("Filter: "))
		b.WriteString(m.filter)
		b.WriteString("\n")
	}

	visible := m.visible()
	if len(visible) == 0 {
		b.WriteString(m.unavailable.Render("  No matching output formats."))
		b.WriteString("\n")
	} else {
		end := minInt(len(visible), m.offset+m.height)
		for i := m.offset; i < end; i++ {
			choice := visible[i]
			cursor := "  "
			if i == m.cursor {
				cursor = m.numberStyle.Render("› ")
			}
			label := choice.Format.String()
			if !choice.Available && choice.Reason != "" {
				label += "  " + choice.Reason
			}
			style := m.available
			if !choice.Available {
				style = m.unavailable
			}
			if i == m.cursor && choice.Available {
				style = m.selectedStyle
			}
			b.WriteString(cursor)
			b.WriteString(style.Render(label))
			b.WriteString("\n")
		}
		if len(visible) > m.height {
			b.WriteString(m.hintStyle.Render(fmt.Sprintf("Showing %d-%d of %d", m.offset+1, end, len(visible))))
			b.WriteString("\n")
		}
	}
	if m.err != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.err))
		b.WriteString("\n")
	}
	return tea.NewView(b.String())
}

func (m *formatPickerModel) updateFilter(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		m.filter = ""
		m.filtering = false
		m.cursor = 0
		m.offset = 0
	case "enter":
		m.filtering = false
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.cursor = 0
			m.offset = 0
		}
	default:
		if text := msg.Key().Text; text != "" {
			m.filter += text
			m.cursor = 0
			m.offset = 0
		}
	}
}

func (m *formatPickerModel) visible() []ports.FormatChoice {
	if strings.TrimSpace(m.filter) == "" {
		return m.choices
	}
	needle := strings.ToLower(strings.TrimSpace(m.filter))
	var result []ports.FormatChoice
	for _, choice := range m.choices {
		label := strings.ToLower(choice.Format.String() + " " + choice.Reason)
		if strings.Contains(label, needle) {
			result = append(result, choice)
		}
	}
	return result
}

func (m *formatPickerModel) move(delta int) {
	visible := m.visible()
	if len(visible) == 0 {
		return
	}
	m.cursor = clampInt(m.cursor+delta, 0, len(visible)-1)
	m.ensureCursorVisible()
	m.err = ""
}

func (m *formatPickerModel) goStart() {
	m.cursor = 0
	m.ensureCursorVisible()
	m.err = ""
}

func (m *formatPickerModel) goEnd() {
	m.cursor = maxInt(0, len(m.visible())-1)
	m.ensureCursorVisible()
	m.err = ""
}

func (m *formatPickerModel) ensureCursorVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *formatPickerModel) selectCurrent() {
	visible := m.visible()
	if len(visible) == 0 || m.cursor < 0 || m.cursor >= len(visible) {
		return
	}
	choice := visible[m.cursor]
	m.result = choice.Format
}

func entriesFromFiles(files []domain.FileRef, includeParent bool) []filePickerEntry {
	entries := make([]filePickerEntry, 0, len(files)+1)
	if includeParent {
		entries = append(entries, filePickerEntry{file: domain.FileRef{Name: "..", Format: domain.FormatDir}, parent: true})
	}
	for _, file := range files {
		entries = append(entries, filePickerEntry{file: file})
	}
	sortPickerEntries(entries)
	return entries
}

func readPickerDir(path string, startDir string) ([]filePickerEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	files := make([]domain.FileRef, 0, len(entries))
	for _, entry := range entries {
		path := filepath.Join(path, entry.Name())
		format, ok := pickerDiscoveredFormat(path, entry.Name(), entry.IsDir())
		if !ok {
			continue
		}
		files = append(files, domain.FileRef{
			Path:   path,
			Name:   entry.Name(),
			Format: format,
		})
	}

	result := entriesFromFiles(files, false)
	if canShowParent(path, startDir) {
		result = append([]filePickerEntry{{
			file: domain.FileRef{
				Path:   filepath.Dir(filepath.Clean(path)),
				Name:   "..",
				Format: domain.FormatDir,
			},
			parent: true,
		}}, result...)
	}
	return result, nil
}

func pickerDiscoveredFormat(path string, name string, isDir bool) (domain.Format, bool) {
	if isDir {
		return domain.FormatDir, true
	}
	format, err := domain.FormatFromPath(name)
	if err != nil {
		if pickerTextFile(path) {
			return domain.FormatTXT, true
		}
		return "", false
	}
	if !domain.IsRegisteredFormat(format) && pickerTextFile(path) {
		return domain.FormatTXT, true
	}
	return format, true
}

func pickerTextFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	buffer := make([]byte, 8192)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false
	}
	return pickerLooksLikeText(buffer[:n])
}

func pickerLooksLikeText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	control := 0
	for _, b := range data {
		if b == 0 {
			return false
		}
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' && b != '\f' && b != '\b' {
			control++
		}
		if b == 0x7f {
			control++
		}
	}
	return control*100 <= len(data)*30
}

func fileFormatColumn(format domain.Format) string {
	label := format.String()
	if format == domain.FormatDir {
		label = "dir"
	}
	return fmt.Sprintf("%-5s", label)
}

func formatLabel(format domain.Format) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(formatColor(format))).Render(fileFormatColumn(format))
}

func formatColor(format domain.Format) string {
	if format == domain.FormatDir {
		return "111"
	}
	if format.IsImage() {
		return "205"
	}
	if format.IsArchive() {
		return "214"
	}
	if format.IsAudio() || format.IsVideo() {
		return "170"
	}
	if format.IsFont() {
		return "45"
	}
	if format.IsDiskImage() {
		return "196"
	}
	switch format {
	case domain.FormatTXT, domain.FormatMD, domain.FormatHTML, domain.FormatRTF, domain.FormatTEX, domain.FormatDOCX, domain.FormatODT, domain.FormatPDF, domain.FormatEPUB, domain.FormatFB2:
		return "42"
	case domain.FormatJSON, domain.FormatYAML, domain.FormatTOML, domain.FormatCSV, domain.FormatINI, domain.FormatXML, domain.FormatPLIST, domain.FormatSQL, domain.FormatSQLite:
		return "39"
	case domain.FormatGeoJSON, domain.FormatTopoJSON, domain.FormatKML, domain.FormatKMZ, domain.FormatGPX, domain.FormatSHP, domain.FormatGPKG, domain.FormatGML, domain.FormatOSM, domain.FormatPBF, domain.FormatMBTiles, domain.FormatPMTiles, domain.FormatMVT, domain.FormatWKT, domain.FormatWKB, domain.FormatLAS, domain.FormatLAZ, domain.FormatHGT:
		return "70"
	case domain.FormatOpenAPI, domain.FormatSwagger, domain.FormatJSONSchema, domain.FormatAsyncAPI, domain.FormatGraphQL, domain.FormatProto, domain.FormatProtoSet, domain.FormatThrift, domain.FormatAvroSchema, domain.FormatFlatBuffers, domain.FormatCapnp, domain.FormatWSDL, domain.FormatXSD:
		return "99"
	}
	colors := []string{"33", "38", "81", "105", "141", "177", "203", "209", "215", "221"}
	return colors[formatHash(format)%len(colors)]
}

func formatHash(format domain.Format) int {
	hash := 0
	for _, r := range format.String() {
		hash = hash*31 + int(r)
	}
	if hash < 0 {
		return -hash
	}
	return hash
}

func sortPickerEntries(entries []filePickerEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].parent != entries[j].parent {
			return entries[i].parent
		}
		leftDir := entries[i].file.Format == domain.FormatDir
		rightDir := entries[j].file.Format == domain.FormatDir
		if leftDir != rightDir {
			return leftDir
		}
		return strings.ToLower(entries[i].file.Name) < strings.ToLower(entries[j].file.Name)
	})
}

func canShowParent(path string, startDir string) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	startAbs, err := filepath.Abs(startDir)
	if err != nil {
		return false
	}
	return filepath.Clean(pathAbs) != filepath.Clean(startAbs)
}

func (p *Prompt) runField(ctx context.Context, field huh.Field) error {
	keymap := huh.NewDefaultKeyMap()
	keymap.Quit = key.NewBinding(key.WithKeys("ctrl+c", "q"), key.WithHelp("q", "quit"))

	form := huh.NewForm(huh.NewGroup(field)).
		WithInput(p.source).
		WithOutput(p.out).
		WithKeyMap(keymap).
		WithAccessible(os.Getenv("CONVERT_ACCESSIBLE") != "").
		WithShowHelp(true).
		WithHeight(p.formHeight()).
		WithWidth(90)

	if err := form.RunWithContext(ctx); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return ports.ErrUserAborted
		}
		return err
	}
	return nil
}

func (p *Prompt) selectFilesFallback(ctx context.Context, files []domain.FileRef) ([]domain.FileRef, error) {
	fmt.Fprintln(p.out, p.titleStyle.Render("Select input files"))
	for i, file := range files {
		name := file.Name
		if file.Format == domain.FormatDir {
			name += "/"
		}
		fmt.Fprintf(
			p.out,
			"  %s %s %s\n",
			p.numberStyle.Render(fmt.Sprintf("%d.", i+1)),
			formatLabel(file.Format),
			name,
		)
	}
	fmt.Fprintln(p.out, p.hintStyle.Render("Use numbers, ranges, or all. Example: 1,3-5"))
	fmt.Fprint(p.out, p.promptStyle.Render("Selection: "))

	line, err := p.readLine(ctx)
	if err != nil {
		return nil, err
	}

	indexes, err := parseSelection(line, len(files), true)
	if err != nil {
		return nil, err
	}

	selected := make([]domain.FileRef, 0, len(indexes))
	for _, index := range indexes {
		selected = append(selected, files[index])
	}
	return selected, nil
}

func (p *Prompt) selectFormatFallback(ctx context.Context, choices []ports.FormatChoice) (domain.Format, error) {
	fmt.Fprintln(p.out, p.titleStyle.Render("Select output format"))
	for i, choice := range choices {
		label := choice.Format.String()
		if !choice.Available {
			label += " " + p.hintStyle.Render("unavailable: "+choice.Reason)
		}
		fmt.Fprintf(p.out, "  %s %s\n", p.numberStyle.Render(fmt.Sprintf("%d.", i+1)), label)
	}
	fmt.Fprint(p.out, p.promptStyle.Render("Format: "))

	index, err := p.readSingleIndex(ctx, len(choices))
	if err != nil {
		return "", err
	}
	return choices[index].Format, nil
}

func (p *Prompt) hasTerminal() bool {
	file, ok := p.source.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func (p *Prompt) listHeight(reserve int) int {
	return maxInt(4, p.terminalHeight(18)-reserve)
}

func (p *Prompt) formHeight() int {
	return maxInt(8, p.terminalHeight(18)-1)
}

func (p *Prompt) terminalHeight(fallback int) int {
	file, ok := p.source.(*os.File)
	if !ok {
		return fallback
	}
	_, height, err := term.GetSize(int(file.Fd()))
	if err != nil || height <= 0 {
		return fallback
	}
	return height
}

func (p *Prompt) terminalWidth(fallback int) int {
	file, ok := p.source.(*os.File)
	if !ok {
		return fallback
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width <= 0 {
		return fallback
	}
	return width
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func clampInt(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func archiveActionFromIndex(index int) domain.ArchiveAction {
	switch index {
	case 0:
		return domain.ArchiveActionExtract
	case 1:
		return domain.ArchiveActionConvert
	default:
		return domain.ArchiveActionCancel
	}
}

func sameFormatActionFromIndex(index int) domain.TransformAction {
	switch index {
	case 0:
		return domain.ActionCompress
	case 1:
		return domain.ActionResize
	default:
		return domain.ActionConvert
	}
}

func (p *Prompt) readSingleIndex(ctx context.Context, max int) (int, error) {
	line, err := p.readLine(ctx)
	if err != nil {
		return 0, err
	}

	indexes, err := parseSelection(line, max, false)
	if err != nil {
		return 0, err
	}
	if len(indexes) != 1 {
		return 0, fmt.Errorf("select exactly one option")
	}
	return indexes[0], nil
}

func (p *Prompt) readLine(ctx context.Context) (string, error) {
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := p.in.ReadString('\n')
		if err == io.EOF && line != "" {
			err = nil
		}
		ch <- result{line: strings.TrimSpace(line), err: err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-ch:
		return result.line, result.err
	}
}

func parseSelection(input string, max int, allowAll bool) ([]int, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return nil, fmt.Errorf("empty selection")
	}
	if input == "q" {
		return nil, ports.ErrUserAborted
	}

	if allowAll && (input == "all" || input == "*") {
		indexes := make([]int, max)
		for i := 0; i < max; i++ {
			indexes[i] = i
		}
		return indexes, nil
	}

	seen := map[int]bool{}
	var indexes []int
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			start, err := parseOneBasedIndex(rangeParts[0], max)
			if err != nil {
				return nil, err
			}
			end, err := parseOneBasedIndex(rangeParts[1], max)
			if err != nil {
				return nil, err
			}
			if start > end {
				return nil, fmt.Errorf("invalid range: %s", part)
			}
			for i := start; i <= end; i++ {
				if !seen[i] {
					indexes = append(indexes, i)
					seen[i] = true
				}
			}
			continue
		}

		index, err := parseOneBasedIndex(part, max)
		if err != nil {
			return nil, err
		}
		if !seen[index] {
			indexes = append(indexes, index)
			seen[index] = true
		}
	}

	if len(indexes) == 0 {
		return nil, fmt.Errorf("empty selection")
	}
	return indexes, nil
}

func parseOneBasedIndex(input string, max int) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil {
		return 0, fmt.Errorf("invalid selection: %s", input)
	}
	if value < 1 || value > max {
		return 0, fmt.Errorf("selection out of range: %d", value)
	}
	return value - 1, nil
}
