package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// Inkscape renders SVG to print-oriented formats. It is the only built-in
// backend with a faithful svg -> pdf/eps/ps path; for svg -> png it ranks
// below resvg and acts as a fallback.
type Inkscape struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewInkscape(runner ports.CommandRunner) *Inkscape {
	return &Inkscape{
		runner: runner,
		caps: []domain.ConversionCapability{
			{Input: domain.FormatSVG, Output: domain.FormatPDF, Priority: 90},
			{Input: domain.FormatSVG, Output: domain.FormatEPS, Priority: 90},
			{Input: domain.FormatSVG, Output: domain.FormatPS, Priority: 90},
			{Input: domain.FormatSVG, Output: domain.FormatPNG, Priority: 60},
		},
	}
}

func (c *Inkscape) ID() string { return "inkscape" }

func (c *Inkscape) Description() string {
	return "svg -> pdf/eps/ps print rendering and png fallback"
}

func (c *Inkscape) RequiredCommands() []string { return []string{"inkscape"} }

func (c *Inkscape) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Inkscape) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Inkscape) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "inkscape", c.args(job), job, c.ID())
}

func (c *Inkscape) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("inkscape", c.args(job))
}

func (c *Inkscape) args(job domain.ConvertJob) []string {
	args := []string{
		"--export-type=" + job.OutputFormat.Extension(),
		"--export-filename=" + job.OutputPath,
	}
	if job.Options.Resize != "" {
		width, height := resizeDimensions(job.Options.Resize)
		if width != "" {
			args = append(args, "--export-width="+width)
		}
		if height != "" {
			args = append(args, "--export-height="+height)
		}
	}
	args = append(args, extraArgs(job.Options.ToolOptions, "inkscape")...)
	return append(args, job.InputPath)
}
