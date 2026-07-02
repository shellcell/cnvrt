package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type Resvg struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewResvg(runner ports.CommandRunner) *Resvg {
	return &Resvg{
		runner: runner,
		caps: []domain.ConversionCapability{{
			Input:    domain.FormatSVG,
			Output:   domain.FormatPNG,
			Priority: 100,
		}},
	}
}

func (c *Resvg) ID() string { return "resvg" }

func (c *Resvg) Description() string {
	return "fast, faithful svg -> png rendering"
}

func (c *Resvg) RequiredCommands() []string { return []string{"resvg"} }

func (c *Resvg) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Resvg) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Resvg) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "resvg", c.args(job), job, c.ID())
}

func (c *Resvg) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("resvg", c.args(job))
}

func (c *Resvg) args(job domain.ConvertJob) []string {
	args := []string{}
	if job.Options.Resize != "" {
		width, height := resizeDimensions(job.Options.Resize)
		if width != "" {
			args = append(args, "--width", width)
		}
		if height != "" {
			args = append(args, "--height", height)
		}
	}
	args = append(args, extraArgs(job.Options.ToolOptions, "resvg")...)
	args = append(args, job.InputPath, job.OutputPath)
	return args
}
