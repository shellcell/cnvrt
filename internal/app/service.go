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
	"github.com/shellcell/convert/internal/shell"
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

	// formatCache remembers detected input formats per path; detection can
	// stat and read files and runs several times per interactive flow.
	formatCache map[string]domain.Format
	// backendChoice remembers the user's backend pick per conversion pair so
	// a batch asks at most once per pair.
	backendChoice map[string]string
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
	Command      string
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
	Backend     string
	Description string
	Commands    []CommandStatus
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

type commandReview struct {
	editedCommand string
	cancelled     bool
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
		progress:      progress,
		formatCache:   map[string]domain.Format{},
		backendChoice: map[string]string{},
	}
}

// plannedJob is one input after planning: either a runnable job with its
// selected converter, or the error that stopped it from becoming runnable.
type plannedJob struct {
	input     string
	job       domain.ConvertJob
	converter ports.Converter
	err       error
	built     bool
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

	plans := s.planJobs(ctx, req)

	review, err := s.reviewBatch(ctx, plans)
	if err != nil {
		return report, err
	}

	s.progress.Start(len(plans))
	defer s.progress.Finish()
	for i, plan := range plans {
		index := i + 1
		switch {
		case plan.err != nil && !plan.built:
			s.progress.JobFailed(index, len(plans), domain.ConvertJob{InputPath: plan.input}, "", plan.err)
			report.Items = append(report.Items, jobReportFromError(plan.input, "", "", "", plan.err, nil))
		case plan.err != nil:
			if errors.Is(plan.err, domain.ErrUnsupportedConvert) {
				s.progress.JobSkipped(index, len(plans), plan.job.InputPath, plan.job.OutputPath, plan.err.Error())
			} else {
				s.progress.JobFailed(index, len(plans), plan.job, "", plan.err)
			}
			report.Items = append(report.Items, s.reportForJobError(plan.job, plan.err))
		case review.cancelled:
			s.progress.JobSkipped(index, len(plans), plan.job.InputPath, plan.job.OutputPath, "cancelled")
			report.Items = append(report.Items, JobReport{
				InputPath:    plan.job.InputPath,
				OutputPath:   plan.job.OutputPath,
				InputFormat:  plan.job.InputFormat,
				OutputFormat: plan.job.OutputFormat,
				Backend:      plan.converter.ID(),
				Status:       StatusSkipped,
				Message:      "cancelled",
			})
		default:
			report.Items = append(report.Items, s.executeJob(ctx, index, len(plans), plan, review.editedCommand))
		}
	}

	return report, nil
}

func (s *Service) planJobs(ctx context.Context, req ConvertRequest) []plannedJob {
	plans := make([]plannedJob, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		plan := plannedJob{input: input}
		job, err := s.buildJob(ctx, input, req)
		if err != nil {
			plan.err = err
			plans = append(plans, plan)
			continue
		}
		plan.job = job
		plan.built = true

		converter, err := s.pickConverterForJob(ctx, job)
		if err != nil {
			plan.err = err
			plans = append(plans, plan)
			continue
		}
		plan.converter = converter
		plans = append(plans, plan)
	}
	return plans
}

// pickConverterForJob resolves the backend for a job. When several backends
// are installed and capable, interactive mode asks the user once per
// conversion pair; the answer is reused for the rest of the batch.
func (s *Service) pickConverterForJob(ctx context.Context, job domain.ConvertJob) (ports.Converter, error) {
	available := s.availableConverters(job.InputFormat, job.OutputFormat, job.Options)
	if s.prompt == nil || len(available) < 2 {
		return s.pickConverter(job.InputFormat, job.OutputFormat, job.Options)
	}
	// A configured backend preference answers the question up front.
	if preferred := s.preferences.PreferredConverter(job.InputFormat, job.OutputFormat, available); preferred != nil {
		return preferred, nil
	}

	pairKey := job.InputFormat.String() + "\x00" + job.OutputFormat.String()
	if chosen, ok := s.backendChoice[pairKey]; ok {
		for _, converter := range available {
			if converter.ID() == chosen {
				return converter, nil
			}
		}
	}

	choices := make([]ports.BackendChoice, 0, len(available))
	for _, converter := range available {
		choice := ports.BackendChoice{ID: converter.ID()}
		if described, ok := converter.(ports.Describable); ok {
			choice.Description = described.Description()
		}
		choices = append(choices, choice)
	}

	selected, err := s.prompt.SelectBackend(ctx, job.InputFormat, job.OutputFormat, choices)
	if err != nil {
		return nil, err
	}
	for _, converter := range available {
		if converter.ID() == selected {
			s.backendChoice[pairKey] = selected
			return converter, nil
		}
	}
	return nil, fmt.Errorf("%w: unknown backend: %s", domain.ErrInvalidJob, selected)
}

