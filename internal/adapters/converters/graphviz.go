package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// Graphviz renders DOT graph descriptions, a staple of developer tooling.
type Graphviz struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewGraphviz(runner ports.CommandRunner) *Graphviz {
	outputs := []domain.Format{
		domain.FormatSVG,
		domain.FormatPNG,
		domain.FormatPDF,
	}
	return &Graphviz{runner: runner, caps: capabilities([]domain.Format{domain.FormatDOT}, outputs, 90, false, false)}
}

func (c *Graphviz) ID() string { return "graphviz" }

func (c *Graphviz) Description() string {
	return "dot graph files -> svg, png, pdf"
}

func (c *Graphviz) RequiredCommands() []string { return []string{"dot"} }

func (c *Graphviz) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Graphviz) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Graphviz) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "dot", c.args(job), job, c.ID())
}

func (c *Graphviz) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("dot", c.args(job))
}

func (c *Graphviz) args(job domain.ConvertJob) []string {
	args := []string{"-T" + job.OutputFormat.Extension()}
	args = append(args, extraArgs(job.Options.ToolOptions, "graphviz")...)
	return append(args, "-o", job.OutputPath, job.InputPath)
}
