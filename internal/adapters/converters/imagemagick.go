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
	legacy := []domain.Format{
		domain.FormatXPM,
		domain.FormatXBM,
		domain.FormatPNM,
		domain.FormatPPM,
		domain.FormatPGM,
		domain.FormatPBM,
		domain.FormatTGA,
		domain.FormatDDS,
		domain.FormatEXR,
		domain.FormatQOI,
	}
	inputs := append([]domain.Format{
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
	}, legacy...)
	outputs := append([]domain.Format{
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
	}, legacy...)
	return &ImageMagick{runner: runner, caps: capabilities(inputs, outputs, 50, true, false)}
}

func (c *ImageMagick) ID() string { return "imagemagick" }

func (c *ImageMagick) Description() string {
	return "general image conversion, resize, and compression, including legacy formats (xpm, pnm, tga, dds, exr, qoi)"
}

func (c *ImageMagick) OptionSpecs(input domain.Format, output domain.Format) []ports.OptionSpec {
	return []ports.OptionSpec{{
		Tool:        "imagemagick",
		Key:         "background",
		Title:       "Background",
		Description: "Background for transparency: transparent, white, black, or #rrggbb. Empty keeps the source background.",
		Default:     "",
	}}
}

func (c *ImageMagick) RequiredCommands() []string { return []string{"magick"} }

func (c *ImageMagick) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *ImageMagick) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *ImageMagick) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "magick", c.args(job), job, c.ID())
}

func (c *ImageMagick) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("magick", c.args(job))
}

func (c *ImageMagick) args(job domain.ConvertJob) []string {
	args := []string{}
	if background := stringOption(job.Options.ToolOptions, "imagemagick", "background", ""); background != "" {
		if background == "transparent" {
			background = "none"
		}
		args = append(args, "-background", background)
		args = append(args, job.InputPath, "-flatten")
	} else {
		args = append(args, job.InputPath)
	}
	if job.Options.Resize != "" {
		args = append(args, "-resize", job.Options.Resize)
	}
	if job.Options.Action == domain.ActionCompress {
		// Compression without stripping metadata rarely shrinks anything.
		args = append(args, "-strip")
		if job.Options.Quality <= 0 {
			args = append(args, "-quality", "85")
		}
	}
	args = append(args, qualityArgs(job)...)
	args = append(args, extraArgs(job.Options.ToolOptions, "imagemagick")...)
	args = append(args, job.OutputPath)
	return args
}
