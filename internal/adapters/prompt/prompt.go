package promptadapter

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
	"github.com/shellcell/convert/internal/scan"
	"github.com/shellcell/convert/internal/shell"
	"github.com/shellcell/convert/internal/theme"
	"golang.org/x/term"
)

type Prompt struct {
	source   io.Reader
	in       *bufio.Reader
	out      io.Writer
	showHelp bool

	titleStyle                  lipgloss.Style
	keyStyle                    lipgloss.Style
	numberStyle                 lipgloss.Style
	hintStyle                   lipgloss.Style
	flagStyle                   lipgloss.Style
	badgeStyle                  lipgloss.Style
	promptStyle                 lipgloss.Style
	selectedStyle               lipgloss.Style
	dimStyle                    lipgloss.Style
	errorStyle                  lipgloss.Style
	unavailableStyle            lipgloss.Style
	categoryStyles              map[string]lipgloss.Style
	categoryFaintStyles         map[string]lipgloss.Style
	categorySelectedStyles      map[string]lipgloss.Style
	categorySelectedFaintStyles map[string]lipgloss.Style
}

func New(in io.Reader, out io.Writer, palettes ...theme.Palette) *Prompt {
	palette := theme.Default()
	if len(palettes) > 0 {
		palette = palettes[0]
	}
	categoryStyles, categoryFaintStyles, categorySelectedStyles, categorySelectedFaintStyles := categoryStyleMaps(palette)
	return &Prompt{
		source:                      in,
		in:                          bufio.NewReader(in),
		out:                         out,
		showHelp:                    true,
		titleStyle:                  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette.Title)),
		keyStyle:                    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette.Number)),
		numberStyle:                 lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Number)),
		hintStyle:                   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Hint)),
		flagStyle:                   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Flag)),
		badgeStyle:                  lipgloss.NewStyle().Foreground(lipgloss.Color(palette.BadgeForeground)).Background(lipgloss.Color(palette.BadgeBackground)).Padding(0, 1),
		promptStyle:                 lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette.Prompt)),
		selectedStyle:               lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Selected)).Bold(true),
		dimStyle:                    lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Dim)),
		errorStyle:                  lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Error)),
		unavailableStyle:            lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Unavailable)),
		categoryStyles:              categoryStyles,
		categoryFaintStyles:         categoryFaintStyles,
		categorySelectedStyles:      categorySelectedStyles,
		categorySelectedFaintStyles: categorySelectedFaintStyles,
	}
}

// SetShowHelp toggles the key-hint helper lines in interactive prompts.
func (p *Prompt) SetShowHelp(show bool) {
	p.showHelp = show
}

// helpLine renders alternating key/description pairs ("enter", "selects",
// ...) with keys highlighted. Returns "" when helper lines are disabled.
func (p *Prompt) helpLine(pairs ...string) string {
	if !p.showHelp {
		return ""
	}
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString(p.hintStyle.Render(", "))
		}
		b.WriteString(p.keyStyle.Render(pairs[i]))
		b.WriteString(p.hintStyle.Render(" " + pairs[i+1]))
	}
	return b.String()
}

func (p *Prompt) SelectBackend(ctx context.Context, input domain.Format, output domain.Format, choices []ports.BackendChoice) (string, error) {
	if len(choices) == 0 {
		return "", fmt.Errorf("no backends to select")
	}
	// Non-terminal runs stay scriptable: take the best-ranked backend.
	if !p.hasTerminal() {
		return choices[0].ID, nil
	}

	options := make([]selectPromptOption[string], 0, len(choices))
	for _, choice := range choices {
		options = append(options, selectPromptOption[string]{
			Label: choice.ID,
			Value: choice.ID,
			Hint:  choice.Description,
		})
	}
	return selectValueTerminal(ctx, p, "Select backend", "Several installed backends can convert "+input.String()+" -> "+output.String()+".", options)
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
		return selectValueTerminal(ctx, p, "Save outputs", "Current directory: "+currentDir, []selectPromptOption[ports.OutputLocation]{
			{Label: "Current directory", Value: ports.OutputLocationCurrent},
			{Label: "Beside each source", Value: ports.OutputLocationSource},
		})
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
		return selectValueTerminal(ctx, p, "Archive detected", file.Name+" ["+file.Format.String()+"]", []selectPromptOption[domain.ArchiveAction]{
			{Label: "Extract archive", Value: domain.ArchiveActionExtract},
			{Label: "Choose output format", Value: domain.ArchiveActionConvert},
			{Label: "Cancel", Value: domain.ArchiveActionCancel},
		})
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
	options := []selectPromptOption[domain.TransformAction]{
		{Label: "Compress", Value: domain.ActionCompress},
	}
	// Resizing only means something for visual formats.
	if format.IsImage() || format.IsVideo() {
		options = append(options, selectPromptOption[domain.TransformAction]{Label: "Resize", Value: domain.ActionResize})
	}
	options = append(options, selectPromptOption[domain.TransformAction]{Label: "Convert/copy", Value: domain.ActionConvert})

	if p.hasTerminal() {
		return selectValueTerminal(ctx, p, "Input and output format match", "Choose what to do with "+format.String()+" files.", options)
	}

	fmt.Fprintln(p.out, p.titleStyle.Render("Input and output format match"))
	for i, option := range options {
		badge := ""
		if i == 0 {
			badge = " " + p.badgeStyle.Render(format.String())
		}
		fmt.Fprintf(p.out, "  %s %s%s\n", p.numberStyle.Render(fmt.Sprintf("%d.", i+1)), option.Label, badge)
	}
	fmt.Fprint(p.out, p.promptStyle.Render("Action: "))

	index, err := p.readSingleIndex(ctx, len(options))
	if err != nil {
		return "", err
	}

	return options[index].Value, nil
}

