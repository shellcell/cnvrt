package converters

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// Jupyter exports notebooks through `jupyter nbconvert`: HTML and Markdown
// reports, or the plain Python script embedded in the notebook.
type Jupyter struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewJupyter(runner ports.CommandRunner) *Jupyter {
	return &Jupyter{
		runner: runner,
		caps: []domain.ConversionCapability{
			{Input: domain.FormatIPYNB, Output: domain.FormatHTML, Priority: 90},
			{Input: domain.FormatIPYNB, Output: domain.FormatMD, Priority: 90},
			{Input: domain.FormatIPYNB, Output: domain.FormatPY, Priority: 90},
			{Input: domain.FormatIPYNB, Output: domain.FormatPDF, Priority: 90},
		},
	}
}

func (c *Jupyter) ID() string { return "jupyter" }

func (c *Jupyter) Description() string {
	return "notebooks -> html, markdown, python script, pdf"
}

func (c *Jupyter) RequiredCommands() []string { return []string{"jupyter"} }

func (c *Jupyter) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Jupyter) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Jupyter) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "jupyter", c.args(job), job, c.ID())
}

func (c *Jupyter) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("jupyter", c.args(job))
}

func (c *Jupyter) args(job domain.ConvertJob) []string {
	base := filepath.Base(job.OutputPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	args := []string{
		"nbconvert",
		"--to", nbconvertTarget(job.OutputFormat),
		"--output", base,
		"--output-dir", filepath.Dir(job.OutputPath),
	}
	args = append(args, extraArgs(job.Options.ToolOptions, "jupyter")...)
	return append(args, job.InputPath)
}

func nbconvertTarget(format domain.Format) string {
	switch format {
	case domain.FormatMD:
		return "markdown"
	case domain.FormatPY:
		return "script"
	default:
		return format.Extension()
	}
}
