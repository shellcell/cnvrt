package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type Service struct {
	converters  []ports.Converter
	discovery   ports.FileDiscovery
	fs          ports.FileSystem
	prompt      ports.Prompt
	runner      ports.CommandRunner
	advisor     ports.InstallAdvisor
	preferences Preferences
	progress    ports.ProgressReporter
}

type ConvertRequest struct {
	Inputs       []string
	OutputPath   string
	InputFormat  domain.Format
	OutputFormat domain.Format
	OutputDir    string
	SourceDirOut bool
	Overwrite    bool
	Quality      int
	Action       domain.TransformAction
	Resize       string
	ToolOptions  domain.ToolOptions
}

type InteractiveRequest struct {
	Root         string
	Inputs       []string
	InputFormat  domain.Format
	OutputFormat domain.Format
	OutputDir    string
	SourceDirOut bool
	Overwrite    bool
	Quality      int
	Action       domain.TransformAction
	Resize       string
	ToolOptions  domain.ToolOptions
}

type ReportStatus string

const (
	StatusConverted ReportStatus = "converted"
	StatusSkipped   ReportStatus = "skipped"
	StatusFailed    ReportStatus = "failed"
)

type JobReport struct {
	InputPath    string
	OutputPath   string
	InputFormat  domain.Format
	OutputFormat domain.Format
	Backend      string
	Status       ReportStatus
	Message      string
	Err          error
	InstallHints []ports.InstallSuggestion
}

type RunReport struct {
	Items []JobReport
}

func (r RunReport) HasFailures() bool {
	for _, item := range r.Items {
		if item.Status != StatusConverted {
			return true
		}
	}
	return false
}

func (r RunReport) Count(status ReportStatus) int {
	count := 0
	for _, item := range r.Items {
		if item.Status == status {
			count++
		}
	}
	return count
}

type DependencyStatus struct {
	Backend  string
	Commands []CommandStatus
}

type CommandStatus struct {
	Name  string
	Found bool
	Hints []ports.InstallSuggestion
}

type missingDependencyError struct {
	message string
	hints   []ports.InstallSuggestion
}

func (e missingDependencyError) Error() string {
	return e.message
}

func NewService(converters []ports.Converter, discovery ports.FileDiscovery, fs ports.FileSystem, prompt ports.Prompt, runner ports.CommandRunner, advisor ports.InstallAdvisor, preferences Preferences, progress ports.ProgressReporter) *Service {
	if progress == nil {
		progress = ports.NoopProgressReporter{}
	}
	return &Service{
		converters:  converters,
		discovery:   discovery,
		fs:          fs,
		prompt:      prompt,
		runner:      runner,
		advisor:     advisor,
		preferences: preferences,
		progress:    progress,
	}
}

func (s *Service) Convert(ctx context.Context, req ConvertRequest) (RunReport, error) {
	report := RunReport{}
	if len(req.Inputs) == 0 {
		return report, fmt.Errorf("%w: no input files", domain.ErrInvalidJob)
	}
	if req.OutputPath != "" && len(req.Inputs) != 1 {
		return report, fmt.Errorf("%w: explicit output path requires exactly one input", domain.ErrInvalidJob)
	}
	if req.OutputDir != "" {
		if err := s.fs.EnsureDir(req.OutputDir); err != nil {
			return report, err
		}
	}

	s.progress.Start(len(req.Inputs))
	defer s.progress.Finish()
	for i, input := range req.Inputs {
		index := i + 1
		job, err := s.buildJob(input, req)
		if err != nil {
			s.progress.JobFailed(index, len(req.Inputs), domain.ConvertJob{InputPath: input}, "", err)
			report.Items = append(report.Items, jobReportFromError(input, domain.Format(""), domain.Format(""), "", err, nil))
			continue
		}

		converter, err := s.pickConverter(job.InputFormat, job.OutputFormat, job.Options)
		if err != nil {
			if errors.Is(err, domain.ErrUnsupportedConvert) {
				s.progress.JobSkipped(index, len(req.Inputs), job.InputPath, job.OutputPath, err.Error())
			} else {
				s.progress.JobFailed(index, len(req.Inputs), job, "", err)
			}
			report.Items = append(report.Items, s.reportForJobError(job, err))
			continue
		}

		s.progress.JobStart(index, len(req.Inputs), job, converter.ID())
		result, err := converter.Convert(ctx, job)
		if err != nil {
			hints := s.installHintsFromError(err)
			s.progress.JobFailed(index, len(req.Inputs), job, converter.ID(), err)
			report.Items = append(report.Items, JobReport{
				InputPath:    job.InputPath,
				OutputPath:   job.OutputPath,
				InputFormat:  job.InputFormat,
				OutputFormat: job.OutputFormat,
				Backend:      converter.ID(),
				Status:       StatusFailed,
				Message:      err.Error(),
				Err:          err,
				InstallHints: hints,
			})
			continue
		}

		s.progress.JobSuccess(index, len(req.Inputs), result.Job, result.Backend)
		report.Items = append(report.Items, JobReport{
			InputPath:    result.Job.InputPath,
			OutputPath:   result.OutputPath,
			InputFormat:  result.Job.InputFormat,
			OutputFormat: result.Job.OutputFormat,
			Backend:      result.Backend,
			Status:       StatusConverted,
			Message:      "ok",
		})
	}

	return report, nil
}