func (p *Prompt) ConfirmOption(ctx context.Context, title string, description string, defaultValue bool) (bool, error) {
	if p.hasTerminal() {
		options := []selectPromptOption[bool]{{Label: "No", Value: false}, {Label: "Yes", Value: true}}
		if defaultValue {
			options = []selectPromptOption[bool]{{Label: "Yes", Value: true}, {Label: "No", Value: false}}
		}
		return selectValueTerminal(ctx, p, title, description, options)
	}

	fmt.Fprintln(p.out, p.titleStyle.Render(title))
	if description != "" {
		fmt.Fprintln(p.out, p.hintStyle.Render(description))
	}
	if defaultValue {
		fmt.Fprintf(p.out, "  %s Yes\n", p.numberStyle.Render("1."))
		fmt.Fprintf(p.out, "  %s No\n", p.numberStyle.Render("2."))
	} else {
		fmt.Fprintf(p.out, "  %s No\n", p.numberStyle.Render("1."))
		fmt.Fprintf(p.out, "  %s Yes\n", p.numberStyle.Render("2."))
	}
	fmt.Fprint(p.out, p.promptStyle.Render("Choice: "))
	index, err := p.readSingleIndex(ctx, 2)
	if err != nil {
		return false, err
	}
	if defaultValue {
		return index == 0, nil
	}
	return index == 1, nil
}

func (p *Prompt) ConfirmCommand(ctx context.Context, review ports.CommandReview) (ports.CommandReviewAction, string, error) {
	if !p.hasTerminal() {
		return ports.CommandReviewProceed, review.EditCommand, nil
	}

	options := []selectPromptOption[ports.CommandReviewAction]{{Label: "Proceed", Value: ports.CommandReviewProceed}}
	if review.Editable {
		options = append(options, selectPromptOption[ports.CommandReviewAction]{Label: "Edit command", Value: ports.CommandReviewEdit})
	}
	options = append(options, selectPromptOption[ports.CommandReviewAction]{Label: "Cancel", Value: ports.CommandReviewCancel})

	title := "Confirm conversion"
	commandLabel := "Command:"
	if review.JobCount > 1 {
		title = fmt.Sprintf("Confirm conversion of %d files", review.JobCount)
		commandLabel = "First command (the remaining files run the same way):"
	}
	command := p.commandReviewDescription(review)
	selected, err := selectValueTerminal(ctx, p, title, "Backend: "+review.Backend+"\n"+commandLabel+"\n"+command, options)
	if err != nil {
		return "", "", err
	}
	if selected != ports.CommandReviewEdit {
		return selected, review.EditCommand, nil
	}

	edited := strings.TrimSpace(review.EditCommand)
	edited, err = p.inputTerminal(ctx, inputPromptConfig{
		Title:       "Edit command",
		Description: "The edited command runs through the shell for this job.",
		Value:       edited,
		Validate: func(value string) error {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("command is required")
			}
			return nil
		},
	})
	if err != nil {
		return "", "", err
	}
	return ports.CommandReviewEdit, strings.TrimSpace(edited), nil
}

