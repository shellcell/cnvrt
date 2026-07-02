package converters

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type FontForge struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewFontForge(runner ports.CommandRunner) *FontForge {
	formats := []domain.Format{
		domain.FormatTTF,
		domain.FormatOTF,
		domain.FormatWOFF,
		domain.FormatWOFF2,
		domain.FormatEOT,
		domain.FormatBDF,
		domain.FormatPCF,
		domain.FormatFON,
		domain.FormatPFA,
		domain.FormatPFB,
	}
	return &FontForge{runner: runner, caps: capabilities(formats, formats, 80, false, false)}
}

func (c *FontForge) ID() string { return "fontforge" }

func (c *FontForge) Description() string {
	return "outline and bitmap font conversions (ttf, otf, woff, woff2, bdf, pcf, fon)"
}

func (c *FontForge) RequiredCommands() []string { return []string{"fontforge"} }

func (c *FontForge) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *FontForge) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *FontForge) OptionSpecs(input domain.Format, output domain.Format) []ports.OptionSpec {
	if !bitmapFontFormat(output) {
		return nil
	}
	return []ports.OptionSpec{{
		Tool:        "fontforge",
		Key:         "pixel_size",
		Title:       "Bitmap pixel size",
		Description: "Pixel size of the generated bitmap strike, for example 12, 16, or 24.",
		Default:     strconv.Itoa(defaultBitmapPixelSize),
	}}
}

func (c *FontForge) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	result, err := runSimple(ctx, c.runner, "fontforge", c.args(job), job, c.ID())
	if err != nil {
		return result, err
	}
	if err := c.collectBitmapOutput(job); err != nil {
		return domain.ConversionResult{}, err
	}
	return result, nil
}

// collectBitmapOutput renames FontForge's strike-suffixed bitmap output
// (name-16.bdf) to the requested path when the exact path was not written.
func (c *FontForge) collectBitmapOutput(job domain.ConvertJob) error {
	if !bitmapFontFormat(job.OutputFormat) {
		return nil
	}
	if _, err := os.Stat(job.OutputPath); err == nil {
		return nil
	}

	ext := filepath.Ext(job.OutputPath)
	pattern := strings.TrimSuffix(job.OutputPath, ext) + "-*" + ext
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	return moveFile(matches[0], job.OutputPath)
}

func (c *FontForge) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("fontforge", c.args(job))
}

const defaultBitmapPixelSize = 16

func (c *FontForge) args(job domain.ConvertJob) []string {
	script := `Open($1); Generate($2); Close();`
	if bitmapFontFormat(job.OutputFormat) && !bitmapFontFormat(job.InputFormat) {
		// Outline fonts have no bitmap strikes; Generate() for a bitmap
		// target silently writes nothing without BitmapsAvail first.
		size := intOption(job.Options.ToolOptions, "fontforge", "pixel_size", defaultBitmapPixelSize)
		if size <= 0 {
			size = defaultBitmapPixelSize
		}
		script = `Open($1); BitmapsAvail([` + strconv.Itoa(size) + `]); Generate($2); Close();`
	}
	return []string{"-lang=ff", "-c", script, job.InputPath, job.OutputPath}
}

func bitmapFontFormat(format domain.Format) bool {
	switch format {
	case domain.FormatBDF, domain.FormatPCF, domain.FormatFON:
		return true
	default:
		return false
	}
}
