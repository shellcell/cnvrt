package converters

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// Whisper transcribes audio (and the audio track of video) to text with the
// OpenAI Whisper CLI, which fetches models on demand.
type Whisper struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewWhisper(runner ports.CommandRunner) *Whisper {
	inputs := []domain.Format{
		domain.FormatMP3,
		domain.FormatWAV,
		domain.FormatFLAC,
		domain.FormatM4A,
		domain.FormatOGG,
		domain.FormatOPUS,
		domain.FormatAAC,
		domain.FormatMP4,
		domain.FormatMOV,
		domain.FormatWebM,
		domain.FormatMKV,
	}
	return &Whisper{runner: runner, caps: capabilities(inputs, []domain.Format{domain.FormatTXT}, 80, false, false)}
}

func (c *Whisper) ID() string { return "whisper" }

func (c *Whisper) Description() string {
	return "speech to text: transcribe audio/video to txt via OpenAI Whisper"
}

func (c *Whisper) RequiredCommands() []string { return []string{"whisper"} }

func (c *Whisper) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Whisper) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Whisper) OptionSpecs(input domain.Format, output domain.Format) []ports.OptionSpec {
	return []ports.OptionSpec{
		{
			Tool:        "whisper",
			Key:         "model",
			Title:       "Whisper model",
			Description: "tiny, base, small, medium, large, or turbo. Larger is more accurate and slower.",
			Default:     "base",
		},
		{
			Tool:        "whisper",
			Key:         "lang",
			Title:       "Audio language",
			Description: "Language code such as en, de, ru. Empty auto-detects.",
			Default:     "",
		},
	}
}

func (c *Whisper) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	result, err := runSimple(ctx, c.runner, "whisper", c.args(job), job, c.ID())
	if err != nil {
		return result, err
	}
	if err := c.collectOutput(job); err != nil {
		return domain.ConversionResult{}, err
	}
	return result, nil
}

func (c *Whisper) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("whisper", c.args(job))
}

func (c *Whisper) args(job domain.ConvertJob) []string {
	options := job.Options.ToolOptions
	args := []string{
		job.InputPath,
		"--output_format", "txt",
		"--output_dir", filepath.Dir(job.OutputPath),
		"--model", stringOption(options, "whisper", "model", "base"),
	}
	if lang := stringOption(options, "whisper", "lang", ""); lang != "" {
		args = append(args, "--language", lang)
	}
	return append(args, extraArgs(options, "whisper")...)
}

// collectOutput renames whisper's fixed "<input-stem>.txt" output to the
// requested output path.
func (c *Whisper) collectOutput(job domain.ConvertJob) error {
	if _, err := os.Stat(job.OutputPath); err == nil {
		return nil
	}
	stem := strings.TrimSuffix(filepath.Base(job.InputPath), filepath.Ext(job.InputPath))
	produced := filepath.Join(filepath.Dir(job.OutputPath), stem+".txt")
	if produced == job.OutputPath {
		return nil
	}
	if _, err := os.Stat(produced); err != nil {
		return nil
	}
	return moveFile(produced, job.OutputPath)
}