func (p *Prompt) AskOutputPath(ctx context.Context, currentPath string) (string, error) {
	if !p.hasTerminal() {
		return currentPath, nil
	}

	value := currentPath
	value, err := p.inputTerminal(ctx, inputPromptConfig{
		Title:       "Output already exists",
		Description: "Enter a different output path.",
		Value:       value,
		Validate: func(value string) error {
			value = strings.TrimSpace(value)
			if value == "" {
				return fmt.Errorf("output path is required")
			}
			if value == currentPath {
				return fmt.Errorf("enter a different output path")
			}
			return nil
		},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func (p *Prompt) commandReviewDescription(review ports.CommandReview) string {
	if len(review.Commands) == 0 {
		return review.Message
	}

	lines := make([]string, 0, len(review.Commands))
	for _, command := range review.Commands {
		lines = append(lines, p.formatCommand(command))
	}
	return strings.Join(lines, "\n")
}

func (p *Prompt) formatCommand(command ports.Command) string {
	var b strings.Builder
	if command.Dir != "" {
		b.WriteString("cd ")
		b.WriteString(promptShellQuote(command.Dir))
		b.WriteString(" &&\n")
	}
	b.WriteString(promptShellQuote(command.Name))
	for i := 0; i < len(command.Args); {
		arg := command.Args[i]
		b.WriteString("\n  ")
		if strings.HasPrefix(arg, "-") {
			b.WriteString(p.flagStyle.Render(promptShellQuote(arg)))
			i++
			for i < len(command.Args) && !strings.HasPrefix(command.Args[i], "-") {
				b.WriteString(" ")
				b.WriteString(promptShellQuote(command.Args[i]))
				i++
			}
			continue
		}
		b.WriteString(promptShellQuote(arg))
		i++
	}
	return b.String()
}

func promptShellQuote(value string) string {
	return shell.Quote(value)
}

func (p *Prompt) AskText(ctx context.Context, title string, description string, defaultValue string) (string, error) {
	if p.hasTerminal() {
		value, err := p.inputTerminal(ctx, inputPromptConfig{
			Title:       title,
			Description: description,
			Placeholder: defaultValue,
			Value:       defaultValue,
		})
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) == "" {
			return defaultValue, nil
		}
		return strings.TrimSpace(value), nil
	}

	if description != "" {
		fmt.Fprintln(p.out, p.hintStyle.Render(description))
	}
	label := title
	if defaultValue != "" {
		label += " [" + defaultValue + "]"
	}
	fmt.Fprint(p.out, p.promptStyle.Render(label+": "))
	value, err := p.readLine(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return defaultValue, nil
	}
	return strings.TrimSpace(value), nil
}

func (p *Prompt) AskOutputSize(ctx context.Context, defaultSize string) (string, error) {
	if p.hasTerminal() {
		value, err := p.inputTerminal(ctx, inputPromptConfig{
			Title:       "Output size",
			Description: "Examples: 1024x1024, 1280x720, 800x, x600. Press enter to use the default.",
			Placeholder: defaultSize,
			Value:       defaultSize,
		})
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) == "" {
			return defaultSize, nil
		}
		return strings.TrimSpace(value), nil
	}

	fmt.Fprintf(p.out, "%s %s\n", p.hintStyle.Render("Output size examples: 1024x1024, 1280x720, 800x, x600."), p.hintStyle.Render("Default: "+defaultSize))
	fmt.Fprint(p.out, p.promptStyle.Render("Output size: "))
	value, err := p.readLine(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return defaultSize, nil
	}
	return strings.TrimSpace(value), nil
}

func (p *Prompt) AskResize(ctx context.Context) (string, error) {
	if p.hasTerminal() {
		value, err := p.inputTerminal(ctx, inputPromptConfig{
			Title:       "Resize",
			Description: "Examples: 800x, x600, 1280x720, 50%",
			Placeholder: "800x",
			Validate: func(value string) error {
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("resize value is required")
				}
				return nil
			},
		})
		if err != nil {
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
		value, err := p.inputTerminal(ctx, inputPromptConfig{
			Title:       "Quality",
			Description: "Enter quality from 1 to 100.",
			Placeholder: strconv.Itoa(defaultQuality),
			Value:       strconv.Itoa(defaultQuality),
			Validate: func(value string) error {
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
			},
		})
		if err != nil {
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

type selectPromptOption[T comparable] struct {
	Label string
	Value T
	Hint  string
}

func selectValueTerminal[T comparable](ctx context.Context, p *Prompt, title string, description string, options []selectPromptOption[T]) (T, error) {
	var zero T
	if len(options) == 0 {
		return zero, fmt.Errorf("no options to select")
	}
	model := &selectPromptModel[T]{
		title:         title,
		description:   description,
		options:       options,
		height:        p.listHeight(6),
		help:          p.helpLine("↑/↓", "moves", "enter", "selects", "q", "quits"),
		titleStyle:    p.titleStyle,
		numberStyle:   p.numberStyle,
		hintStyle:     p.hintStyle,
		selectedStyle: p.selectedStyle,
		errorStyle:    p.errorStyle,
	}
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(p.source), tea.WithOutput(p.out))
	result, err := program.Run()
	if err != nil {
		return zero, err
	}
	finished, ok := result.(*selectPromptModel[T])
	if !ok {
		return zero, fmt.Errorf("unexpected prompt result")
	}
	if finished.aborted {
		return zero, ports.ErrUserAborted
	}
	if !finished.submitted {
		return zero, fmt.Errorf("select an option")
	}
	return finished.result, nil
}

type selectPromptModel[T comparable] struct {
	title         string
	description   string
	options       []selectPromptOption[T]
	cursor        int
	offset        int
	height        int
	help          string
	result        T
	submitted     bool
	aborted       bool
	titleStyle    lipgloss.Style
	numberStyle   lipgloss.Style
	hintStyle     lipgloss.Style
	selectedStyle lipgloss.Style
	errorStyle    lipgloss.Style
}

func (m *selectPromptModel[T]) Init() tea.Cmd { return nil }

func (m *selectPromptModel[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = maxInt(3, msg.Height-6)
		m.ensureCursorVisible()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.aborted = true
			return m, tea.Quit
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup", "pageup", "ctrl+up", "alt+up", "option+up", "ctrl+u":
			m.move(-m.height)
		case "pgdown", "pagedown", "ctrl+down", "alt+down", "option+down", "ctrl+d":
			m.move(m.height)
		case "home", "ctrl+a":
			m.cursor = 0
			m.ensureCursorVisible()
		case "end", "G", "shift+g", "ctrl+e":
			m.cursor = len(m.options) - 1
			m.ensureCursorVisible()
		case "enter", "space", "x":
			m.result = m.options[m.cursor].Value
			m.submitted = true
			return m, tea.Quit
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-1)
		case tea.MouseWheelDown:
			m.move(1)
		}
	}
	return m, nil
}

func (m *selectPromptModel[T]) View() tea.View {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render(m.title))
	b.WriteString("\n")
	if m.description != "" {
		b.WriteString(m.description)
		b.WriteString("\n")
	}
	if m.help != "" {
		b.WriteString(m.help)
		b.WriteString("\n")
	}
	end := minInt(len(m.options), m.offset+m.height)
	for i := m.offset; i < end; i++ {
		option := m.options[i]
		cursor := "  "
		label := option.Label
		if i == m.cursor {
			cursor = m.numberStyle.Render("› ")
			label = m.selectedStyle.Render(label)
		}
		b.WriteString(cursor)
		b.WriteString(label)
		if option.Hint != "" {
			b.WriteString(" ")
			b.WriteString(m.hintStyle.Render(option.Hint))
		}
		b.WriteString("\n")
	}
	if len(m.options) > m.height {
		b.WriteString(m.hintStyle.Render(fmt.Sprintf("Showing %d-%d of %d", m.offset+1, end, len(m.options))))
		b.WriteString("\n")
	}
	return mouseView(b.String())
}

