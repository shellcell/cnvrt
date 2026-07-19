package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/shellcell/cnvrt/internal/adapters/settings"
	"github.com/shellcell/cnvrt/internal/adapters/toolconfig"
	"github.com/shellcell/cnvrt/internal/app"
	"github.com/shellcell/cnvrt/internal/domain"
	"github.com/shellcell/cnvrt/internal/ports"
	"github.com/shellcell/cnvrt/internal/theme"
)

type Runner struct {
	service *app.Service
	fs      ports.FileSystem
	stdin   io.Reader
	stdout  io.Writer
	stderr  io.Writer

	titleStyle lipgloss.Style
	okStyle    lipgloss.Style
	skipStyle  lipgloss.Style
	failStyle  lipgloss.Style
	dimStyle   lipgloss.Style
	hintStyle  lipgloss.Style
	palette    theme.Palette
}

func (r *Runner) categoryStyle(category string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(r.palette.CategoryColor(category)))
}

func NewRunner(service *app.Service, fs ports.FileSystem, stdin io.Reader, stdout io.Writer, stderr io.Writer, palettes ...theme.Palette) *Runner {
	palette := theme.Default()
	if len(palettes) > 0 {
		palette = palettes[0]
	}
	return &Runner{
		service:    service,
		fs:         fs,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		titleStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette.Title)),
		okStyle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette.OK)),
		skipStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette.Skip)),
		failStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(palette.Fail)),
		dimStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Dim)),
		hintStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color(palette.Hint)),
		palette:    palette,
	}
}

func (r *Runner) Run(ctx context.Context, args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "doctor":
			r.printDoctor()
			return 0
		case "formats":
			r.printFormats()
			return 0
		case "backends":
			r.printBackends()
			return 0
		case "add-format":
			return r.addFormat()
		case "add-tool":
			return r.addTool()
		case "config":
			return r.editConfig()
		case "help", "-h", "--help":
			r.printUsage()
			return 0
		}
	}

	req, interactive, err := r.parse(args)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n\n", err)
		r.printUsageTo(r.stderr)
		return 2
	}

	start := time.Now()
	var report app.RunReport
	if interactive {
		report, err = r.service.Interactive(ctx, app.InteractiveRequest{
			Root:         req.Root,
			Inputs:       req.Inputs,
			InputFormat:  req.InputFormat,
			OutputFormat: req.OutputFormat,
			OutputDir:    req.OutputDir,
			Overwrite:    req.Overwrite,
			Quality:      req.Quality,
			Action:       req.Action,
			Resize:       req.Resize,
			ToolOptions:  req.ToolOptions,
		})
	} else {
		report, err = r.service.Convert(ctx, app.ConvertRequest{
			Inputs:       req.Inputs,
			OutputPath:   req.OutputPath,
			InputFormat:  req.InputFormat,
			OutputFormat: req.OutputFormat,
			OutputDir:    req.OutputDir,
			Overwrite:    req.Overwrite,
			Quality:      req.Quality,
			Action:       req.Action,
			Resize:       req.Resize,
			ToolOptions:  req.ToolOptions,
		})
	}
	if err != nil {
		if errors.Is(err, ports.ErrUserAborted) {
			report.Items = append(report.Items, app.JobReport{
				Status:  app.StatusSkipped,
				Message: "cancelled",
			})
			err = nil
		} else {
			report.Items = append(report.Items, app.JobReport{
				Status:  app.StatusFailed,
				Message: err.Error(),
				Err:     err,
			})
		}
	}

	r.printReport(report, time.Since(start))
	if err != nil || report.HasFailures() {
		return 1
	}
	return 0
}

type parsedRequest struct {
	Inputs       []string
	Root         string
	OutputPath   string
	InputFormat  domain.Format
	OutputFormat domain.Format
	OutputDir    string
	Overwrite    bool
	Quality      int
	Action       domain.TransformAction
	Resize       string
	ToolOptions  domain.ToolOptions
}

