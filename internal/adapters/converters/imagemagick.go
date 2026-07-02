package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type ImageMagick struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewImageMagick(runner ports.CommandRunner) *ImageMagick {
	inputs := []domain.Format{
		domain.FormatPNG,
		domain.FormatJPEG,
		domain.FormatWebP,
		domain.FormatBMP,
		domain.FormatTIFF,
		domain.FormatGIF,
		domain.FormatAPNG,
		domain.FormatAVIF,
		domain.FormatHEIC,
		domain.FormatICO,
		domain.FormatICNS,
		domain.FormatPSD,
		domain.FormatJP2,
		domain.FormatSVG,
		domain.FormatPDF,
	}
	outputs := []domain.Format{
		domain.FormatPNG,
		domain.FormatJPEG,
		domain.FormatWebP,
		domain.FormatBMP,
		domain.FormatTIFF,
		domain.FormatGIF,
		domain.FormatAVIF,
		domain.FormatHEIC,
		domain.FormatICO,
		domain.FormatJP2,
		domain.FormatPDF,
	}
	return &ImageMagick{runner: runner, caps: capabilities(inputs, outputs, 50, true, false)}
}

func (c *ImageMagick) ID() string { return "imagemagick" }

func (c *ImageMagick) RequiredCommands() []string { return []string{"magick"} }

func (c *ImageMagick) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *ImageMagick) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *ImageMagick) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	args := []string{job.InputPath}
	if job.Options.Resize != "" {
		args = append(args, "-resize", job.Options.Resize)
	}
	args = append(args, qualityArgs(job)...)
	args = append(args, job.OutputPath)
	return runSimple(ctx, c.runner, "magick", args, job, c.ID())
}