// mouseView wraps rendered prompt output in a view with mouse reporting on,
// so wheel scrolling and clicking work in every picker.
func mouseView(content string) tea.View {
	view := tea.NewView(content)
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m *selectPromptModel[T]) move(delta int) {
	m.cursor = clampInt(m.cursor+delta, 0, len(m.options)-1)
	m.ensureCursorVisible()
}

func (m *selectPromptModel[T]) ensureCursorVisible() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
	maxOffset := maxInt(0, len(m.options)-m.height)
	m.offset = clampInt(m.offset, 0, maxOffset)
}

type inputPromptConfig struct {
	Title       string
	Description string
	Placeholder string
	Value       string
	Validate    func(string) error
}

func (p *Prompt) inputTerminal(ctx context.Context, config inputPromptConfig) (string, error) {
	model := &inputPromptModel{
		title:       config.Title,
		description: config.Description,
		placeholder: config.Placeholder,
		help:        p.helpLine("enter", "accepts", "ctrl+c", "quits"),
		value:       []rune(config.Value),
		cursor:      len([]rune(config.Value)),
		validate:    config.Validate,
		titleStyle:  p.titleStyle,
		hintStyle:   p.hintStyle,
		promptStyle: p.promptStyle,
		errorStyle:  p.errorStyle,
	}
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(p.source), tea.WithOutput(p.out))
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	finished, ok := result.(*inputPromptModel)
	if !ok {
		return "", fmt.Errorf("unexpected input result")
	}
	if finished.aborted {
		return "", ports.ErrUserAborted
	}
	if !finished.submitted {
		return "", fmt.Errorf("enter a value")
	}
	return string(finished.value), nil
}

type inputPromptModel struct {
	title       string
	description string
	placeholder string
	help        string
	value       []rune
	cursor      int
	err         string
	validate    func(string) error
	submitted   bool
	aborted     bool
	titleStyle  lipgloss.Style
	hintStyle   lipgloss.Style
	promptStyle lipgloss.Style
	errorStyle  lipgloss.Style
}

func (m *inputPromptModel) Init() tea.Cmd { return nil }