func (r *Runner) parse(args []string) (parsedRequest, bool, error) {
	flags := flag.NewFlagSet("cnvrt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	inputFormatFlag := flags.String("i", "", "input format")
	flags.StringVar(inputFormatFlag, "input-format", "", "input format")
	outputFormatFlag := flags.String("o", "", "output format")
	flags.StringVar(outputFormatFlag, "output-format", "", "output format")
	outputDir := flags.String("out-dir", "", "output directory")
	overwrite := flags.Bool("overwrite", false, "overwrite existing output files")
	quality := flags.Int("quality", 0, "best-effort quality value passed to supported backends")
	actionFlag := flags.String("action", "", "same-format action: convert, compress, or resize")
	compress := flags.Bool("compress", false, "compress same-format image output")
	resize := flags.String("resize", "", "resize value for supported image backends, for example 800x or 50%")

	toolOptions := domain.ToolOptions{}
	flags.Func("opt", "backend option as tool.key=value, repeatable (example: ffmpeg.audio_bitrate=320k)", func(value string) error {
		return parseToolOption(toolOptions, value)
	})

	if err := flags.Parse(args); err != nil {
		return parsedRequest{}, false, err
	}

	var inputFormat domain.Format
	if *inputFormatFlag != "" {
		format, err := domain.ParseFormat(*inputFormatFlag)
		if err != nil {
			return parsedRequest{}, false, err
		}
		inputFormat = format
	}

	var outputFormat domain.Format
	if *outputFormatFlag != "" {
		format, err := domain.ParseFormat(*outputFormatFlag)
		if err != nil {
			return parsedRequest{}, false, err
		}
		outputFormat = format
	}

	action, err := parseAction(*actionFlag)
	if err != nil {
		return parsedRequest{}, false, err
	}
	if *compress {
		action = domain.ActionCompress
	}
	if *resize != "" && action == "" {
		action = domain.ActionResize
	}

	positional := flags.Args()
	req := parsedRequest{
		InputFormat:  inputFormat,
		OutputFormat: outputFormat,
		OutputDir:    *outputDir,
		Overwrite:    *overwrite,
		Quality:      *quality,
		Action:       action,
		Resize:       *resize,
		ToolOptions:  toolOptions,
	}

	if len(positional) == 0 {
		return req, true, nil
	}

	if outputFormat != "" {
		if len(positional) == 1 {
			isDir, err := r.isDirIfExists(positional[0])
			if err != nil {
				return parsedRequest{}, false, err
			}
			if isDir && outputFormat.IsArchive() {
				req.Inputs = positional
				return req, false, nil
			}
			if isDir {
				req.Root = positional[0]
				return req, true, nil
			}
		}

		req.Inputs = positional
		return req, false, nil
	}

	if inputFormat != "" && outputFormat == "" {
		return parsedRequest{}, false, errors.New("-i/--input-format requires -o/--output-format")
	}

	switch len(positional) {
	case 1:
		isDir, err := r.isDirIfExists(positional[0])
		if err != nil {
			return parsedRequest{}, false, err
		}
		if isDir {
			req.Root = positional[0]
			return req, true, nil
		}
		req.Inputs = positional
		return req, true, nil
	case 2:
		req.Inputs = []string{positional[0]}
		req.OutputPath = positional[1]
		return req, false, nil
	default:
		return parsedRequest{}, false, errors.New("multiple inputs require -o/--output-format")
	}
}

// parseToolOption parses "tool.key=value" (or "tool:key=value") flag values
// into the shared ToolOptions map; repeated keys append.
func parseToolOption(options domain.ToolOptions, value string) error {
	pair := strings.SplitN(value, "=", 2)
	if len(pair) != 2 || strings.TrimSpace(pair[1]) == "" {
		return fmt.Errorf("invalid option %q; use tool.key=value", value)
	}
	name := strings.TrimSpace(pair[0])
	separator := strings.IndexAny(name, ".:")
	if separator <= 0 || separator == len(name)-1 {
		return fmt.Errorf("invalid option %q; use tool.key=value", value)
	}
	tool := strings.ToLower(strings.TrimSpace(name[:separator]))
	key := strings.ToLower(strings.TrimSpace(name[separator+1:]))
	if options[tool] == nil {
		options[tool] = map[string][]string{}
	}
	options[tool][key] = append(options[tool][key], strings.TrimSpace(pair[1]))
	return nil
}

func parseAction(value string) (domain.TransformAction, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "":
		return "", nil
	case string(domain.ActionConvert):
		return domain.ActionConvert, nil
	case string(domain.ActionCompress):
		return domain.ActionCompress, nil
	case string(domain.ActionResize):
		return domain.ActionResize, nil
	default:
		return "", fmt.Errorf("unknown action: %s", value)
	}
}

