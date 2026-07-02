package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// Mermaid renders Mermaid diagram files through the mermaid-cli (mmdc).
type Mermaid struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewMermaid(runner ports.CommandRunner) *Mermaid {
	outputs := []domain.Format{
		domain.FormatSVG,
		domain.FormatPNG,
		domain.FormatPDF,
	}
	return &Mermaid{runner: runner, caps: capabilities([]domain.Format{domain.FormatMermaid}, outputs, 90, false, false)}
}

func (c *Mermaid) ID() string { return "mermaid" }

func (c *Mermaid) Description() string {
	return "mermaid diagrams -> svg, png, pdf"
}

func (c *Mermaid) RequiredCommands() []string { return []string{"mmdc"} }

func (c *Mermaid) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Mermaid) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Mermaid) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "mmdc", c.args(job), job, c.ID())
}

func (c *Mermaid) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("mmdc", c.args(job))
}

func (c *Mermaid) args(job domain.ConvertJob) []string {
	args := []string{"-i", job.InputPath, "-o", job.OutputPath}
	return append(args, extraArgs(job.Options.ToolOptions, "mermaid")...)
}