func (m *inputPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		case "enter":
			value := string(m.value)
			if m.validate != nil {
				if err := m.validate(value); err != nil {
					m.err = err.Error()
					return m, nil
				}
			}
			m.submitted = true
			return m, tea.Quit
		case "backspace", "ctrl+h":
			if m.cursor > 0 {
				m.value = append(m.value[:m.cursor-1], m.value[m.cursor:]...)
				m.cursor--
				m.err = ""
			}
		case "delete", "ctrl+d":
			if m.cursor < len(m.value) {
				m.value = append(m.value[:m.cursor], m.value[m.cursor+1:]...)
				m.err = ""
			}
		case "left", "ctrl+b":
			m.cursor = maxInt(0, m.cursor-1)
		case "right", "ctrl+f":
			m.cursor = minInt(len(m.value), m.cursor+1)
		case "home", "ctrl+a":
			m.cursor = 0
		case "end", "ctrl+e":
			m.cursor = len(m.value)
		default:
			if text := msg.Key().Text; text != "" {
				runes := []rune(text)
				m.value = append(m.value[:m.cursor], append(runes, m.value[m.cursor:]...)...)
				m.cursor += len(runes)
				m.err = ""
			}
		}
	}
	return m, nil
}

func (m *inputPromptModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render(m.title))
	b.WriteString("\n")
	if m.description != "" {
		b.WriteString(m.hintStyle.Render(m.description))
		b.WriteString("\n")
	}
	b.WriteString(m.promptStyle.Render("> "))
	b.WriteString(m.renderValue())
	b.WriteString("\n")
	if m.err != "" {
		b.WriteString(m.errorStyle.Render(m.err))
		b.WriteString("\n")
	}
	if m.help != "" {
		b.WriteString(m.help)
		b.WriteString("\n")
	}
	return tea.NewView(b.String())
}

func (m *inputPromptModel) renderValue() string {
	if len(m.value) == 0 {
		return m.hintStyle.Render(m.placeholder) + m.promptStyle.Render(" ")
	}
	var b strings.Builder
	for i, r := range m.value {
		if i == m.cursor {
			b.WriteString(m.promptStyle.Render("▏"))
		}
		b.WriteRune(r)
	}
	if m.cursor == len(m.value) {
		b.WriteString(m.promptStyle.Render("▏"))
	}
	return b.String()
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
	file        domain.FileRef
	parent      bool
	label       string
	searchText  string
	formatLabel string
	size        int64
}

type pickerSortMode int

const (
	sortByName pickerSortMode = iota
	sortByNameDesc
	sortBySize
	sortBySizeDesc
	sortByFormat
	sortModeCount
)

func (m pickerSortMode) String() string {
	switch m {
	case sortByNameDesc:
		return "name desc"
	case sortBySize:
		return "size"
	case sortBySizeDesc:
		return "size desc"
	case sortByFormat:
		return "format"
	default:
		return "name"
	}
}

type filePickerPosition struct {
	cursor int
	offset int
	filter string
}

type filePickerModel struct {
	in             io.Reader
	out            io.Writer
	startDir       string
	currentDir     string
	entries        []filePickerEntry
	selected       map[string]domain.FileRef
	positions      map[string]filePickerPosition
	cursor         int
	offset         int
	height         int
	width          int
	sortMode       pickerSortMode
	help           string
	filterHelp     string
	filter         string
	filtering      bool
	pendingG       bool
	err            string
	aborted        bool
	result         []domain.FileRef
	titleStyle     lipgloss.Style
	numberStyle    lipgloss.Style
	hintStyle      lipgloss.Style
	badgeStyle     lipgloss.Style
	promptStyle    lipgloss.Style
	selectedStyle  lipgloss.Style
	dimStyle       lipgloss.Style
	errorStyle     lipgloss.Style
	categoryStyles map[string]lipgloss.Style
}

func newFilePickerModel(p *Prompt, root string, files []domain.FileRef) *filePickerModel {
	model := &filePickerModel{
		in:             p.source,
		out:            p.out,
		startDir:       filepath.Clean(root),
		currentDir:     filepath.Clean(root),
		selected:       map[string]domain.FileRef{},
		positions:      map[string]filePickerPosition{},
		height:         p.listHeight(8),
		width:          p.terminalWidth(90),
		titleStyle:     p.titleStyle,
		numberStyle:    p.numberStyle,
		hintStyle:      p.hintStyle,
		badgeStyle:     p.badgeStyle,
		promptStyle:    p.promptStyle,
		selectedStyle:  p.selectedStyle,
		dimStyle:       p.dimStyle,
		errorStyle:     p.errorStyle,
		categoryStyles: p.categoryStyles,
		help: p.helpLine(
			"enter/→", "opens dirs", "←", "goes up", "space/x", "selects",
			"a", "selects filtered", "c", "clears", "/", "filters", "s", "sorts", "q", "quits",
		),
		filterHelp: p.helpLine("enter", "keeps filter", "esc", "clears"),
	}
	model.entries = entriesFromFiles(files, false, p.categoryStyles)
	model.applySort()
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
		case "s", "S":
			m.pendingG = false
			m.cycleSort()
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
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-1)
		case tea.MouseWheelDown:
			m.move(1)
		}
	}
	return m, nil
}

