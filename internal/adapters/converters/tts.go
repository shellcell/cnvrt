package converters

import (
	"context"
	"fmt"
	"runtime"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

// TTS speaks plain text into an audio file. It uses the macOS `say` engine
// when present (aiff/m4a/wav) and falls back to espeak-ng (wav only).
type TTS struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewTTS(runner ports.CommandRunner) *TTS {
	inputs := []domain.Format{domain.FormatTXT, domain.FormatMD}
	outputs := []domain.Format{domain.FormatWAV, domain.FormatAIFF, domain.FormatM4A}
	return &TTS{runner: runner, caps: capabilities(inputs, outputs, 80, true, false)}
}

func (c *TTS) ID() string { return "tts" }

func (c *TTS) Description() string {
	return "text to speech: txt/md -> wav/aiff/m4a via say or espeak-ng"
}

// RequiredCommands is empty because either engine satisfies the backend;
// MissingDependencies reports engine availability per output format.
func (c *TTS) RequiredCommands() []string { return nil }

func (c *TTS) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *TTS) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *TTS) MissingDependencies(input domain.Format, output domain.Format, options domain.ConvertOptions) []string {
	if !c.CanConvert(input, output) {
		return nil
	}
	if _, ok := c.sayCommand(); ok {
		return nil
	}
	if output == domain.FormatWAV {
		if _, err := c.runner.LookPath("espeak-ng"); err == nil {
			return nil
		}
		return []string{"espeak-ng"}
	}
	// aiff/m4a need the macOS say engine.
	return []string{"say"}
}

func (c *TTS) DependencyChecks() []ports.DependencyCheck {
	_, saidFound := c.sayCommand()
	_, espeakErr := c.runner.LookPath("espeak-ng")
	return []ports.DependencyCheck{{
		Name:     "speech engine (say or espeak-ng)",
		Found:    saidFound || espeakErr == nil,
		Commands: []string{"espeak-ng"},
	}}
}

func (c *TTS) OptionSpecs(input domain.Format, output domain.Format) []ports.OptionSpec {
	return []ports.OptionSpec{
		{
			Tool:        "tts",
			Key:         "voice",
			Title:       "Voice",
			Description: "Voice name for the speech engine (say -v / espeak-ng -v). Empty uses the system default.",
			Default:     "",
		},
		{
			Tool:        "tts",
			Key:         "rate",
			Title:       "Speech rate",
			Description: "Words per minute. Empty uses the engine default.",
			Default:     "",
		},
	}
}

func (c *TTS) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	command, args, err := c.command(job)
	if err != nil {
		return domain.ConversionResult{}, err
	}
	return runSimple(ctx, c.runner, command, args, job, c.ID())
}

func (c *TTS) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	command, args, err := c.command(job)
	if err != nil {
		return ports.CommandPreview{}
	}
	return previewCommand(command, args)
}

func (c *TTS) command(job domain.ConvertJob) (string, []string, error) {
	options := job.Options.ToolOptions
	voice := stringOption(options, "tts", "voice", "")
	rate := stringOption(options, "tts", "rate", "")

	if say, ok := c.sayCommand(); ok {
		args := []string{"-f", job.InputPath, "-o", job.OutputPath}
		if job.OutputFormat == domain.FormatWAV {
			args = append(args, "--data-format=LEI16@22050")
		}
		if voice != "" {
			args = append(args, "-v", voice)
		}
		if rate != "" {
			args = append(args, "-r", rate)
		}
		args = append(args, extraArgs(options, "tts")...)
		return say, args, nil
	}

	if _, err := c.runner.LookPath("espeak-ng"); err == nil {
		if job.OutputFormat != domain.FormatWAV {
			return "", nil, domain.MissingDependencyError{
				Message:  fmt.Sprintf("espeak-ng only writes wav; %s output needs the macOS say engine", job.OutputFormat),
				Commands: []string{"say"},
			}
		}
		args := []string{"-f", job.InputPath, "-w", job.OutputPath}
		if voice != "" {
			args = append(args, "-v", voice)
		}
		if rate != "" {
			args = append(args, "-s", rate)
		}
		args = append(args, extraArgs(options, "tts")...)
		return "espeak-ng", args, nil
	}

	return "", nil, domain.MissingDependencyError{
		Message:  "text to speech requires the macOS say command or espeak-ng",
		Commands: []string{"espeak-ng"},
	}
}

func (c *TTS) sayCommand() (string, bool) {
	if runtime.GOOS != "darwin" {
		return "", false
	}
	if _, err := c.runner.LookPath("say"); err == nil {
		return "say", true
	}
	return "", false
}