// availableConverters lists capable converters with all dependencies
// installed, best first.
func (s *Service) availableConverters(input domain.Format, output domain.Format, options domain.ConvertOptions) []ports.Converter {
	var available []ports.Converter
	for _, converter := range s.preferences.OrderConverters(input, output, s.convertersByPriority(input, output)) {
		if !converter.CanConvert(input, output) {
			continue
		}
		if len(s.missingDependencies(converter, input, output, options)) == 0 {
			available = append(available, converter)
		}
	}
	return available
}

func (s *Service) executeJob(ctx context.Context, index int, total int, plan plannedJob, editedCommand string) JobReport {
	job := plan.job
	converter := plan.converter
	s.progress.JobStart(index, total, job, converter.ID())

	commandUsed := editedCommand
	if commandUsed == "" {
		if previewer, ok := converter.(ports.CommandPreviewer); ok {
			commandUsed = previewCommandLines(previewer.PreviewCommands(job))
		}
	}

	var result domain.ConversionResult
	var err error
	if editedCommand != "" {
		if override, ok := converter.(ports.CommandOverrideConverter); ok {
			result, err = override.ConvertWithCommand(ctx, job, editedCommand)
		} else {
			result, err = s.runEditedCommand(ctx, editedCommand, job, converter.ID())
		}
	} else {
		result, err = converter.Convert(ctx, job)
	}
	if err == nil {
		err = s.verifyOutput(result.OutputPath)
	}
	if err != nil {
		s.progress.JobFailed(index, total, job, converter.ID(), err)
		return JobReport{
			InputPath:    job.InputPath,
			OutputPath:   job.OutputPath,
			InputFormat:  job.InputFormat,
			OutputFormat: job.OutputFormat,
			Backend:      converter.ID(),
			Command:      commandUsed,
			Status:       StatusFailed,
			Message:      err.Error(),
			Err:          err,
			InstallHints: s.installHintsFromError(err),
		}
	}

	s.progress.JobSuccess(index, total, result.Job, result.Backend)
	return JobReport{
		InputPath:    result.Job.InputPath,
		OutputPath:   result.OutputPath,
		InputFormat:  result.Job.InputFormat,
		OutputFormat: result.Job.OutputFormat,
		Backend:      result.Backend,
		Command:      commandUsed,
		Status:       StatusConverted,
		Message:      "ok",
	}
}

// verifyOutput guards against tools that exit zero without producing the
// requested file (fontforge does this for unsupported bitmap generations).
func (s *Service) verifyOutput(outputPath string) error {
	if outputPath == "" {
		return nil
	}
	exists, err := s.fs.Exists(outputPath)
	if err != nil {
		return nil
	}
	if !exists {
		return fmt.Errorf("converter reported success but output was not created: %s", outputPath)
	}
	return nil
}

func previewCommandLines(preview ports.CommandPreview) string {
	if len(preview.Commands) == 0 {
		return ""
	}
	lines := make([]string, 0, len(preview.Commands))
	for _, command := range preview.Commands {
		lines = append(lines, shell.Line(command))
	}
	return strings.Join(lines, "\n")
}