func (r *Runner) isDirIfExists(path string) (bool, error) {
	exists, err := r.fs.Exists(path)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	return r.fs.IsDir(path)
}

func (r *Runner) printUsage() {
	r.printUsageTo(r.stdout)
}

func (r *Runner) printUsageTo(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  cnvrt")
	fmt.Fprintln(w, "  cnvrt input.svg output.png")
	fmt.Fprintln(w, "  cnvrt input.svg")
	fmt.Fprintln(w, "  cnvrt -i svg -o png abc.svg cde.svg")
	fmt.Fprintln(w, "  cnvrt -o png")
	fmt.Fprintln(w, "  cnvrt -o png ../../")
	fmt.Fprintln(w, "  cnvrt -o zip ./directory")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  -i, --input-format   override input format")
	fmt.Fprintln(w, "  -o, --output-format  output format")
	fmt.Fprintln(w, "      --out-dir        output directory")
	fmt.Fprintln(w, "      --overwrite      overwrite existing output files")
	fmt.Fprintln(w, "      --quality        best-effort quality value for supported backends")
	fmt.Fprintln(w, "      --compress       compress same-format image output")
	fmt.Fprintln(w, "      --resize         resize value for supported image and video backends")
	fmt.Fprintln(w, "      --action         same-format action: convert, compress, or resize")
	fmt.Fprintln(w, "      --opt            backend option as tool.key=value, repeatable")
	fmt.Fprintln(w, "                       examples: ffmpeg.audio_bitrate=320k, 7z.level=9,")
	fmt.Fprintln(w, "                       structured.text_style=raw, imagemagick.args=\"-sharpen 0x1\"")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  doctor    check external converter dependencies")
	fmt.Fprintln(w, "  formats   list known and config-registered formats")
	fmt.Fprintln(w, "  backends  list converter backends")
	fmt.Fprintln(w, "  config    interactively edit and save user settings")
	fmt.Fprintln(w, "  add-format  interactively add a config-defined format")
	fmt.Fprintln(w, "  add-tool    interactively add a config-defined converter tool")
	fmt.Fprintln(w, "  version     print version and exit (also -v, --version)")
}

func (r *Runner) addFormat() int {
	reader := bufio.NewReader(r.stdin)
	fmt.Fprintln(r.stdout, r.titleStyle.Render("Add format"))

	name, err := r.ask(reader, "Format name", true)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	aliases, err := r.ask(reader, "Aliases, comma separated", false)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	extensions, err := r.ask(reader, "Extensions, comma separated", false)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	category, err := r.ask(reader, "Category", false)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}

	config := toolconfig.Config{Formats: []toolconfig.FormatConfig{{
		Name:       name,
		Aliases:    splitCSV(aliases),
		Extensions: splitCSV(extensions),
		Category:   category,
	}}}

	path, err := toolconfig.WriteUserConfig("format-"+name, config)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(r.stdout, "%s %s\n", r.okStyle.Render("saved"), path)
	fmt.Fprintln(r.stdout, r.dimStyle.Render("The format will be available on the next run."))
	return 0
}