func (s *Service) Interactive(ctx context.Context, req InteractiveRequest) (RunReport, error) {
	inputs := req.Inputs
	if len(inputs) == 0 {
		root := req.Root
		if root == "" {
			var err error
			root, err = s.fs.CurrentDir()
			if err != nil {
				return RunReport{}, err
			}
		}

		files, err := s.discovery.ListFiles(ctx, root)
		if err != nil {
			return RunReport{}, err
		}
		if len(files) == 0 {
			return RunReport{}, fmt.Errorf("no supported input files found in %s", root)
		}

		selected, err := s.prompt.SelectFiles(ctx, files)
		if err != nil {
			return RunReport{}, err
		}
		for _, file := range selected {
			inputs = append(inputs, file.Path)
		}
	}

	outputFormat := req.OutputFormat
	if len(inputs) == 1 && outputFormat == "" {
		inputFormat, err := s.detectInputFormat(inputs[0], req.InputFormat)
		if err != nil {
			return RunReport{}, err
		}
		if inputFormat.IsArchive() {
			action, err := s.prompt.SelectArchiveAction(ctx, domain.FileRef{
				Path:   inputs[0],
				Name:   filepath.Base(inputs[0]),
				Format: inputFormat,
			})
			if err != nil {
				return RunReport{}, err
			}
			switch action {
			case domain.ArchiveActionExtract:
				outputFormat = domain.FormatDir
			case domain.ArchiveActionCancel:
				return RunReport{Items: []JobReport{{
					InputPath:   inputs[0],
					InputFormat: inputFormat,
					Status:      StatusSkipped,
					Message:     "cancelled",
				}}}, nil
			}
		}
	}

	if outputFormat == "" {
		formats, err := s.OutputFormatChoicesForInputs(inputs, req.InputFormat, req.ToolOptions)
		if err != nil {
			return RunReport{}, err
		}
		if len(formats) == 0 {
			return RunReport{}, errors.New("no output formats for selected inputs")
		}

		selectedFormat, err := s.prompt.SelectFormat(ctx, formats)
		if err != nil {
			return RunReport{}, err
		}
		outputFormat = selectedFormat
	}

	sourceDirOut := req.SourceDirOut
	if req.OutputDir == "" {
		currentDir, err := s.fs.CurrentDir()
		if err != nil {
			return RunReport{}, err
		}
		location, err := s.prompt.SelectOutputLocation(ctx, currentDir)
		if err != nil {
			return RunReport{}, err
		}
		sourceDirOut = location == ports.OutputLocationSource
	}

	action := req.Action
	quality := req.Quality
	resize := req.Resize
	if action == "" && outputFormat.IsImage() && s.allInputsMatchFormat(inputs, req.InputFormat, outputFormat) {
		selectedAction, err := s.prompt.SelectSameFormatAction(ctx, outputFormat)
		if err != nil {
			return RunReport{}, err
		}
		action = selectedAction

		switch action {
		case domain.ActionCompress:
			if quality == 0 {
				quality, err = s.prompt.AskQuality(ctx, 85)
				if err != nil {
					return RunReport{}, err
				}
			}
		case domain.ActionResize:
			if resize == "" {
				resize, err = s.prompt.AskResize(ctx)
				if err != nil {
					return RunReport{}, err
				}
			}
		}
	}

	return s.Convert(ctx, ConvertRequest{
		Inputs:       inputs,
		InputFormat:  req.InputFormat,
		OutputFormat: outputFormat,
		OutputDir:    req.OutputDir,
		SourceDirOut: sourceDirOut,
		Overwrite:    req.Overwrite,
		Quality:      quality,
		Action:       action,
		Resize:       resize,
		ToolOptions:  req.ToolOptions,
	})
}

