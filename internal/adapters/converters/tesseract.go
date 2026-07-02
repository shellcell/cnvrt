package converters

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// Tesseract OCRs raster images into plain text or a searchable PDF.
type Tesseract struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewTesseract(runner ports.CommandRunner) *Tesseract {
	inputs := []domain.Format{
		domain.FormatPNG,
		domain.FormatJPEG,
		domain.FormatTIFF,
		domain.FormatBMP,
		domain.FormatWebP,
		domain.FormatGIF,
	}
	outputs := []domain.Format{domain.FormatTXT, domain.FormatPDF}
	return &Tesseract{runner: runner, caps: capabilities(inputs, outputs, 70, false, false)}
}

func (c *Tesseract) ID() string { return "tesseract" }

func (c *Tesseract) Description() string {
	return "OCR: extract text from images as txt or searchable pdf"
}

func (c *Tesseract) RequiredCommands() []string { return []string{"tesseract"} }

func (c *Tesseract) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Tesseract) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Tesseract) OptionSpecs(input domain.Format, output domain.Format) []ports.OptionSpec {
	return []ports.OptionSpec{{
		Tool:        "tesseract",
		Key:         "lang",
		Title:       "OCR language",
		Description: "Tesseract language code(s), for example eng, deu, or eng+rus. Requires the matching language pack.",
		Default:     "eng",
	}}
}

func (c *Tesseract) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "tesseract", c.args(job), job, c.ID())
}

func (c *Tesseract) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("tesseract", c.args(job))
}

func (c *Tesseract) args(job domain.ConvertJob) []string {
	// Tesseract appends .txt/.pdf itself, so pass the output base name.
	outputBase := strings.TrimSuffix(job.OutputPath, filepath.Ext(job.OutputPath))
	args := []string{job.InputPath, outputBase}
	if lang := stringOption(job.Options.ToolOptions, "tesseract", "lang", ""); lang != "" {
		args = append(args, "-l", lang)
	}
	args = append(args, extraArgs(job.Options.ToolOptions, "tesseract")...)
	if job.OutputFormat == domain.FormatPDF {
		args = append(args, "pdf")
	}
	return args
}
