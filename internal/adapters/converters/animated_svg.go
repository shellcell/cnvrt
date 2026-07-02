package converters

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type AnimatedSVG struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewAnimatedSVG(runner ports.CommandRunner) *AnimatedSVG {
	outputs := []domain.Format{
		domain.FormatMP4,
		domain.FormatWebM,
		domain.FormatGIF,
		domain.FormatWebP,
		domain.FormatAPNG,
		domain.FormatMOV,
		domain.FormatMKV,
	}
	caps := make([]domain.ConversionCapability, 0, len(outputs))
	for _, output := range outputs {
		caps = append(caps, domain.ConversionCapability{Input: domain.FormatSVG, Output: output, Priority: 85, Lossy: true, PreservesAnimation: true})
	}
	return &AnimatedSVG{runner: runner, caps: caps}
}

func (c *AnimatedSVG) ID() string { return "animated-svg" }

func (c *AnimatedSVG) RequiredCommands() []string { return []string{"ffmpeg"} }

func (c *AnimatedSVG) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *AnimatedSVG) CapabilitiesForInput(path string, input domain.Format) []domain.ConversionCapability {
	if input != domain.FormatSVG || !animatedSVG(path) {
		return nil
	}
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *AnimatedSVG) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *AnimatedSVG) MissingDependencies(input domain.Format, output domain.Format, options domain.ConvertOptions) []string {
	if !c.CanConvert(input, output) {
		return nil
	}
	if _, ok := c.browserCommand(); ok {
		return nil
	}
	return []string{"chromium"}
}

func (c *AnimatedSVG) DependencyChecks() []ports.DependencyCheck {
	_, found := c.browserCommand()
	return []ports.DependencyCheck{{
		Name:     "browser (Chrome/Chromium)",
		Found:    found,
		Commands: []string{"chromium"},
	}}
}

func (c *AnimatedSVG) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	browser, ok := c.browserCommand()
	if !ok {
		return domain.ConversionResult{}, domain.MissingDependencyError{Message: "animated SVG conversion requires a headless browser", Commands: []string{"chromium"}}
	}

	width, height := c.outputSize(job)
	fps := intOption(job.Options.ToolOptions, "animated_svg", "fps", 30)
	duration := intOption(job.Options.ToolOptions, "animated_svg", "duration", 3)
	if fps <= 0 {
		fps = 30
	}
	if duration <= 0 {
		duration = 3
	}
	frames := max(1, fps*duration)

	tmpDir, err := os.MkdirTemp("", "convert-animated-svg-*")
	if err != nil {
		return domain.ConversionResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	inputURI, err := fileURI(job.InputPath)
	if err != nil {
		return domain.ConversionResult{}, err
	}
	for i := 0; i < frames; i++ {
		frame := filepath.Join(tmpDir, fmt.Sprintf("frame-%06d.png", i))
		budget := i * 1000 / fps
		args := []string{
			"--headless",
			"--disable-gpu",
			"--hide-scrollbars",
			"--window-size=" + strconv.Itoa(width) + "," + strconv.Itoa(height),
			"--virtual-time-budget=" + strconv.Itoa(budget),
			"--screenshot=" + frame,
			inputURI,
		}
		result, err := c.runner.Run(ctx, ports.Command{Name: browser, Args: args})
		if err != nil {
			return domain.ConversionResult{}, commandError(result, err)
		}
	}

	args := []string{"-hide_banner", "-loglevel", "error"}
	if job.Options.Overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}
	args = append(args, "-framerate", strconv.Itoa(fps), "-i", filepath.Join(tmpDir, "frame-%06d.png"))
	switch job.OutputFormat {
	case domain.FormatMP4, domain.FormatMOV, domain.FormatMKV:
		args = append(args, "-pix_fmt", "yuv420p")
	}
	args = append(args, job.OutputPath)
	result, err := c.runner.Run(ctx, ports.Command{Name: "ffmpeg", Args: args})
	if err != nil {
		return domain.ConversionResult{}, commandError(result, err)
	}

	return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
}

