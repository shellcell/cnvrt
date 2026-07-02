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
	inputs := []domain.Format{
		domain.FormatEPUB,
		domain.FormatMOBI,
		domain.FormatAZW3,
		domain.FormatFB2,
		domain.FormatHTML,
		domain.FormatTXT,
		domain.FormatRTF,
		domain.FormatDOCX,
		domain.FormatPDF,
	}
	outputs := []domain.Format{
		domain.FormatEPUB,
		domain.FormatMOBI,
		domain.FormatAZW3,
		domain.FormatFB2,
		domain.FormatHTML,
		domain.FormatTXT,
		domain.FormatRTF,
		domain.FormatDOCX,
		domain.FormatPDF,
	}
	return &Calibre{runner: runner, caps: capabilities(inputs, outputs, 75, false, false)}
}

func (c *Calibre) ID() string { return "calibre" }

func (c *Calibre) RequiredCommands() []string { return []string{"ebook-convert"} }

func (c *Calibre) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Calibre) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Calibre) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "ebook-convert", []string{job.InputPath, job.OutputPath}, job, c.ID())
}