func (r *Runner) addTool() int {
	reader := bufio.NewReader(r.stdin)
	fmt.Fprintln(r.stdout, r.titleStyle.Render("Add converter tool"))
	fmt.Fprintln(r.stdout, r.dimStyle.Render("Use placeholders: {input}, {output}, {input_format}, {output_format}, {quality}, {resize}."))

	id, err := r.ask(reader, "Tool id", true)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	commandsLine, err := r.ask(reader, "Required command binaries, comma separated", true)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	templateLine, err := r.ask(reader, "Convert command template", false)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	capabilitiesLine, err := r.ask(reader, "Capabilities input:output, comma separated", true)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	installLine, err := r.ask(reader, "Install methods manager=package, comma separated", false)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}

	commands := splitCSV(commandsLine)
	template := parseCommandTemplate(templateLine, commands)
	capabilities, err := parseCapabilityConfigs(capabilitiesLine)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}

	config := toolconfig.Config{Tools: []toolconfig.ToolConfig{{
		ID:           id,
		Commands:     commands,
		Capabilities: capabilities,
		Convert:      template,
		Install:      parseInstallConfigs(installLine),
	}}}

	path, err := toolconfig.WriteUserConfig("tool-"+id, config)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(r.stdout, "%s %s\n", r.okStyle.Render("saved"), path)
	fmt.Fprintln(r.stdout, r.dimStyle.Render("The tool will be available on the next run."))
	return 0
}

func (r *Runner) ask(reader *bufio.Reader, label string, required bool) (string, error) {
	fmt.Fprintf(r.stdout, "%s ", r.hintStyle.Render(label+":"))
	value, err := reader.ReadString('\n')
	if err != nil && value == "" {
		return "", err
	}
	value = strings.TrimSpace(value)
	if required && value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return value, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(part, "."))
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseCommandTemplate(value string, commands []string) toolconfig.CommandTemplate {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		command := ""
		if len(commands) > 0 {
			command = commands[0]
		}
		return toolconfig.CommandTemplate{Command: command, Args: []string{"{input}", "{output}"}}
	}

	return toolconfig.CommandTemplate{Command: fields[0], Args: fields[1:]}
}

func parseCapabilityConfigs(value string) ([]toolconfig.CapabilityConfig, error) {
	parts := splitCSV(value)
	capabilities := make([]toolconfig.CapabilityConfig, 0, len(parts))
	for _, part := range parts {
		pair := strings.Split(part, ":")
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" || strings.TrimSpace(pair[1]) == "" {
			return nil, fmt.Errorf("invalid capability %q; use input:output", part)
		}
		capabilities = append(capabilities, toolconfig.CapabilityConfig{
			Input:    strings.TrimSpace(pair[0]),
			Output:   strings.TrimSpace(pair[1]),
			Priority: 50,
		})
	}
	return capabilities, nil
}

func parseInstallConfigs(value string) []toolconfig.InstallConfig {
	parts := splitCSV(value)
	installs := make([]toolconfig.InstallConfig, 0, len(parts))
	for _, part := range parts {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			continue
		}
		manager := strings.TrimSpace(pair[0])
		pkg := strings.TrimSpace(pair[1])
		if manager == "" || pkg == "" {
			continue
		}
		installs = append(installs, toolconfig.InstallConfig{Manager: manager, Package: pkg})
	}
	return installs
}