func (m *filePickerModel) cycleSort() {
	m.sortMode = (m.sortMode + 1) % sortModeCount
	m.applySort()
	m.cursor = 0
	m.offset = 0
	m.err = ""
}

func (m *filePickerModel) applySort() {
	sortPickerEntries(m.entries, m.sortMode)
}

func (m *filePickerModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("Select input files"))
	b.WriteString("\n")
	if m.help != "" {
		b.WriteString(m.help)
		b.WriteString("\n")
	}
	b.WriteString(m.dimStyle.Render(filepath.Clean(m.currentDir)))
	if m.sortMode != sortByName {
		b.WriteString(m.dimStyle.Render("  sort: " + m.sortMode.String()))
	}
	b.WriteString("\n")
	if m.filtering || m.filter != "" {
		b.WriteString(m.promptStyle.Render("Filter: "))
		b.WriteString(m.filter)
		if m.filtering {
			b.WriteString(m.promptStyle.Render("▏"))
			if m.filterHelp != "" {
				b.WriteString("  ")
				b.WriteString(m.filterHelp)
			}
		}
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
			if entry.parent {
				mark = "   "
			}
			b.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, mark, entry.formatLabel, entry.label))
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
		b.WriteString(m.errorStyle.Render(m.err))
		b.WriteString("\n")
	}
	return mouseView(b.String())
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
		if strings.Contains(entry.searchText, needle) {
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
	entries, err := readPickerDir(target, m.startDir, m.categoryStyles)
	if err != nil {
		m.err = err.Error()
		return
	}
	m.savePosition()
	m.currentDir = target
	m.entries = entries
	m.applySort()
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

type formatPickerEntry struct {
	choice      ports.FormatChoice
	category    string
	label       string
	backends    string
	reasonLabel string
	searchText  string
}

type formatPickerModel struct {
	choices                     []formatPickerEntry
	categoryStyles              map[string]lipgloss.Style
	categoryFaintStyles         map[string]lipgloss.Style
	categorySelectedStyles      map[string]lipgloss.Style
	categorySelectedFaintStyles map[string]lipgloss.Style
	cursor                      int
	offset                      int
	height                      int
	sortMode                    int
	help                        string
	note                        string
	filterHelp                  string
	filter                      string
	filtering                   bool
	pendingG                    bool
	err                         string
	aborted                     bool
	result                      domain.Format
	titleStyle                  lipgloss.Style
	numberStyle                 lipgloss.Style
	hintStyle                   lipgloss.Style
	promptStyle                 lipgloss.Style
	unavailable                 lipgloss.Style
	selectedStyle               lipgloss.Style
	errorStyle                  lipgloss.Style
}

func newFormatPickerModel(p *Prompt, choices []ports.FormatChoice) *formatPickerModel {
	// Column widths are computed over all rows so the format, category, and
	// backends columns line up.
	formatWidth, categoryWidth := 0, 0
	for _, choice := range choices {
		formatWidth = maxInt(formatWidth, len(choice.Format.String()))
		categoryWidth = maxInt(categoryWidth, len(formatCategory(choice.Format)))
	}

	entries := make([]formatPickerEntry, 0, len(choices))
	for _, choice := range choices {
		category := formatCategory(choice.Format)
		label := padRight(choice.Format.String(), formatWidth)
		backends := strings.Join(choice.Backends, ", ")
		reasonLabel := label + "  " + padRight(category, categoryWidth)
		if choice.Reason != "" {
			reasonLabel += "  " + choice.Reason
		}
		entries = append(entries, formatPickerEntry{
			choice:      choice,
			category:    padRight(category, categoryWidth),
			label:       label,
			backends:    backends,
			reasonLabel: reasonLabel,
			searchText:  strings.ToLower(choice.Format.String() + " " + category + " " + choice.Reason + " " + backends),
		})
	}
	return &formatPickerModel{
		choices:                     entries,
		categoryStyles:              p.categoryStyles,
		categoryFaintStyles:         p.categoryFaintStyles,
		categorySelectedStyles:      p.categorySelectedStyles,
		categorySelectedFaintStyles: p.categorySelectedFaintStyles,
		height:                      p.listHeight(6),
		help: p.helpLine(
			"space/x/enter", "selects", "/", "filters", "s", "sorts",
			"gg", "top", "G", "end", "q", "quits",
		),
		note:          p.hintStyle.Render("dimmed formats need install"),
		filterHelp:    p.helpLine("enter", "keeps filter", "esc", "clears"),
		titleStyle:    p.titleStyle,
		numberStyle:   p.numberStyle,
		hintStyle:     p.hintStyle,
		promptStyle:   p.promptStyle,
		unavailable:   p.unavailableStyle,
		selectedStyle: p.selectedStyle,
		errorStyle:    p.errorStyle,
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
		case "s", "S":
			m.pendingG = false
			m.cycleSort()
		case "enter", "space", "x":
			m.pendingG = false
			m.selectCurrent()
			if m.result != "" {
				return m, tea.Quit
			}
		default:
			m.pendingG = false
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-1)
		case tea.MouseWheelDown:
			m.move(1)
		}
	}
	return m, nil
}