func (s *Service) BuildJobs(req ConvertRequest) ([]domain.ConvertJob, error) {
	if len(req.Inputs) == 0 {
		return nil, fmt.Errorf("%w: no input files", domain.ErrInvalidJob)
	}
	if req.OutputPath != "" && len(req.Inputs) != 1 {
		return nil, fmt.Errorf("%w: explicit output path requires exactly one input", domain.ErrInvalidJob)
	}

	jobs := make([]domain.ConvertJob, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		job, err := s.buildJob(input, req)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *Service) OutputFormatsForInputs(inputs []string, inputOverride domain.Format) ([]domain.Format, error) {
	choices, err := s.OutputFormatChoicesForInputs(inputs, inputOverride, nil)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Format, 0, len(choices))
	for _, choice := range choices {
		result = append(result, choice.Format)
	}
	return result, nil
}

func (s *Service) OutputFormatChoicesForInputs(inputs []string, inputOverride domain.Format, requestOptions domain.ToolOptions) ([]ports.FormatChoice, error) {
	outputs := map[domain.Format]bool{}
	inputFormats := map[domain.Format]bool{}
	for _, input := range inputs {
		inputFormat, err := s.detectInputFormat(input, inputOverride)
		if err != nil {
			return nil, err
		}
		inputFormats[inputFormat] = true

		for _, converter := range s.converters {
			for _, capability := range converter.Capabilities() {
				if capability.Input == inputFormat {
					outputs[capability.Output] = true
				}
			}
		}
	}

	formats := make([]domain.Format, 0, len(outputs))
	for format := range outputs {
		formats = append(formats, format)
	}
	sort.Slice(formats, func(i, j int) bool { return formats[i] < formats[j] })

	choices := make([]ports.FormatChoice, 0, len(formats))
	for _, output := range formats {
		choice := ports.FormatChoice{Format: output}
		var reasons []string
		for input := range inputFormats {
			options := domain.ConvertOptions{ToolOptions: s.preferences.OptionsFor(input, output).Merge(requestOptions)}
			available, reason := s.conversionAvailable(input, output, options)
			if available {
				choice.Available = true
				break
			}
			if reason != "" {
				reasons = append(reasons, reason)
			}
		}
		if !choice.Available {
			choice.Reason = strings.Join(dedupeStrings(reasons), "; ")
			if choice.Reason == "" {
				choice.Reason = "no installed converter"
			}
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

func (s *Service) DependencyStatus() []DependencyStatus {
	reports := make([]DependencyStatus, 0, len(s.converters))
	for _, converter := range s.converters {
		status := DependencyStatus{Backend: converter.ID()}
		for _, command := range converter.RequiredCommands() {
			_, err := s.runner.LookPath(command)
			status.Commands = append(status.Commands, CommandStatus{
				Name:  command,
				Found: err == nil,
				Hints: s.installHints([]string{command}),
			})
		}
		reports = append(reports, status)
	}
	return reports
}

func (s *Service) Converters() []ports.Converter {
	return append([]ports.Converter(nil), s.converters...)
}

func (s *Service) buildJob(input string, req ConvertRequest) (domain.ConvertJob, error) {
	inputFormat, err := s.detectInputFormat(input, req.InputFormat)
	if err != nil {
		return domain.ConvertJob{}, err
	}

	outputFormat := req.OutputFormat
	if outputFormat == "" && req.OutputPath != "" {
		outputFormat, err = s.detectOutputFormat(req.OutputPath)
		if err != nil {
			return domain.ConvertJob{}, err
		}
	}
	if outputFormat == "" {
		return domain.ConvertJob{}, fmt.Errorf("%w: output format is required", domain.ErrInvalidJob)
	}

	options := domain.ConvertOptions{
		Overwrite:   req.Overwrite,
		Quality:     req.Quality,
		Action:      req.Action,
		Resize:      req.Resize,
		ToolOptions: s.preferences.OptionsFor(inputFormat, outputFormat).Merge(req.ToolOptions),
	}

	outputPath := req.OutputPath
	outputDir := req.OutputDir
	if outputPath == "" && outputDir == "" && !req.SourceDirOut {
		outputDir, err = s.fs.CurrentDir()
		if err != nil {
			return domain.ConvertJob{}, err
		}
	}
	if outputPath == "" {
		outputPath = outputPathFor(input, inputFormat, outputFormat, outputDir, options)
	}

	if err := s.validateOutput(input, outputPath, req.Overwrite); err != nil {
		return domain.ConvertJob{}, err
	}
	if outputFormat != domain.FormatDir {
		if err := s.fs.EnsureDir(filepath.Dir(outputPath)); err != nil {
			return domain.ConvertJob{}, err
		}
	}

	return domain.ConvertJob{
		InputPath:    input,
		OutputPath:   outputPath,
		InputFormat:  inputFormat,
		OutputFormat: outputFormat,
		Options:      options,
	}, nil
}

func (s *Service) detectInputFormat(input string, override domain.Format) (domain.Format, error) {
	if override != "" {
		return override, nil
	}

	if isDir, err := s.fs.IsDir(input); err == nil && isDir {
		return domain.FormatDir, nil
	}

	format, err := domain.FormatFromPath(input)
	if err != nil {
		if s.textFallback(input) {
			return domain.FormatTXT, nil
		}
		return "", err
	}
	if !domain.IsRegisteredFormat(format) && !s.hasInputCapability(format) && s.textFallback(input) {
		return domain.FormatTXT, nil
	}
	return format, nil
}

func (s *Service) textFallback(input string) bool {
	text, err := s.fs.IsTextFile(input)
	return err == nil && text
}

func (s *Service) hasInputCapability(format domain.Format) bool {
	for _, converter := range s.converters {
		for _, capability := range converter.Capabilities() {
			if capability.Input == format {
				return true
			}
		}
	}
	return false
}

func (s *Service) detectOutputFormat(output string) (domain.Format, error) {
	if isDir, err := s.fs.IsDir(output); err == nil && isDir {
		return domain.FormatDir, nil
	}

	return domain.FormatFromPath(output)
}

func (s *Service) validateOutput(input string, output string, overwrite bool) error {
	inputAbs, err := s.fs.Abs(input)
	if err != nil {
		return err
	}
	outputAbs, err := s.fs.Abs(output)
	if err != nil {
		return err
	}
	if inputAbs == outputAbs {
		return fmt.Errorf("%w: input and output path are the same: %s", domain.ErrInvalidJob, output)
	}

	exists, err := s.fs.Exists(output)
	if err != nil {
		return err
	}
	if exists && !overwrite {
		return fmt.Errorf("%w: output already exists: %s", domain.ErrInvalidJob, output)
	}

	return nil
}

func (s *Service) pickConverter(input domain.Format, output domain.Format, options domain.ConvertOptions) (ports.Converter, error) {
	var capable []ports.Converter
	var missing []string
	var hints []ports.InstallSuggestion
	for _, converter := range s.preferences.OrderConverters(input, output, s.converters) {
		if !converter.CanConvert(input, output) {
			continue
		}

		capable = append(capable, converter)
		missingCommands := s.missingDependencies(converter, input, output, options)
		if len(missingCommands) == 0 {
			return converter, nil
		}

		missing = append(missing, fmt.Sprintf("%s requires %s", converter.ID(), strings.Join(missingCommands, ", ")))
		hints = append(hints, s.installHints(missingCommands)...)
	}

	if len(capable) == 0 {
		return nil, fmt.Errorf("%w: %s -> %s", domain.ErrUnsupportedConvert, input, output)
	}

	return nil, missingDependencyError{
		message: fmt.Sprintf("%s: %s", domain.ErrMissingDependency, strings.Join(missing, "; ")),
		hints:   dedupeHints(hints),
	}
}

func (s *Service) conversionAvailable(input domain.Format, output domain.Format, options domain.ConvertOptions) (bool, string) {
	var reasons []string
	for _, converter := range s.preferences.OrderConverters(input, output, s.converters) {
		if !converter.CanConvert(input, output) {
			continue
		}

		missing := s.missingDependencies(converter, input, output, options)
		if len(missing) == 0 {
			return true, ""
		}
		reasons = append(reasons, fmt.Sprintf("%s requires %s", converter.ID(), strings.Join(missing, ", ")))
	}
	if len(reasons) == 0 {
		return false, fmt.Sprintf("unsupported conversion: %s -> %s", input, output)
	}
	return false, strings.Join(dedupeStrings(reasons), "; ")
}

func (s *Service) missingDependencies(converter ports.Converter, input domain.Format, output domain.Format, options domain.ConvertOptions) []string {
	missing := s.missingCommands(converter.RequiredCommands())
	if aware, ok := converter.(ports.RuntimeDependencyAware); ok {
		missing = append(missing, aware.MissingDependencies(input, output, options)...)
	}
	return dedupeStrings(missing)
}

func (s *Service) missingCommands(commands []string) []string {
	var missing []string
	for _, command := range commands {
		if _, err := s.runner.LookPath(command); err != nil {
			missing = append(missing, command)
		}
	}
	return missing
}

func (s *Service) installHints(commands []string) []ports.InstallSuggestion {
	if s.advisor == nil {
		return nil
	}

	var hints []ports.InstallSuggestion
	for _, command := range commands {
		hints = append(hints, s.advisor.Suggestions(command)...)
	}
	return dedupeHints(hints)
}

func (s *Service) installHintsFromError(err error) []ports.InstallSuggestion {
	var depErr domain.MissingDependencyError
	if errors.As(err, &depErr) {
		return s.installHints(depErr.Commands)
	}
	return nil
}

func (s *Service) reportForJobError(job domain.ConvertJob, err error) JobReport {
	var depErr missingDependencyError
	var hints []ports.InstallSuggestion
	if errors.As(err, &depErr) {
		hints = depErr.hints
	}

	return jobReportFromError(job.InputPath, job.InputFormat, job.OutputFormat, job.OutputPath, err, hints)
}

func jobReportFromError(input string, inputFormat domain.Format, outputFormat domain.Format, output string, err error, hints []ports.InstallSuggestion) JobReport {
	status := StatusFailed
	if errors.Is(err, domain.ErrUnsupportedConvert) {
		status = StatusSkipped
	}

	return JobReport{
		InputPath:    input,
		OutputPath:   output,
		InputFormat:  inputFormat,
		OutputFormat: outputFormat,
		Status:       status,
		Message:      err.Error(),
		Err:          err,
		InstallHints: hints,
	}
}

func (s *Service) allInputsMatchFormat(inputs []string, inputOverride domain.Format, output domain.Format) bool {
	if len(inputs) == 0 {
		return false
	}

	for _, input := range inputs {
		format, err := s.detectInputFormat(input, inputOverride)
		if err != nil || format != output {
			return false
		}
	}
	return true
}

func outputPathFor(input string, inputFormat domain.Format, outputFormat domain.Format, outputDir string, options domain.ConvertOptions) string {
	base := baseNameWithoutFormat(input, inputFormat)
	if inputFormat == outputFormat {
		switch options.Action {
		case domain.ActionCompress:
			base += ".compressed"
		case domain.ActionResize:
			base += ".resized"
		default:
			base += ".converted"
		}
	}

	dir := filepath.Dir(input)
	if outputDir != "" {
		dir = outputDir
	}

	if outputFormat == domain.FormatDir {
		return filepath.Join(dir, base)
	}

	return filepath.Join(dir, base+"."+outputFormat.Extension())
}

func baseNameWithoutFormat(path string, format domain.Format) string {
	base := filepath.Base(filepath.Clean(path))
	if base == "." || base == ".." || base == string(filepath.Separator) || base == "" {
		base = "archive"
	}

	ext := format.Extension()
	if ext != "" {
		suffix := "." + ext
		if strings.HasSuffix(strings.ToLower(base), suffix) {
			base = base[:len(base)-len(suffix)]
		}
	} else {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}

	if base == "" {
		return "output"
	}
	return base
}

func dedupeHints(hints []ports.InstallSuggestion) []ports.InstallSuggestion {
	seen := map[string]bool{}
	result := make([]ports.InstallSuggestion, 0, len(hints))
	for _, hint := range hints {
		key := hint.Manager + "\x00" + hint.Command
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, hint)
	}
	return result
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