// editConfig interactively updates the user settings file.
func (r *Runner) editConfig() int {
	reader := bufio.NewReader(r.stdin)
	fmt.Fprintln(r.stdout, r.titleStyle.Render("Configure cnvrt"))
	if path, err := settings.UserConfigPath(); err == nil {
		fmt.Fprintln(r.stdout, r.dimStyle.Render("Settings file: "+path))
	}

	themeName, err := r.ask(reader, "Theme (nord, classic; empty keeps current)", false)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	themeName = strings.ToLower(strings.TrimSpace(themeName))
	if themeName != "" && themeName != "nord" && themeName != "classic" {
		fmt.Fprintf(r.stderr, "error: unknown theme %q; use nord or classic\n", themeName)
		return 1
	}

	helpAnswer, err := r.ask(reader, "Show key-hint helper lines? (Y/n)", false)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	showHelp := true
	switch strings.ToLower(strings.TrimSpace(helpAnswer)) {
	case "n", "no", "false", "off", "0":
		showHelp = false
	}

	fmt.Fprintln(r.stdout, r.dimStyle.Render("Preferred backends skip the backend question. Keys are pairs (svg->png) or input formats (pdf)."))
	backendsAnswer, err := r.ask(reader, "Preferred backends, comma separated key=backend (example: svg->png=resvg, pdf=ghostscript)", false)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}
	backendPrefs := map[string]string{}
	for _, part := range splitCSV(backendsAnswer) {
		key, backend, ok := strings.Cut(part, "=")
		key = strings.TrimSpace(key)
		backend = strings.TrimSpace(backend)
		if !ok || key == "" || backend == "" {
			fmt.Fprintf(r.stderr, "error: invalid backend preference %q; use key=backend\n", part)
			return 1
		}
		backendPrefs[key] = backend
	}

	path, err := settings.SaveUserConfig(themeName, showHelp, backendPrefs)
	if err != nil {
		fmt.Fprintf(r.stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintf(r.stdout, "%s %s\n", r.okStyle.Render("saved"), path)
	fmt.Fprintln(r.stdout, r.dimStyle.Render("Settings apply on the next run."))
	return 0
}

// printDoctor renders one aligned status row per backend, with its purpose
// and, for unavailable backends, what is missing and how to install it.
func (r *Runner) printDoctor() {
	reports := r.service.DependencyStatus()

	nameWidth := 0
	for _, report := range reports {
		nameWidth = max(nameWidth, len(report.Backend))
	}

	fmt.Fprintln(r.stdout, r.titleStyle.Render("Backends"))
	ready := 0
	for _, report := range reports {
		var missing []app.CommandStatus
		for _, command := range report.Commands {
			if !command.Found {
				missing = append(missing, command)
			}
		}

		badge := r.okStyle.Render("✓")
		state := r.okStyle.Render("ready")
		if len(report.Commands) == 0 {
			state = r.okStyle.Render("built-in")
		}
		if len(missing) > 0 {
			badge = r.failStyle.Render("✗")
			state = r.failStyle.Render("missing " + joinCommandNames(missing))
		} else {
			ready++
		}

		fmt.Fprintf(r.stdout, "  %s %-*s  %s\n", badge, nameWidth, report.Backend, state)
		if report.Description != "" {
			fmt.Fprintf(r.stdout, "    %s\n", r.dimStyle.Render(report.Description))
		}
		for _, command := range missing {
			for _, hint := range command.Hints {
				fmt.Fprintf(r.stdout, "    %s %s\n", r.hintStyle.Render("install:"), hint.Command)
			}
		}
	}

	fmt.Fprintf(r.stdout, "\n%s %d of %d backends ready\n", r.titleStyle.Render("Summary:"), ready, len(reports))
}

func joinCommandNames(commands []app.CommandStatus) string {
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		names = append(names, command.Name)
	}
	return strings.Join(names, ", ")
}

