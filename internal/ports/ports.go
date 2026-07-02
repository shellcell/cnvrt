package ports

import (
	"context"
	"errors"
	"io"

	"github.com/shellcell/convert/internal/domain"
)

var ErrUserAborted = errors.New("user aborted")

type Converter interface {
	ID() string
	RequiredCommands() []string
	Capabilities() []domain.ConversionCapability
	CanConvert(input domain.Format, output domain.Format) bool
	Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error)
}

type RuntimeDependencyAware interface {
	MissingDependencies(input domain.Format, output domain.Format, options domain.ConvertOptions) []string
}

type InputCapabilityAware interface {
	CapabilitiesForInput(path string, input domain.Format) []domain.ConversionCapability
}

type DependencyCheck struct {
	Name     string
	Found    bool
	Commands []string
}

type DependencyStatusAware interface {
	DependencyChecks() []DependencyCheck
}

type FormatChoice struct {
	Format    domain.Format
	Available bool
	Reason    string
}

type OutputLocation string

const (
	OutputLocationCurrent OutputLocation = "current"
	OutputLocationSource  OutputLocation = "source"
)

type FileDiscovery interface {
	ListFiles(ctx context.Context, root string) ([]domain.FileRef, error)
}

type FileSystem interface {
	CurrentDir() (string, error)
	Abs(path string) (string, error)
	Exists(path string) (bool, error)
	IsDir(path string) (bool, error)
	IsTextFile(path string) (bool, error)
	SourceSize(path string, format domain.Format) (string, bool, error)
	EnsureDir(path string) error
}

type Prompt interface {
	SelectFiles(ctx context.Context, files []domain.FileRef) ([]domain.FileRef, error)
	SelectFormat(ctx context.Context, choices []FormatChoice) (domain.Format, error)
	SelectOutputLocation(ctx context.Context, currentDir string) (OutputLocation, error)
	SelectArchiveAction(ctx context.Context, file domain.FileRef) (domain.ArchiveAction, error)
	SelectSameFormatAction(ctx context.Context, format domain.Format) (domain.TransformAction, error)
	ConfirmOption(ctx context.Context, title string, description string, defaultValue bool) (bool, error)
	AskOutputSize(ctx context.Context, defaultSize string) (string, error)
	AskResize(ctx context.Context) (string, error)
	AskQuality(ctx context.Context, defaultQuality int) (int, error)
}

type Command struct {
	Name string
	Args []string
	Dir  string
}

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type CommandRunner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, command Command) (CommandResult, error)
}

type InstallSuggestion struct {
	Manager string
	Package string
	Command string
}

type InstallAdvisor interface {
	Suggestions(command string) []InstallSuggestion
}

type ProgressReporter interface {
	Start(total int)
	JobStart(index int, total int, job domain.ConvertJob, backend string)
	JobSuccess(index int, total int, job domain.ConvertJob, backend string)
	JobSkipped(index int, total int, input string, output string, message string)
	JobFailed(index int, total int, job domain.ConvertJob, backend string, err error)
	Finish()
}

type NoopProgressReporter struct{}

func (NoopProgressReporter) Start(total int)                                                        {}
func (NoopProgressReporter) JobStart(index int, total int, job domain.ConvertJob, backend string)   {}
func (NoopProgressReporter) JobSuccess(index int, total int, job domain.ConvertJob, backend string) {}
func (NoopProgressReporter) JobSkipped(index int, total int, input string, output string, message string) {
}
func (NoopProgressReporter) JobFailed(index int, total int, job domain.ConvertJob, backend string, err error) {
}
func (NoopProgressReporter) Finish() {}

type AppRunner interface {
	Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int
}
