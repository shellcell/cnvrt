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

	"charm.land/lipgloss/v2"
	"github.com/shellcell/convert/internal/adapters/toolconfig"
	"github.com/shellcell/convert/internal/app"
	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
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
}

func NewRunner(service *app.Service, fs ports.FileSystem, stdin io.Reader, stdout io.Writer, stderr io.Writer) *Runner {
	return &Runner{
		service:    service,
		fs:         fs,
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		titleStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		okStyle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
		skipStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
		failStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		dimStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		hintStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
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

	r.printReport(report)
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
}

func (r *Runner) parse(args []string) (parsedRequest, bool, error) {
	flags := flag.NewFlagSet("convert", flag.ContinueOnError)
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
	fmt.Fprintln(w, "  convert")
	fmt.Fprintln(w, "  convert input.svg output.png")
	fmt.Fprintln(w, "  convert input.svg")
	fmt.Fprintln(w, "  convert -i svg -o png abc.svg cde.svg")
	fmt.Fprintln(w, "  convert -o png")
	fmt.Fprintln(w, "  convert -o png ../../")
	fmt.Fprintln(w, "  convert -o zip ./directory")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  -i, --input-format   override input format")
	fmt.Fprintln(w, "  -o, --output-format  output format")
	fmt.Fprintln(w, "      --out-dir        output directory")
	fmt.Fprintln(w, "      --overwrite      overwrite existing output files")
	fmt.Fprintln(w, "      --quality        best-effort quality value for supported backends")
	fmt.Fprintln(w, "      --compress       compress same-format image output")
	fmt.Fprintln(w, "      --resize         resize value for supported image backends")
	fmt.Fprintln(w, "      --action         same-format action: convert, compress, or resize")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  doctor    check external converter dependencies")
	fmt.Fprintln(w, "  formats   list known and config-registered formats")
	fmt.Fprintln(w, "  backends  list converter backends")
	fmt.Fprintln(w, "  add-format  interactively add a config-defined format")
	fmt.Fprintln(w, "  add-tool    interactively add a config-defined converter tool")
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

func (r *Runner) printDoctor() {
	fmt.Fprintln(r.stdout, r.titleStyle.Render("Backends"))
	for _, report := range r.service.DependencyStatus() {
		if len(report.Commands) == 0 {
			fmt.Fprintf(r.stdout, "  %-14s %s\n", report.Backend, r.okStyle.Render("built-in"))
			continue
		}

		fmt.Fprintf(r.stdout, "  %s\n", report.Backend)
		for _, command := range report.Commands {
			state := r.failStyle.Render("missing")
			if command.Found {
				state = r.okStyle.Render("found")
			}
			fmt.Fprintf(r.stdout, "    %-18s %s\n", command.Name, state)
			if !command.Found {
				for _, hint := range command.Hints {
					fmt.Fprintf(r.stdout, "      %s %s\n", r.hintStyle.Render("install:"), hint.Command)
				}
			}
		}
	}
}

func (r *Runner) printFormats() {
	formats := domain.AllFormats()
	fmt.Fprintln(r.stdout, r.titleStyle.Render("Known formats"))
	for _, format := range formats {
		fmt.Fprintf(r.stdout, "  %s\n", format)
	}
}

func (r *Runner) printBackends() {
	converters := r.service.Converters()
	sort.Slice(converters, func(i, j int) bool { return converters[i].ID() < converters[j].ID() })
	for _, converter := range converters {
		fmt.Fprintln(r.stdout, r.titleStyle.Render(converter.ID()))
		caps := converter.Capabilities()
		sort.Slice(caps, func(i, j int) bool {
			if caps[i].Input == caps[j].Input {
				return caps[i].Output < caps[j].Output
			}
			return caps[i].Input < caps[j].Input
		})
		for _, capability := range caps {
			fmt.Fprintf(r.stdout, "  %s -> %s\n", capability.Input, capability.Output)
		}
	}
}

func (r *Runner) printReport(report app.RunReport) {
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
		for _, hint := range item.InstallHints {
			fmt.Fprintf(r.stdout, "    %s %s\n", r.hintStyle.Render("install:"), hint.Command)
		}
	}

	fmt.Fprintf(
		r.stdout,
		"\n%s %d converted, %d skipped, %d failed\n",
		r.titleStyle.Render("Summary:"),
		report.Count(app.StatusConverted),
		report.Count(app.StatusSkipped),
		report.Count(app.StatusFailed),
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
