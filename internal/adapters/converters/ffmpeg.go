package converters

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type FFmpeg struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

var (
	ffmpegVideoInputs = []domain.Format{
		domain.FormatMP4,
		domain.FormatMOV,
		domain.FormatAVI,
		domain.FormatWebM,
		domain.FormatMKV,
		domain.FormatM4V,
		domain.FormatMPG,
		domain.FormatMPEG,
		domain.FormatFLV,
		domain.FormatOGV,
	}
	ffmpegAnimationInputs = []domain.Format{
		domain.FormatGIF,
		domain.FormatAPNG,
		domain.FormatWebP,
	}
	ffmpegAudioInputs = []domain.Format{
		domain.FormatMP3,
		domain.FormatWAV,
		domain.FormatFLAC,
		domain.FormatAAC,
		domain.FormatM4A,
		domain.FormatOGG,
		domain.FormatOPUS,
		domain.FormatWMA,
		domain.FormatAIFF,
	}
	ffmpegVideoOutputs = []domain.Format{
		domain.FormatMP4,
		domain.FormatWebM,
		domain.FormatMOV,
		domain.FormatAVI,
		domain.FormatMKV,
		domain.FormatM4V,
	}
	ffmpegAnimationOutputs = []domain.Format{
		domain.FormatGIF,
		domain.FormatWebP,
		domain.FormatAPNG,
	}
	ffmpegAudioOutputs = []domain.Format{
		domain.FormatMP3,
		domain.FormatWAV,
		domain.FormatFLAC,
		domain.FormatAAC,
		domain.FormatM4A,
		domain.FormatOGG,
		domain.FormatOPUS,
		domain.FormatAIFF,
	}
)

// NewFFmpeg builds a capability matrix that only contains pairs FFmpeg can
// actually produce: video converts to video/animation and extracts audio,
// animations convert to video/animation, and audio stays audio. Audio can
// never become a picture, so those pairs are not declared.
func NewFFmpeg(runner ports.CommandRunner) *FFmpeg {
	visualOutputs := append(append([]domain.Format{}, ffmpegVideoOutputs...), ffmpegAnimationOutputs...)

	caps := capabilities(ffmpegVideoInputs, append(append([]domain.Format{}, visualOutputs...), ffmpegAudioOutputs...), 90, true, true)
	caps = append(caps, capabilities(ffmpegAnimationInputs, visualOutputs, 90, true, true)...)
	caps = append(caps, capabilities(ffmpegAudioInputs, ffmpegAudioOutputs, 90, true, false)...)
	return &FFmpeg{runner: runner, caps: caps}
}

func (c *FFmpeg) ID() string { return "ffmpeg" }

func (c *FFmpeg) Description() string {
	return "video/audio/animation conversion, audio extraction, gif rendering"
}

func (c *FFmpeg) RequiredCommands() []string { return []string{"ffmpeg"} }

func (c *FFmpeg) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *FFmpeg) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *FFmpeg) OptionSpecs(input domain.Format, output domain.Format) []ports.OptionSpec {
	var specs []ports.OptionSpec
	if lossyAudioFormat(output) {
		specs = append(specs, ports.OptionSpec{
			Tool:        "ffmpeg",
			Key:         "audio_bitrate",
			Title:       "Audio bitrate",
			Description: "Target audio bitrate, for example 128k, 192k, or 320k.",
			Default:     defaultAudioBitrate,
		})
	}
	if output == domain.FormatGIF {
		specs = append(specs, ports.OptionSpec{
			Tool:        "ffmpeg",
			Key:         "fps",
			Title:       "GIF frame rate",
			Description: "Frames per second for the generated GIF.",
			Default:     strconv.Itoa(defaultGIFFPS),
		})
	}
	if output.IsVideo() {
		specs = append(specs, ports.OptionSpec{
			Tool:        "ffmpeg",
			Key:         "crf",
			Title:       "Video quality (CRF)",
			Description: "Constant rate factor; lower is better quality. Empty keeps the encoder default.",
			Default:     "",
		})
	}
	return specs
}

func (c *FFmpeg) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "ffmpeg", c.args(job), job, c.ID())
}

func (c *FFmpeg) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("ffmpeg", c.args(job))
}

const (
	defaultAudioBitrate = "192k"
	defaultGIFFPS       = 15
)

