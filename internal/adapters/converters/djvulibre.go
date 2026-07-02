package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// DjVuLibre gives the djvu format (previously recognized but unconvertible)
// real outputs: pdf/tiff via ddjvu and plain text via djvutxt.
type DjVuLibre struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewDjVuLibre(runner ports.CommandRunner) *DjVuLibre {
	return &DjVuLibre{
		runner: runner,
		caps: []domain.ConversionCapability{
			{Input: domain.FormatDJVU, Output: domain.FormatPDF, Priority: 90},
			{Input: domain.FormatDJVU, Output: domain.FormatTIFF, Priority: 90},
			{Input: domain.FormatDJVU, Output: domain.FormatTXT, Priority: 90},
		},
	}
}

func (c *DjVuLibre) ID() string { return "djvulibre" }

func (c *DjVuLibre) Description() string {
	return "djvu -> pdf, tiff, and plain text"
}

func (c *DjVuLibre) RequiredCommands() []string { return []string{"ddjvu"} }

func (c *DjVuLibre) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *DjVuLibre) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *DjVuLibre) MissingDependencies(input domain.Format, output domain.Format, options domain.ConvertOptions) []string {
	if output != domain.FormatTXT || !c.CanConvert(input, output) {
		return nil
	}
	if _, err := c.runner.LookPath("djvutxt"); err != nil {
		return []string{"djvutxt"}
	}
	return nil
}

func (c *DjVuLibre) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	command, args := c.command(job)
	return runSimple(ctx, c.runner, command, args, job, c.ID())
}

func (c *DjVuLibre) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	command, args := c.command(job)
	return previewCommand(command, args)
}

func (c *DjVuLibre) command(job domain.ConvertJob) (string, []string) {
	options := job.Options.ToolOptions
	if job.OutputFormat == domain.FormatTXT {
		args := extraArgs(options, "djvulibre")
		return "djvutxt", append(args, job.InputPath, job.OutputPath)
	}

	args := []string{"-format=" + job.OutputFormat.Extension()}
	args = append(args, extraArgs(options, "djvulibre")...)
	return "ddjvu", append(args, job.InputPath, job.OutputPath)
}
