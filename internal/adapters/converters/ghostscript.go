package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// Ghostscript covers the PDF/PostScript niches the other backends miss:
// PostScript/EPS to PDF, and same-format PDF compression.
type Ghostscript struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewGhostscript(runner ports.CommandRunner) *Ghostscript {
	return &Ghostscript{
		runner: runner,
		caps: []domain.ConversionCapability{
			{Input: domain.FormatPS, Output: domain.FormatPDF, Priority: 90},
			{Input: domain.FormatEPS, Output: domain.FormatPDF, Priority: 90},
			{Input: domain.FormatPDF, Output: domain.FormatPDF, Priority: 90, Lossy: true},
			{Input: domain.FormatPDF, Output: domain.FormatPS, Priority: 90},
		},
	}
}

func (c *Ghostscript) ID() string { return "ghostscript" }

func (c *Ghostscript) Description() string {
	return "ps/eps -> pdf, pdf -> ps, and pdf re-compression"
}

func (c *Ghostscript) RequiredCommands() []string { return []string{"gs"} }

func (c *Ghostscript) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Ghostscript) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Ghostscript) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "gs", c.args(job), job, c.ID())
}

func (c *Ghostscript) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("gs", c.args(job))
}

func (c *Ghostscript) args(job domain.ConvertJob) []string {
	device := "pdfwrite"
	if job.OutputFormat == domain.FormatPS {
		device = "ps2write"
	}
	args := []string{"-dNOPAUSE", "-dBATCH", "-dQUIET", "-sDEVICE=" + device}
	if job.InputFormat == domain.FormatPDF && job.OutputFormat == domain.FormatPDF {
		args = append(args, "-dPDFSETTINGS="+pdfSettings(job.Options.Quality))
	}
	args = append(args, extraArgs(job.Options.ToolOptions, "ghostscript")...)
	return append(args, "-sOutputFile="+job.OutputPath, job.InputPath)
}

// pdfSettings maps the generic 1-100 quality value onto Ghostscript's
// distiller presets used for PDF re-compression.
func pdfSettings(quality int) string {
	switch {
	case quality > 0 && quality <= 50:
		return "/screen"
	case quality > 85:
		return "/printer"
	default:
		return "/ebook"
	}
}
