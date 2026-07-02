package app

import (
	"context"
	"testing"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

func TestConvertBatchConfirmsOnce(t *testing.T) {
	created := map[string]bool{}
	converter := &fakeExternalConverter{created: created}
	prompt := &fakePrompt{confirmAction: ports.CommandReviewProceed}
	service := NewService([]ports.Converter{converter}, nil, fakeFS{created: created}, prompt, nil, nil, Preferences{}, nil)

	report, err := service.Convert(context.Background(), ConvertRequest{
		Inputs:       []string{"a.svg", "b.svg", "c.svg"},
		OutputFormat: domain.FormatPNG,
	})
	if err != nil {
		t.Fatal(err)
	}

	if prompt.confirmCalls != 1 {
		t.Fatalf("expected one confirmation for the batch, got %d", prompt.confirmCalls)
	}
	if prompt.lastReview.JobCount != 3 {
		t.Fatalf("expected review to cover 3 jobs, got %d", prompt.lastReview.JobCount)
	}
	if converter.calls != 3 {
		t.Fatalf("expected 3 conversions, got %d", converter.calls)
	}
	if report.Count(StatusConverted) != 3 {
		t.Fatalf("expected 3 converted, got %d", report.Count(StatusConverted))
	}
}

func TestConvertBatchCancelSkipsAllJobs(t *testing.T) {
	created := map[string]bool{}
	converter := &fakeExternalConverter{created: created}
	prompt := &fakePrompt{confirmAction: ports.CommandReviewCancel}
	service := NewService([]ports.Converter{converter}, nil, fakeFS{created: created}, prompt, nil, nil, Preferences{}, nil)

	report, err := service.Convert(context.Background(), ConvertRequest{
		Inputs:       []string{"a.svg", "b.svg"},
		OutputFormat: domain.FormatPNG,
	})
	if err != nil {
		t.Fatal(err)
	}

	if converter.calls != 0 {
		t.Fatalf("expected no conversions after cancel, got %d", converter.calls)
	}
	if report.Count(StatusSkipped) != 2 {
		t.Fatalf("expected 2 skipped, got %d", report.Count(StatusSkipped))
	}
}

func TestConvertInProcessBackendSkipsConfirmation(t *testing.T) {
	created := map[string]bool{}
	converter := &fakeInternalConverter{created: created}
	prompt := &fakePrompt{confirmAction: ports.CommandReviewCancel}
	service := NewService([]ports.Converter{converter}, nil, fakeFS{created: created}, prompt, nil, nil, Preferences{}, nil)

	report, err := service.Convert(context.Background(), ConvertRequest{
		Inputs:       []string{"a.json"},
		OutputFormat: domain.FormatYAML,
	})
	if err != nil {
		t.Fatal(err)
	}

	if prompt.confirmCalls != 0 {
		t.Fatalf("expected no confirmation for in-process backend, got %d", prompt.confirmCalls)
	}
	if report.Count(StatusConverted) != 1 {
		t.Fatalf("expected 1 converted, got %d", report.Count(StatusConverted))
	}
}

type fakeFS struct {
	created map[string]bool
}

func (fs fakeFS) CurrentDir() (string, error)                            { return ".", nil }
func (fs fakeFS) Abs(path string) (string, error)                        { return "/" + path, nil }
func (fs fakeFS) Exists(path string) (bool, error)                       { return fs.created[path], nil }
func (fs fakeFS) IsDir(path string) (bool, error)                        { return false, nil }
func (fs fakeFS) IsTextFile(path string) (bool, error)                   { return false, nil }
func (fs fakeFS) SourceSize(string, domain.Format) (string, bool, error) { return "", false, nil }
func (fs fakeFS) EnsureDir(string) error                                 { return nil }

type fakeExternalConverter struct {
	calls   int
	created map[string]bool
}

func (c *fakeExternalConverter) ID() string                 { return "fake" }
func (c *fakeExternalConverter) RequiredCommands() []string { return nil }
func (c *fakeExternalConverter) Capabilities() []domain.ConversionCapability {
	return []domain.ConversionCapability{{Input: domain.FormatSVG, Output: domain.FormatPNG}}
}
func (c *fakeExternalConverter) CanConvert(input domain.Format, output domain.Format) bool {
	return input == domain.FormatSVG && output == domain.FormatPNG
}
func (c *fakeExternalConverter) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	c.calls++
	if c.created != nil {
		c.created[job.OutputPath] = true
	}
	return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
}
func (c *fakeExternalConverter) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return ports.CommandPreview{Commands: []ports.Command{{Name: "fake", Args: []string{job.InputPath, job.OutputPath}}}, Editable: true}
}

type fakeInternalConverter struct {
	created map[string]bool
}

func (c *fakeInternalConverter) ID() string                 { return "internal" }
func (c *fakeInternalConverter) RequiredCommands() []string { return nil }
func (c *fakeInternalConverter) Capabilities() []domain.ConversionCapability {
	return []domain.ConversionCapability{{Input: domain.FormatJSON, Output: domain.FormatYAML}}
}
func (c *fakeInternalConverter) CanConvert(input domain.Format, output domain.Format) bool {
	return input == domain.FormatJSON && output == domain.FormatYAML
}
func (c *fakeInternalConverter) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	if c.created != nil {
		c.created[job.OutputPath] = true
	}
	return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
}

type fakePrompt struct {
	confirmCalls  int
	confirmAction ports.CommandReviewAction
	lastReview    ports.CommandReview
}

func (p *fakePrompt) SelectFiles(ctx context.Context, files []domain.FileRef) ([]domain.FileRef, error) {
	return files, nil
}
func (p *fakePrompt) SelectFormat(ctx context.Context, choices []ports.FormatChoice) (domain.Format, error) {
	return choices[0].Format, nil
}
func (p *fakePrompt) SelectOutputLocation(ctx context.Context, currentDir string) (ports.OutputLocation, error) {
	return ports.OutputLocationCurrent, nil
}
func (p *fakePrompt) SelectArchiveAction(ctx context.Context, file domain.FileRef) (domain.ArchiveAction, error) {
	return domain.ArchiveActionCancel, nil
}
func (p *fakePrompt) SelectSameFormatAction(ctx context.Context, format domain.Format) (domain.TransformAction, error) {
	return domain.ActionConvert, nil
}
func (p *fakePrompt) SelectBackend(ctx context.Context, input domain.Format, output domain.Format, choices []ports.BackendChoice) (string, error) {
	return choices[0].ID, nil
}
func (p *fakePrompt) ConfirmOption(ctx context.Context, title string, description string, defaultValue bool) (bool, error) {
	return defaultValue, nil
}
func (p *fakePrompt) ConfirmCommand(ctx context.Context, review ports.CommandReview) (ports.CommandReviewAction, string, error) {
	p.confirmCalls++
	p.lastReview = review
	return p.confirmAction, "", nil
}
func (p *fakePrompt) AskOutputPath(ctx context.Context, currentPath string) (string, error) {
	return currentPath, nil
}
func (p *fakePrompt) AskOutputSize(ctx context.Context, defaultSize string) (string, error) {
	return defaultSize, nil
}
func (p *fakePrompt) AskText(ctx context.Context, title string, description string, defaultValue string) (string, error) {
	return defaultValue, nil
}
func (p *fakePrompt) AskResize(ctx context.Context) (string, error) { return "", nil }
func (p *fakePrompt) AskQuality(ctx context.Context, defaultQuality int) (int, error) {
	return defaultQuality, nil
}