// reviewBatch asks for one confirmation covering every runnable job in the
// batch. In-process converters without external commands run unconfirmed;
// editing is offered only when the batch contains exactly one job.
func (s *Service) reviewBatch(ctx context.Context, plans []plannedJob) (commandReview, error) {
	if s.prompt == nil {
		return commandReview{}, nil
	}

	var runnable []plannedJob
	for _, plan := range plans {
		if plan.err == nil && plan.converter != nil {
			runnable = append(runnable, plan)
		}
	}
	if len(runnable) == 0 {
		return commandReview{}, nil
	}

	review := ports.CommandReview{JobCount: len(runnable)}
	backends := make([]string, 0, 2)
	seenBackend := map[string]bool{}
	var preview ports.CommandPreview
	for _, plan := range runnable {
		id := plan.converter.ID()
		if seenBackend[id] {
			continue
		}
		seenBackend[id] = true
		backends = append(backends, id)

		if previewer, ok := plan.converter.(ports.CommandPreviewer); ok {
			jobPreview := previewer.PreviewCommands(plan.job)
			if len(jobPreview.Commands) > 0 {
				review.Commands = append(review.Commands, jobPreview.Commands...)
				preview = jobPreview
			}
		}
	}
	review.Backend = strings.Join(backends, ", ")

	// Nothing shells out: no command to review, run without a prompt.
	if len(review.Commands) == 0 {
		return commandReview{}, nil
	}
	if len(runnable) == 1 {
		review.Editable = preview.Editable
		review.EditCommand = editableCommandLine(preview)
	}

	action, editedCommand, err := s.prompt.ConfirmCommand(ctx, review)
	if err != nil {
		return commandReview{}, err
	}
	switch action {
	case ports.CommandReviewProceed:
		return commandReview{}, nil
	case ports.CommandReviewEdit:
		editedCommand = strings.TrimSpace(editedCommand)
		if editedCommand == "" {
			return commandReview{}, fmt.Errorf("%w: command is required", domain.ErrInvalidJob)
		}
		return commandReview{editedCommand: editedCommand}, nil
	case ports.CommandReviewCancel:
		return commandReview{cancelled: true}, nil
	default:
		return commandReview{}, fmt.Errorf("%w: unknown command review action: %s", domain.ErrInvalidJob, action)
	}
}