func (c *AnimatedSVG) browserCommand() (string, bool) {
	if c.runner != nil {
		for _, command := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome", "msedge", "microsoft-edge", "brave-browser"} {
			if _, err := c.runner.LookPath(command); err == nil {
				return command, true
			}
		}
	}
	for _, command := range browserAppPaths() {
		if executableFile(command) {
			return command, true
		}
	}
	return "", false
}

func browserAppPaths() []string {
	home, _ := os.UserHomeDir()
	roots := []string{"/Applications"}
	if home != "" {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	apps := []struct {
		bundle string
		binary string
	}{
		{bundle: "Google Chrome.app", binary: "Google Chrome"},
		{bundle: "Google Chrome for Testing.app", binary: "Google Chrome for Testing"},
		{bundle: "Chromium.app", binary: "Chromium"},
		{bundle: "Microsoft Edge.app", binary: "Microsoft Edge"},
		{bundle: "Brave Browser.app", binary: "Brave Browser"},
	}
	paths := make([]string, 0, len(roots)*len(apps))
	for _, root := range roots {
		for _, app := range apps {
			paths = append(paths, filepath.Join(root, app.bundle, "Contents", "MacOS", app.binary))
		}
	}
	return paths
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func (c *AnimatedSVG) outputSize(job domain.ConvertJob) (int, int) {
	if width, height, ok := parseFullSize(job.Options.Resize); ok {
		return width, height
	}
	if size, ok := svgIntrinsicSize(job.InputPath); ok {
		if width, height, ok := parseFullSize(size); ok {
			return width, height
		}
	}
	return 1024, 1024
}

func intOption(options domain.ToolOptions, tool string, key string, fallback int) int {
	values := options.Values(tool, key)
	if len(values) == 0 {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(values[0]))
	if err != nil {
		return fallback
	}
	return value
}

func parseFullSize(value string) (int, int, bool) {
	widthValue, heightValue := resizeDimensions(value)
	width, err := strconv.Atoi(widthValue)
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err := strconv.Atoi(heightValue)
	if err != nil || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func fileURI(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return (&url.URL{Scheme: "file", Path: abs}).String(), nil
}

func animatedSVG(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	if !strings.Contains(lower, "<svg") {
		return false
	}
	for _, marker := range []string{"<animate", "<set", "@keyframes", "animation:", "animation-name", "<script", "requestanimationframe"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func svgIntrinsicSize(path string) (string, bool) {
	file, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer file.Close()

	decoder := xml.NewDecoder(file)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return "", false
		}
		if err != nil {
			return "", false
		}
		start, ok := token.(xml.StartElement)
		if !ok || strings.ToLower(start.Name.Local) != "svg" {
			continue
		}
		attrs := map[string]string{}
		for _, attr := range start.Attr {
			attrs[strings.ToLower(attr.Name.Local)] = attr.Value
		}
		if width, ok := svgLength(attrs["width"]); ok {
			if height, ok := svgLength(attrs["height"]); ok {
				return svgSizeString(width, height), true
			}
		}
		if width, height, ok := svgViewBoxSize(attrs["viewbox"]); ok {
			return svgSizeString(width, height), true
		}
		return "", false
	}
}

func svgLength(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasSuffix(value, "%") {
		return 0, false
	}
	end := 0
	for end < len(value) {
		ch := value[end]
		if (ch >= '0' && ch <= '9') || ch == '.' || ch == '+' || ch == '-' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(value[:end], 64)
	if err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

func svgViewBoxSize(value string) (float64, float64, bool) {
	fields := strings.Fields(strings.ReplaceAll(strings.TrimSpace(value), ",", " "))
	if len(fields) != 4 {
		return 0, 0, false
	}
	width, err := strconv.ParseFloat(fields[2], 64)
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err := strconv.ParseFloat(fields[3], 64)
	if err != nil || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func svgSizeString(width float64, height float64) string {
	return strconv.Itoa(max(1, int(math.Round(width)))) + "x" + strconv.Itoa(max(1, int(math.Round(height))))
}
