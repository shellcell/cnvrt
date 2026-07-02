package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type Calibre struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewCalibre(runner ports.CommandRunner) *Calibre {
	ebookInputs := []domain.Format{
		domain.FormatEPUB,
		domain.FormatMOBI,
		domain.FormatAZW3,
		domain.FormatFB2,
	}
	ebookOutputs := []domain.Format{
		domain.FormatEPUB,
		domain.FormatMOBI,
		domain.FormatAZW3,
		domain.FormatFB2,
	}
	documentInputs := []domain.Format{
		domain.FormatHTML,
		domain.FormatTXT,
		domain.FormatRTF,
		domain.FormatDOCX,
		domain.FormatPDF,
	}
	readableOutputs := []domain.Format{
		domain.FormatTXT,
		domain.FormatRTF,
		domain.FormatDOCX,
		domain.FormatPDF,
	}

	caps := capabilities(ebookInputs, append(append([]domain.Format{}, ebookOutputs...), readableOutputs...), 75, false, false)
	caps = append(caps, capabilities(documentInputs, ebookOutputs, 75, false, false)...)
	return &Calibre{runner: runner, caps: caps}
}

func (c *Calibre) ID() string { return "calibre" }

func (c *Calibre) Description() string {
	return "e-book conversions: epub, mobi, azw3, fb2, and readable outputs"
}

func (c *Calibre) RequiredCommands() []string { return []string{"ebook-convert"} }

func (c *Calibre) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Calibre) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Calibre) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "ebook-convert", c.args(job), job, c.ID())
}

func (c *Calibre) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("ebook-convert", c.args(job))
}

func (c *Calibre) args(job domain.ConvertJob) []string {
	args := []string{job.InputPath, job.OutputPath}
	return append(args, extraArgs(job.Options.ToolOptions, "calibre")...)
}
