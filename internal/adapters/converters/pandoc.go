package converters

import (
	"context"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type Pandoc struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewPandoc(runner ports.CommandRunner) *Pandoc {
	inputs := []domain.Format{
		domain.FormatMD,
		domain.FormatTXT,
		domain.FormatHTML,
		domain.FormatDOCX,
		domain.FormatRTF,
		domain.FormatTEX,
		domain.FormatODT,
		domain.FormatEPUB,
		domain.FormatFB2,
		domain.FormatRST,
		domain.FormatORG,
	}
	outputs := []domain.Format{
		domain.FormatPDF,
		domain.FormatHTML,
		domain.FormatDOCX,
		domain.FormatTXT,
		domain.FormatMD,
		domain.FormatRTF,
		domain.FormatTEX,
		domain.FormatODT,
		domain.FormatEPUB,
		domain.FormatRST,
		domain.FormatORG,
		domain.FormatPPTX,
	}
	return &Pandoc{runner: runner, caps: capabilities(inputs, outputs, 70, false, false)}
}

func (c *Pandoc) ID() string { return "pandoc" }

func (c *Pandoc) Description() string {
	return "text documents: markdown, html, docx, epub, tex, rst, org, pdf"
}

func (c *Pandoc) RequiredCommands() []string { return []string{"pandoc"} }

func (c *Pandoc) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Pandoc) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Pandoc) MissingDependencies(input domain.Format, output domain.Format, options domain.ConvertOptions) []string {
	if output != domain.FormatPDF || !c.CanConvert(input, output) {
		return nil
	}
	engines := pdfEngines(options)
	if _, ok := c.availablePDFEngine(engines); ok {
		return nil
	}
	return engines
}

func (c *Pandoc) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	args, err := c.args(job)
	if err != nil {
		return domain.ConversionResult{}, err
	}
	return runSimple(ctx, c.runner, "pandoc", args, job, c.ID())
}

func (c *Pandoc) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	args, err := c.args(job)
	if err != nil {
		return ports.CommandPreview{}
	}
	return previewCommand("pandoc", args)
}

func (c *Pandoc) args(job domain.ConvertJob) ([]string, error) {
	args := []string{job.InputPath}
	if job.OutputFormat == domain.FormatPDF {
		engines := pdfEngines(job.Options)
		engine, ok := c.availablePDFEngine(engines)
		if !ok {
			return nil, domain.MissingDependencyError{
				Message:  "pandoc PDF output requires a PDF engine: install one of " + joinList(engines),
				Commands: engines,
			}
		}
		args = append(args, "--pdf-engine", engine)
	}
	if job.OutputFormat == domain.FormatHTML {
		// Without --standalone pandoc emits a body fragment, which most
		// callers converting a file to .html do not expect.
		args = append(args, "--standalone")
	}
	args = append(args, extraArgs(job.Options.ToolOptions, "pandoc")...)
	args = append(args, "-o", job.OutputPath)
	return args, nil
}

func pdfEngines(options domain.ConvertOptions) []string {
	engines := options.ToolOptions.Values("pandoc", "pdf_engines")
	if len(engines) == 0 {
		engines = options.ToolOptions.Values("pandoc", "pdf_engine")
	}
	if len(engines) == 0 {
		engines = []string{"tectonic", "typst", "pdflatex"}
	}
	return engines
}

func (c *Pandoc) availablePDFEngine(engines []string) (string, bool) {
	for _, engine := range engines {
		if _, err := c.runner.LookPath(engine); err == nil {
			return engine, true
		}
	}
	return "", false
}

func joinList(values []string) string {
	return strings.Join(values, ", ")
}