// cycleSort switches between name, reverse name, and category ordering.
func (m *formatPickerModel) cycleSort() {
	m.sortMode = (m.sortMode + 1) % 3
	mode := m.sortMode
	sort.SliceStable(m.choices, func(i, j int) bool {
		switch mode {
		case 1:
			return m.choices[i].label > m.choices[j].label
		case 2:
			if m.choices[i].category != m.choices[j].category {
				return m.choices[i].category < m.choices[j].category
			}
		}
		return m.choices[i].label < m.choices[j].label
	})
	m.cursor = 0
	m.offset = 0
}

func (m *formatPickerModel) sortModeName() string {
	switch m.sortMode {
	case 1:
		return "name desc"
	case 2:
		return "category"
	default:
		return "name"
	}
}

func (m *formatPickerModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.titleStyle.Render("Select output format"))
	b.WriteString("\n")
	if m.help != "" {
		b.WriteString(m.help)
		b.WriteString("  ")
		b.WriteString(m.note)
		b.WriteString("\n")
	}
	if m.sortMode != 0 {
		b.WriteString(m.hintStyle.Render("sort: " + m.sortModeName()))
		b.WriteString("\n")
	}
	if m.filtering || m.filter != "" {
		b.WriteString(m.promptStyle.Render("Filter: "))
		b.WriteString(m.filter)
		if m.filtering {
			b.WriteString(m.promptStyle.Render("▏"))
			if m.filterHelp != "" {
				b.WriteString("  ")
				b.WriteString(m.filterHelp)
			}
		}
		b.WriteString("\n")
	}

	visible := m.visible()
	if len(visible) == 0 {
		b.WriteString(m.unavailable.Render("  No matching output formats."))
		b.WriteString("\n")
	} else {
		end := minInt(len(visible), m.offset+m.height)
		for i := m.offset; i < end; i++ {
			entry := visible[i]
			cursor := "  "
			if i == m.cursor {
				cursor = m.numberStyle.Render("› ")
			}
			b.WriteString(cursor)
			b.WriteString(m.renderChoice(entry, i == m.cursor))
			b.WriteString("\n")
		}
		if len(visible) > m.height {
			b.WriteString(m.hintStyle.Render(fmt.Sprintf("Showing %d-%d of %d", m.offset+1, end, len(visible))))
			b.WriteString("\n")
		}
	}
	if m.err != "" {
		b.WriteString(m.errorStyle.Render(m.err))
		b.WriteString("\n")
	}
	return mouseView(b.String())
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

func (m *formatPickerModel) visible() []formatPickerEntry {
	if strings.TrimSpace(m.filter) == "" {
		return m.choices
	}
	needle := strings.ToLower(strings.TrimSpace(m.filter))
	var result []formatPickerEntry
	for _, entry := range m.choices {
		if strings.Contains(entry.searchText, needle) {
			result = append(result, entry)
		}
	}
	return result
}

func (m *formatPickerModel) renderChoice(entry formatPickerEntry, selected bool) string {
	if !entry.choice.Available {
		return m.unavailable.Render(entry.reasonLabel)
	}

	category := strings.TrimSpace(entry.category)
	style := formatStyle(m.categoryStyles, category)
	categoryStyle := formatStyle(m.categoryFaintStyles, category)
	if selected {
		style = formatStyle(m.categorySelectedStyles, category)
		categoryStyle = formatStyle(m.categorySelectedFaintStyles, category)
	}
	line := style.Render(entry.label) + "  " + categoryStyle.Render(entry.category)
	if entry.backends != "" {
		line += "  " + m.hintStyle.Render("via "+entry.backends)
	}
	return line
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
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
	entry := visible[m.cursor]
	m.result = entry.choice.Format
}

func entriesFromFiles(files []domain.FileRef, includeParent bool, styles map[string]lipgloss.Style) []filePickerEntry {
	entries := make([]filePickerEntry, 0, len(files)+1)
	if includeParent {
		entries = append(entries, newFilePickerEntry(domain.FileRef{Name: "..", Format: domain.FormatDir}, true, styles))
	}
	for _, file := range files {
		entries = append(entries, newFilePickerEntry(file, false, styles))
	}
	sortPickerEntries(entries, sortByName)
	return entries
}