// printFormats groups the known formats by category, one colored block per
// category with wrapped format names.
func (r *Runner) printFormats() {
	formats := domain.AllFormats()
	grouped := map[string][]string{}
	for _, format := range formats {
		category := domain.CategoryOf(format)
		grouped[category] = append(grouped[category], format.String())
	}

	categories := make([]string, 0, len(grouped))
	for category := range grouped {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	fmt.Fprintf(r.stdout, "%s %s\n", r.titleStyle.Render("Known formats"), r.dimStyle.Render(fmt.Sprintf("(%d)", len(formats))))
	for _, category := range categories {
		names := grouped[category]
		style := r.categoryStyle(category)
		fmt.Fprintf(r.stdout, "\n%s %s\n", style.Bold(true).Render(category), r.dimStyle.Render(fmt.Sprintf("(%d)", len(names))))
		for _, line := range wrapWords(names, 76) {
			fmt.Fprintf(r.stdout, "  %s\n", style.Render(line))
		}
	}
}

// wrapWords joins words into lines no longer than width characters.
func wrapWords(words []string, width int) []string {
	var lines []string
	var current string
	for _, word := range words {
		switch {
		case current == "":
			current = word
		case len(current)+1+len(word) > width:
			lines = append(lines, current)
			current = word
		default:
			current += " " + word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// printBackends shows each backend with its description, required commands,
// and a condensed inputs/outputs summary instead of the full pair matrix.
func (r *Runner) printBackends() {
	converters := r.service.Converters()
	sort.Slice(converters, func(i, j int) bool { return converters[i].ID() < converters[j].ID() })
	for i, converter := range converters {
		if i > 0 {
			fmt.Fprintln(r.stdout)
		}
		fmt.Fprintln(r.stdout, r.titleStyle.Render(converter.ID()))
		if described, ok := converter.(ports.Describable); ok {
			fmt.Fprintf(r.stdout, "  %s\n", r.dimStyle.Render(described.Description()))
		}
		if commands := converter.RequiredCommands(); len(commands) > 0 {
			fmt.Fprintf(r.stdout, "  %s %s\n", r.hintStyle.Render("requires:"), strings.Join(commands, ", "))
		} else {
			fmt.Fprintf(r.stdout, "  %s\n", r.hintStyle.Render("built-in"))
		}

		inputs, outputs := capabilitySummary(converter.Capabilities())
		for _, line := range wrapWords(inputs, 68) {
			fmt.Fprintf(r.stdout, "  %s  %s\n", r.hintStyle.Render("inputs: "), line)
		}
		for _, line := range wrapWords(outputs, 68) {
			fmt.Fprintf(r.stdout, "  %s  %s\n", r.hintStyle.Render("outputs:"), line)
		}
	}
}

func capabilitySummary(caps []domain.ConversionCapability) ([]string, []string) {
	inputSet := map[string]bool{}
	outputSet := map[string]bool{}
	for _, capability := range caps {
		inputSet[capability.Input.String()] = true
		outputSet[capability.Output.String()] = true
	}
	inputs := make([]string, 0, len(inputSet))
	for name := range inputSet {
		inputs = append(inputs, name)
	}
	outputs := make([]string, 0, len(outputSet))
	for name := range outputSet {
		outputs = append(outputs, name)
	}
	sort.Strings(inputs)
	sort.Strings(outputs)
	return inputs, outputs
}

func (r *Runner) printReport(report app.RunReport, elapsed time.Duration) {
	fmt.Fprintln(r.stdout, r.titleStyle.Render("Status report"))
	if len(report.Items) == 0 {
		fmt.Fprintln(r.stdout, r.dimStyle.Render("  No work was performed."))
		return
	}

	for _, item := range report.Items {
		label := r.statusLabel(item.Status)
		line := item.InputPath
		if item.OutputPath != "" {
			line += " -> " + item.OutputPath
		}
		if item.Backend != "" {
			line += " " + r.dimStyle.Render("("+item.Backend+")")
		}
		fmt.Fprintf(r.stdout, "  %s %s\n", label, line)

		if item.Status != app.StatusConverted && item.Message != "" {
			fmt.Fprintf(r.stdout, "    %s %s\n", r.dimStyle.Render("reason:"), item.Message)
		}
		if item.Command != "" && item.Status != app.StatusSkipped {
			for _, commandLine := range strings.Split(item.Command, "\n") {
				fmt.Fprintf(r.stdout, "    %s\n", r.dimStyle.Render("$ "+commandLine))
			}
		}
		for _, hint := range item.InstallHints {
			fmt.Fprintf(r.stdout, "    %s %s\n", r.hintStyle.Render("install:"), hint.Command)
		}
	}

	fmt.Fprintf(
		r.stdout,
		"\n%s %d converted, %d skipped, %d failed %s\n",
		r.titleStyle.Render("Summary:"),
		report.Count(app.StatusConverted),
		report.Count(app.StatusSkipped),
		report.Count(app.StatusFailed),
		r.dimStyle.Render("in "+elapsed.Round(10*time.Millisecond).String()),
	)
}

func (r *Runner) statusLabel(status app.ReportStatus) string {
	switch status {
	case app.StatusConverted:
		return r.okStyle.Render("OK")
	case app.StatusSkipped:
		return r.skipStyle.Render("SKIP")
	default:
		return r.failStyle.Render("FAIL")
	}
}