func (s *Service) runEditedCommand(ctx context.Context, command string, job domain.ConvertJob, backend string) (domain.ConversionResult, error) {
	if s.runner == nil {
		return domain.ConversionResult{}, errors.New("command runner is required")
	}
	result, err := s.runner.Run(ctx, shell.Command(command))
	if err != nil {
		return domain.ConversionResult{}, shell.CommandStringError(command, result, err)
	}
	return domain.ConversionResult{Job: job, Backend: backend, OutputPath: job.OutputPath}, nil
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
	if resize == "" && s.shouldAskSVGOutputSize(inputs, req.InputFormat, outputFormat) {
		useSize, err := s.prompt.ConfirmOption(ctx, "Set output size?", "Optional. Choose No to preserve source dimensions when possible.", false)
		if err != nil {
			return RunReport{}, err
		}
		if useSize {
			selectedResize, err := s.prompt.AskOutputSize(ctx, s.defaultOutputSize(inputs, req.InputFormat, "1024x1024"))
			if err != nil {
				return RunReport{}, err
			}
			resize = selectedResize
		}
	}
	if action == "" && supportsSameFormatAction(outputFormat) && s.allInputsMatchFormat(inputs, req.InputFormat, outputFormat) {
		selectedAction, err := s.prompt.SelectSameFormatAction(ctx, outputFormat)
		if err != nil {
			return RunReport{}, err
		}
		action = selectedAction

		switch action {
		case domain.ActionCompress:
			if quality == 0 && supportsQuality(outputFormat) {
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

	toolOptions, err := s.askConverterOptions(ctx, inputs, req.InputFormat, outputFormat, req.ToolOptions)
	if err != nil {
		return RunReport{}, err
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
		ToolOptions:  toolOptions,
	})
}

// askConverterOptions offers the tunable options declared by the backends
// that will run this batch. Skipping keeps every backend default.
func (s *Service) askConverterOptions(ctx context.Context, inputs []string, inputOverride domain.Format, output domain.Format, requestOptions domain.ToolOptions) (domain.ToolOptions, error) {
	specs := s.optionSpecsForBatch(inputs, inputOverride, output, requestOptions)
	if len(specs) == 0 {
		return requestOptions, nil
	}

	configure, err := s.prompt.ConfirmOption(ctx, "Set converter options?", "Optional. Choose No to use the backend defaults.", false)
	if err != nil {
		return nil, err
	}
	if !configure {
		return requestOptions, nil
	}

	options := requestOptions.Clone()
	for _, spec := range specs {
		value, err := s.prompt.AskText(ctx, spec.Title, spec.Description, spec.Default)
		if err != nil {
			return nil, err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		options = options.Merge(domain.ToolOptions{
			strings.ToLower(spec.Tool): {strings.ToLower(spec.Key): []string{value}},
		})
	}
	return options, nil
}

func (s *Service) optionSpecsForBatch(inputs []string, inputOverride domain.Format, output domain.Format, requestOptions domain.ToolOptions) []ports.OptionSpec {
	var specs []ports.OptionSpec
	seen := map[string]bool{}
	seenInput := map[domain.Format]bool{}
	for _, input := range inputs {
		inputFormat, err := s.detectInputFormat(input, inputOverride)
		if err != nil || seenInput[inputFormat] {
			continue
		}
		seenInput[inputFormat] = true

		options := domain.ConvertOptions{ToolOptions: s.preferences.OptionsFor(inputFormat, output).Merge(requestOptions)}
		converter, err := s.pickConverter(inputFormat, output, options)
		if err != nil {
			continue
		}
		aware, ok := converter.(ports.OptionsAware)
		if !ok {
			continue
		}
		for _, spec := range aware.OptionSpecs(inputFormat, output) {
			key := strings.ToLower(spec.Tool) + "\x00" + strings.ToLower(spec.Key)
			if seen[key] {
				continue
			}
			seen[key] = true
			specs = append(specs, spec)
		}
	}
	return specs
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
		job, err := s.buildJob(context.Background(), input, req)
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
	// inputFormats maps each detected input format to one representative
	// path, so input-aware converters (e.g. animated-svg) can be checked
	// against real file content.
	inputFormats := map[domain.Format]string{}
	for _, input := range inputs {
		inputFormat, err := s.detectInputFormat(input, inputOverride)
		if err != nil {
			return nil, err
		}
		if _, ok := inputFormats[inputFormat]; !ok {
			inputFormats[inputFormat] = input
		}

		for _, converter := range s.converters {
			for _, capability := range converterCapabilitiesForInput(converter, input, inputFormat) {
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
		seenBackend := map[string]bool{}
		for input, path := range inputFormats {
			options := domain.ConvertOptions{ToolOptions: s.preferences.OptionsFor(input, output).Merge(requestOptions)}
			for _, converter := range s.availableConverters(input, output, options) {
				if !hasPairCapability(converterCapabilitiesForInput(converter, path, input), input, output) {
					continue
				}
				choice.Available = true
				if !seenBackend[converter.ID()] {
					seenBackend[converter.ID()] = true
					choice.Backends = append(choice.Backends, converter.ID())
				}
			}
			if choice.Available {
				continue
			}
			if _, reason := s.conversionAvailable(input, output, options); reason != "" {
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

// converterCapabilitiesForInput prefers content-aware capabilities when the
// converter inspects the input file, falling back to the static matrix.
func converterCapabilitiesForInput(converter ports.Converter, path string, input domain.Format) []domain.ConversionCapability {
	if aware, ok := converter.(ports.InputCapabilityAware); ok {
		return aware.CapabilitiesForInput(path, input)
	}
	return converter.Capabilities()
}

func hasPairCapability(capabilities []domain.ConversionCapability, input domain.Format, output domain.Format) bool {
	for _, capability := range capabilities {
		if capability.Input == input && capability.Output == output {
			return true
		}
	}
	return false
}

func (s *Service) DependencyStatus() []DependencyStatus {
	reports := make([]DependencyStatus, 0, len(s.converters))
	for _, converter := range s.converters {
		status := DependencyStatus{Backend: converter.ID()}
		if described, ok := converter.(ports.Describable); ok {
			status.Description = described.Description()
		}
		for _, command := range converter.RequiredCommands() {
			_, err := s.runner.LookPath(command)
			status.Commands = append(status.Commands, CommandStatus{
				Name:  command,
				Found: err == nil,
				Hints: s.installHints([]string{command}),
			})
		}
		if aware, ok := converter.(ports.DependencyStatusAware); ok {
			for _, check := range aware.DependencyChecks() {
				commands := check.Commands
				if len(commands) == 0 {
					commands = []string{check.Name}
				}
				status.Commands = append(status.Commands, CommandStatus{
					Name:  check.Name,
					Found: check.Found,
					Hints: s.installHints(commands),
				})
			}
		}
		reports = append(reports, status)
	}
	return reports
}

func (s *Service) Converters() []ports.Converter {
	return append([]ports.Converter(nil), s.converters...)
}

func (s *Service) buildJob(ctx context.Context, input string, req ConvertRequest) (domain.ConvertJob, error) {
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

	outputPath, err = s.resolveOutputPath(ctx, input, outputPath, req.Overwrite)
	if err != nil {
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
	if cached, ok := s.formatCache[input]; ok {
		return cached, nil
	}

	format, err := s.detectInputFormatUncached(input)
	if err == nil && s.formatCache != nil {
		s.formatCache[input] = format
	}
	return format, err
}

func (s *Service) detectInputFormatUncached(input string) (domain.Format, error) {
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

func (s *Service) resolveOutputPath(ctx context.Context, input string, output string, overwrite bool) (string, error) {
	if err := s.validateOutputIdentity(input, output); err != nil {
		return "", err
	}
	if overwrite {
		return output, nil
	}

	current := output
	for {
		exists, err := s.fs.Exists(current)
		if err != nil {
			return "", err
		}
		if !exists {
			return current, nil
		}
		if s.prompt == nil {
			return "", outputExistsError(current)
		}

		next, err := s.prompt.AskOutputPath(ctx, current)
		if err != nil {
			return "", err
		}
		next = strings.TrimSpace(next)
		if next == "" {
			return "", fmt.Errorf("%w: output path is required", domain.ErrInvalidJob)
		}
		if next == current {
			return "", outputExistsError(current)
		}
		if err := s.validateOutputIdentity(input, next); err != nil {
			return "", err
		}
		current = next
	}
}

func (s *Service) validateOutputIdentity(input string, output string) error {
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
	return nil
}

func outputExistsError(output string) error {
	return fmt.Errorf("%w: output already exists: %s", domain.ErrInvalidJob, output)
}

// convertersByPriority orders converters by their declared capability
// priority for the pair (highest first), keeping registration order between
// equals. User pair preferences still override this ordering.
func (s *Service) convertersByPriority(input domain.Format, output domain.Format) []ports.Converter {
	type ranked struct {
		converter ports.Converter
		priority  int
	}
	rankedConverters := make([]ranked, 0, len(s.converters))
	for _, converter := range s.converters {
		priority := 0
		for _, capability := range converter.Capabilities() {
			if capability.Input == input && capability.Output == output && capability.Priority > priority {
				priority = capability.Priority
			}
		}
		rankedConverters = append(rankedConverters, ranked{converter: converter, priority: priority})
	}
	sort.SliceStable(rankedConverters, func(i, j int) bool {
		return rankedConverters[i].priority > rankedConverters[j].priority
	})
	ordered := make([]ports.Converter, len(rankedConverters))
	for i, item := range rankedConverters {
		ordered[i] = item.converter
	}
	return ordered
}

func (s *Service) pickConverter(input domain.Format, output domain.Format, options domain.ConvertOptions) (ports.Converter, error) {
	var capable []ports.Converter
	var missing []string
	var hints []ports.InstallSuggestion
	for _, converter := range s.preferences.OrderConverters(input, output, s.convertersByPriority(input, output)) {
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
	for _, converter := range s.preferences.OrderConverters(input, output, s.convertersByPriority(input, output)) {
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

func (s *Service) shouldAskSVGOutputSize(inputs []string, inputOverride domain.Format, output domain.Format) bool {
	if !svgOutputNeedsSize(output) {
		return false
	}
	return s.allInputsMatchFormat(inputs, inputOverride, domain.FormatSVG)
}

func (s *Service) defaultOutputSize(inputs []string, inputOverride domain.Format, fallback string) string {
	var common string
	for _, input := range inputs {
		format, err := s.detectInputFormat(input, inputOverride)
		if err != nil {
			return fallback
		}
		size, ok, err := s.fs.SourceSize(input, format)
		if err != nil || !ok || size == "" {
			return fallback
		}
		if common == "" {
			common = size
			continue
		}
		if common != size {
			return fallback
		}
	}
	if common == "" {
		return fallback
	}
	return common
}

func svgOutputNeedsSize(output domain.Format) bool {
	return output.IsImage() || output.IsVideo()
}

// supportsSameFormatAction lists the formats where a same-format run has a
// meaningful compress/resize interpretation beyond a plain copy.
func supportsSameFormatAction(format domain.Format) bool {
	return format.IsImage() || format.IsVideo() || format == domain.FormatPDF || format == domain.FormatQCOW2
}

func supportsQuality(format domain.Format) bool {
	return format.IsImage() || format.IsVideo() || format == domain.FormatPDF
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

func editableCommandLine(preview ports.CommandPreview) string {
	if len(preview.Commands) == 0 {
		return ""
	}
	index := preview.EditableCommand
	if index < 0 || index >= len(preview.Commands) {
		index = 0
	}
	return shell.Line(preview.Commands[index])
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