func (c *FFmpeg) args(job domain.ConvertJob) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if job.Options.Overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}
	args = append(args, "-i", job.InputPath)

	options := job.Options.ToolOptions
	switch {
	case job.OutputFormat.IsAudio():
		if job.InputFormat.IsVideo() {
			args = append(args, "-vn")
		}
		if lossyAudioFormat(job.OutputFormat) {
			args = append(args, "-b:a", stringOption(options, "ffmpeg", "audio_bitrate", defaultAudioBitrate))
		}
	case job.OutputFormat == domain.FormatGIF:
		args = append(args, "-filter_complex", gifPaletteFilter(job, intOption(options, "ffmpeg", "fps", defaultGIFFPS)))
	default:
		if filter := videoFilter(job); filter != "" {
			args = append(args, "-vf", filter)
		}
		if ffmpegNeedsEvenVideo(job.InputFormat, job.OutputFormat) {
			args = append(args, "-pix_fmt", "yuv420p")
		}
		if job.OutputFormat.IsVideo() {
			crf := stringOption(options, "ffmpeg", "crf", "")
			if crf == "" && job.Options.Action == domain.ActionCompress {
				crf = strconv.Itoa(crfFromQuality(job.Options.Quality))
			}
			if crf != "" {
				args = append(args, "-crf", crf)
			}
		}
	}

	args = append(args, extraArgs(options, "ffmpeg")...)
	args = append(args, job.OutputPath)
	return args
}

// gifPaletteFilter renders GIFs through palettegen/paletteuse; the default
// 256-color web palette produces visibly dithered, banded output.
func gifPaletteFilter(job domain.ConvertJob, fps int) string {
	if fps <= 0 {
		fps = defaultGIFFPS
	}
	chain := fmt.Sprintf("[0:v]fps=%d", fps)
	if scale := scaleFilter(job.Options.Resize, false); scale != "" {
		chain += "," + scale
	}
	return chain + ",split[a][b];[a]palettegen[p];[b][p]paletteuse"
}

func videoFilter(job domain.ConvertJob) string {
	even := ffmpegNeedsEvenVideo(job.InputFormat, job.OutputFormat)
	scale := scaleFilter(job.Options.Resize, even)
	if scale == "" && even {
		return "scale=trunc(iw/2)*2:trunc(ih/2)*2"
	}
	return scale
}

// scaleFilter converts a resize value such as 800x, x600, 800x600, or 50%
// into an ffmpeg scale filter. When even is set, free dimensions use -2 so
// yuv420p encoders receive even sizes.
func scaleFilter(resize string, even bool) string {
	resize = strings.TrimSpace(resize)
	if resize == "" {
		return ""
	}
	if strings.HasSuffix(resize, "%") {
		value, err := strconv.ParseFloat(strings.TrimSuffix(resize, "%"), 64)
		if err != nil || value <= 0 {
			return ""
		}
		factor := strconv.FormatFloat(value/100, 'f', -1, 64)
		if even {
			return fmt.Sprintf("scale=trunc(iw*%s/2)*2:trunc(ih*%s/2)*2", factor, factor)
		}
		return fmt.Sprintf("scale=iw*%s:ih*%s", factor, factor)
	}

	width, height := resizeDimensions(resize)
	if width == "" && height == "" {
		return ""
	}
	free := "-1"
	if even {
		free = "-2"
	}
	if width == "" {
		width = free
	}
	if height == "" {
		height = free
	}
	return "scale=" + width + ":" + height
}

// crfFromQuality maps the generic 1-100 quality scale onto a CRF value:
// quality 100 -> 18 (visually lossless), 85 -> 23 (x264 default region),
// 50 -> 34. Unset quality lands on 28, a reasonable "make it smaller".
func crfFromQuality(quality int) int {
	if quality <= 0 {
		return 28
	}
	crf := 18 + (100-quality)/3
	if crf > 45 {
		crf = 45
	}
	return crf
}

func lossyAudioFormat(format domain.Format) bool {
	switch format {
	case domain.FormatMP3, domain.FormatAAC, domain.FormatM4A, domain.FormatOGG, domain.FormatOPUS:
		return true
	default:
		return false
	}
}

func ffmpegNeedsEvenVideo(input domain.Format, output domain.Format) bool {
	if input.IsAudio() {
		return false
	}
	switch output {
	case domain.FormatMP4, domain.FormatMOV, domain.FormatMKV, domain.FormatM4V:
		return true
	default:
		return false
	}
}