func newFilePickerEntry(file domain.FileRef, parent bool, styles map[string]lipgloss.Style) filePickerEntry {
	label := file.Name
	if file.Format == domain.FormatDir && !strings.HasSuffix(label, "/") {
		label += "/"
	}
	var size int64
	if !parent && file.Path != "" {
		if info, err := os.Stat(file.Path); err == nil && !info.IsDir() {
			size = info.Size()
		}
	}
	searchText := strings.ToLower(label + " " + file.Format.String() + " " + formatCategory(file.Format))
	return filePickerEntry{
		file:        file,
		parent:      parent,
		label:       label,
		searchText:  searchText,
		formatLabel: formatLabel(styles, file.Format),
		size:        size,
	}
}

func readPickerDir(path string, startDir string, styles map[string]lipgloss.Style) ([]filePickerEntry, error) {
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

	result := entriesFromFiles(files, false, styles)
	if canShowParent(path, startDir) {
		result = append([]filePickerEntry{newFilePickerEntry(domain.FileRef{
			Path:   filepath.Dir(filepath.Clean(path)),
			Name:   "..",
			Format: domain.FormatDir,
		}, true, styles)}, result...)
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
	text, err := scan.IsTextFile(path)
	return err == nil && text
}

func fileFormatColumn(format domain.Format) string {
	label := format.String()
	if format == domain.FormatDir {
		label = "dir"
	}
	return fmt.Sprintf("%-5s", label)
}

func formatLabel(styles map[string]lipgloss.Style, format domain.Format) string {
	return formatStyle(styles, formatCategory(format)).Render(fileFormatColumn(format))
}

func formatChoiceLabel(format domain.Format) string {
	return format.String() + " (" + formatCategory(format) + ")"
}

func formatCategory(format domain.Format) string {
	return domain.CategoryOf(format)
}

func categoryStyleMaps(palette theme.Palette) (map[string]lipgloss.Style, map[string]lipgloss.Style, map[string]lipgloss.Style, map[string]lipgloss.Style) {
	categories := []string{"directory", "image", "animation", "video", "audio", "archive", "font", "disk", "vm", "doc", "ebook", "spreadsheet", "data", "geo", "schema", "diagram", "code", "custom"}
	base := make(map[string]lipgloss.Style, len(categories))
	faint := make(map[string]lipgloss.Style, len(categories))
	selected := make(map[string]lipgloss.Style, len(categories))
	selectedFaint := make(map[string]lipgloss.Style, len(categories))
	for _, category := range categories {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(palette.CategoryColor(category)))
		base[category] = style
		faint[category] = style.Faint(true)
		selected[category] = style.Bold(true)
		selectedFaint[category] = style.Faint(true).Bold(true)
	}
	return base, faint, selected, selectedFaint
}

func formatStyle(styles map[string]lipgloss.Style, category string) lipgloss.Style {
	if style, ok := styles[category]; ok {
		return style
	}
	return styles["custom"]
}

// sortPickerEntries keeps the parent entry and directories on top, then
// orders files by the selected mode.
func sortPickerEntries(entries []filePickerEntry, mode pickerSortMode) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].parent != entries[j].parent {
			return entries[i].parent
		}
		leftDir := entries[i].file.Format == domain.FormatDir
		rightDir := entries[j].file.Format == domain.FormatDir
		if leftDir != rightDir {
			return leftDir
		}

		leftName := strings.ToLower(entries[i].file.Name)
		rightName := strings.ToLower(entries[j].file.Name)
		switch mode {
		case sortByNameDesc:
			return leftName > rightName
		case sortBySize:
			if entries[i].size != entries[j].size {
				return entries[i].size < entries[j].size
			}
		case sortBySizeDesc:
			if entries[i].size != entries[j].size {
				return entries[i].size > entries[j].size
			}
		case sortByFormat:
			leftFormat := entries[i].file.Format.String()
			rightFormat := entries[j].file.Format.String()
			if leftFormat != rightFormat {
				return leftFormat < rightFormat
			}
		}
		return leftName < rightName
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
			formatLabel(p.categoryStyles, file.Format),
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
		label := formatStyle(p.categoryStyles, formatCategory(choice.Format)).Render(formatChoiceLabel(choice.Format))
		if len(choice.Backends) > 0 {
			label += " " + p.hintStyle.Render("via "+strings.Join(choice.Backends, ", "))
		}
		if !choice.Available {
			label = p.hintStyle.Render(formatChoiceLabel(choice.Format))
			if choice.Reason != "" {
				label += " " + p.hintStyle.Render("unavailable: "+choice.Reason)
			}
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
