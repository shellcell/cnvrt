package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// Poppler extracts the text layer of PDFs via pdftotext. This is instant
// for digital PDFs; scanned PDFs without a text layer need OCR instead.
type Poppler struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewPoppler(runner ports.CommandRunner) *Poppler {
	return &Poppler{
		runner: runner,
		caps: []domain.ConversionCapability{
			{Input: domain.FormatPDF, Output: domain.FormatTXT, Priority: 90},
			{Input: domain.FormatPDF, Output: domain.FormatHTML, Priority: 80},
		},
	}
}

func (c *Poppler) ID() string { return "poppler" }

func (c *Poppler) Description() string {
	return "extract the text layer of digital PDFs as txt or html"
}

func (c *Poppler) RequiredCommands() []string { return []string{"pdftotext"} }

func (c *Poppler) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Poppler) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Poppler) MissingDependencies(input domain.Format, output domain.Format, options domain.ConvertOptions) []string {
	if output != domain.FormatHTML || !c.CanConvert(input, output) {
		return nil
	}
	if _, err := c.runner.LookPath("pdftohtml"); err != nil {
		return []string{"pdftohtml"}
	}
	return nil
}

func (c *Poppler) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	command, args := c.command(job)
	return runSimple(ctx, c.runner, command, args, job, c.ID())
}

func (c *Poppler) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	command, args := c.command(job)
	return previewCommand(command, args)
}

func (c *Poppler) command(job domain.ConvertJob) (string, []string) {
	options := job.Options.ToolOptions
	if job.OutputFormat == domain.FormatHTML {
		args := []string{"-s", "-noframes"}
		args = append(args, extraArgs(options, "poppler")...)
		return "pdftohtml", append(args, job.InputPath, job.OutputPath)
	}

	args := []string{"-layout"}
	args = append(args, extraArgs(options, "poppler")...)
	return "pdftotext", append(args, job.InputPath, job.OutputPath)
}
