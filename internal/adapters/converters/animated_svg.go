package converters

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
	"github.com/shellcell/convert/internal/scan"
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

func (c *AnimatedSVG) Description() string {
	return "animated svg -> video/gif/webp via headless browser + ffmpeg"
}

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

func (c *AnimatedSVG) OptionSpecs(input domain.Format, output domain.Format) []ports.OptionSpec {
	return []ports.OptionSpec{
		{
			Tool:        "animated_svg",
			Key:         "fps",
			Title:       "Animation frame rate",
			Description: "Frames per second captured from the animated SVG.",
			Default:     "30",
		},
		{
			Tool:        "animated_svg",
			Key:         "duration",
			Title:       "Animation duration (seconds)",
			Description: "Capture length in seconds. Empty detects the duration from the SVG.",
			Default:     "",
		},
	}
}

func (c *AnimatedSVG) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return c.convert(ctx, job, "")
}

func (c *AnimatedSVG) ConvertWithCommand(ctx context.Context, job domain.ConvertJob, command string) (domain.ConversionResult, error) {
	return c.convert(ctx, job, command)
}

func (c *AnimatedSVG) convert(ctx context.Context, job domain.ConvertJob, commandOverride string) (domain.ConversionResult, error) {
	browser, ok := c.browserCommand()
	if !ok {
		return domain.ConversionResult{}, domain.MissingDependencyError{Message: "animated SVG conversion requires a headless browser", Commands: []string{"chromium"}}
	}

	width, height := c.outputSize(job)
	if requiresEvenVideoDimensions(job.OutputFormat) {
		width, height = evenDimensions(width, height)
	}
	fps := intOption(job.Options.ToolOptions, "animated_svg", "fps", 30)
	duration := floatOption(job.Options.ToolOptions, "animated_svg", "duration", 0)
	if fps <= 0 {
		fps = 30
	}
	if duration <= 0 {
		duration = svgAnimationDuration(job.InputPath, 3)
	}
	frames := max(1, int(math.Ceil(float64(fps)*duration)))

	tmpDir, err := os.MkdirTemp("", "convert-animated-svg-*")
	if err != nil {
		return domain.ConversionResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	inputURI, err := fileURI(job.InputPath)
	if err != nil {
		return domain.ConversionResult{}, err
	}
	if err := captureAnimatedSVGFrames(ctx, browser, inputURI, tmpDir, width, height, fps, frames); err != nil {
		return domain.ConversionResult{}, err
	}

	if strings.TrimSpace(commandOverride) != "" {
		command := strings.ReplaceAll(commandOverride, "<temp-dir>", tmpDir)
		result, err := c.runner.Run(ctx, shellCommand(command))
		if err != nil {
			return domain.ConversionResult{}, commandStringError(command, result, err)
		}
		return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
	}

	args := c.ffmpegArgs(job, tmpDir, fps)
	command := ports.Command{Name: "ffmpeg", Args: args}
	result, err := c.runner.Run(ctx, command)
	if err != nil {
		return domain.ConversionResult{}, commandError(command, result, err)
	}

	return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
}

func (c *AnimatedSVG) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	browser, ok := c.browserCommand()
	if !ok {
		browser = "chromium"
	}
	width, height := c.outputSize(job)
	if requiresEvenVideoDimensions(job.OutputFormat) {
		width, height = evenDimensions(width, height)
	}
	fps := intOption(job.Options.ToolOptions, "animated_svg", "fps", 30)
	if fps <= 0 {
		fps = 30
	}

	browserArgs := []string{
		"--headless=new",
		"--disable-gpu",
		"--hide-scrollbars",
		"--no-first-run",
		"--no-default-browser-check",
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=<port>",
		"--user-data-dir=" + filepath.Join("<temp-dir>", "browser-profile"),
		"--window-size=" + strconv.Itoa(width) + "," + strconv.Itoa(height),
		"about:blank",
	}
	return ports.CommandPreview{Commands: []ports.Command{
		{Name: browser, Args: browserArgs},
		{Name: "ffmpeg", Args: c.ffmpegArgs(job, "<temp-dir>", fps)},
	}, Editable: true, EditableCommand: 1}
}

func (c *AnimatedSVG) ffmpegArgs(job domain.ConvertJob, frameDir string, fps int) []string {
	args := []string{"-hide_banner", "-loglevel", "error"}
	if job.Options.Overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}
	args = append(args, "-framerate", strconv.Itoa(fps), "-i", filepath.Join(frameDir, "frame-%06d.png"))
	switch job.OutputFormat {
	case domain.FormatMP4, domain.FormatMOV, domain.FormatMKV:
		args = append(args, "-pix_fmt", "yuv420p")
	}
	args = append(args, job.OutputPath)
	return args
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

func captureAnimatedSVGFrames(ctx context.Context, browser string, inputURI string, tmpDir string, width int, height int, fps int, frames int) error {
	port, err := freeLocalPort()
	if err != nil {
		return err
	}
	profileDir := filepath.Join(tmpDir, "browser-profile")
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--hide-scrollbars",
		"--no-first-run",
		"--no-default-browser-check",
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=" + strconv.Itoa(port),
		"--user-data-dir=" + profileDir,
		"--window-size=" + strconv.Itoa(width) + "," + strconv.Itoa(height),
		"about:blank",
	}
	browserCommand := ports.Command{Name: browser, Args: args}
	cmd := exec.CommandContext(ctx, browser, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("command: %s: %w", commandLine(browserCommand), err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	wsURL, err := waitPageWebSocket(ctx, port)
	if err != nil {
		return fmt.Errorf("command: %s: %w", commandLine(browserCommand), err)
	}
	client, err := newCDPClient(ctx, wsURL)
	if err != nil {
		return fmt.Errorf("command: %s: %w", commandLine(browserCommand), err)
	}
	defer client.Close()

	if _, err := client.call(ctx, "Page.enable", nil); err != nil {
		return err
	}
	if _, err := client.call(ctx, "Runtime.enable", nil); err != nil {
		return err
	}
	if _, err := client.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]interface{}{
		"width":             width,
		"height":            height,
		"deviceScaleFactor": 1,
		"mobile":            false,
	}); err != nil {
		return err
	}
	if _, err := client.call(ctx, "Page.navigate", map[string]interface{}{"url": inputURI}); err != nil {
		return err
	}
	_ = client.waitEvent(ctx, "Page.loadEventFired", 5*time.Second)

	for i := 0; i < frames; i++ {
		ms := i * 1000 / fps
		if _, err := client.call(ctx, "Runtime.evaluate", map[string]interface{}{
			"expression":   animationSeekExpression(ms),
			"awaitPromise": false,
		}); err != nil {
			return err
		}
		result, err := client.call(ctx, "Page.captureScreenshot", map[string]interface{}{
			"format":      "png",
			"fromSurface": true,
		})
		if err != nil {
			return err
		}
		var encoded string
		if err := json.Unmarshal(result["data"], &encoded); err != nil {
			return err
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return err
		}
		frame := filepath.Join(tmpDir, fmt.Sprintf("frame-%06d.png", i))
		if err := os.WriteFile(frame, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func freeLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address: %s", listener.Addr())
	}
	return addr.Port, nil
}

func waitPageWebSocket(ctx context.Context, port int) (string, error) {
	client := http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(10 * time.Second)
	endpoint := "http://127.0.0.1:" + strconv.Itoa(port) + "/json/list"
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err == nil {
			var targets []struct {
				Type                 string `json:"type"`
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&targets)
			_ = resp.Body.Close()
			if decodeErr == nil {
				for _, target := range targets {
					if target.Type == "page" && target.WebSocketDebuggerURL != "" {
						return target.WebSocketDebuggerURL, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return "", fmt.Errorf("browser did not expose a DevTools page target")
}

type cdpClient struct {
	conn *websocket.Conn
	next int
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cdpMessage struct {
	ID     int                        `json:"id"`
	Method string                     `json:"method"`
	Result map[string]json.RawMessage `json:"result"`
	Error  *cdpError                  `json:"error"`
	Params map[string]json.RawMessage `json:"params"`
}

func newCDPClient(ctx context.Context, wsURL string) (*cdpClient, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	return &cdpClient{conn: conn}, nil
}

func (c *cdpClient) Close() error {
	return c.conn.Close()
}

func (c *cdpClient) call(ctx context.Context, method string, params map[string]interface{}) (map[string]json.RawMessage, error) {
	c.next++
	id := c.next
	message := map[string]interface{}{"id": id, "method": method}
	if params != nil {
		message["params"] = params
	}
	if err := c.conn.WriteJSON(message); err != nil {
		return nil, err
	}
	for {
		var response cdpMessage
		if err := readCDPMessage(ctx, c.conn, &response); err != nil {
			return nil, err
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, response.Error.Message)
		}
		return response.Result, nil
	}
}

func (c *cdpClient) waitEvent(ctx context.Context, method string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		var message cdpMessage
		if err := readCDPMessage(ctx, c.conn, &message); err != nil {
			return err
		}
		if message.Method == method {
			return nil
		}
	}
}

func readCDPMessage(ctx context.Context, conn *websocket.Conn, target *cdpMessage) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	} else {
		_ = conn.SetReadDeadline(time.Time{})
	}
	err := conn.ReadJSON(target)
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func animationSeekExpression(ms int) string {
	return fmt.Sprintf(`(() => {
  const ms = %d;
  const root = document.querySelector('svg') || document.documentElement;
  if (root && typeof root.setCurrentTime === 'function') {
    try { root.setCurrentTime(ms / 1000); } catch (_) {}
  }
  if (document.getAnimations) {
    for (const animation of document.getAnimations({subtree: true})) {
      try { animation.pause(); animation.currentTime = ms; } catch (_) {}
    }
  }
  return true;
})()`, ms)
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

func requiresEvenVideoDimensions(format domain.Format) bool {
	switch format {
	case domain.FormatMP4, domain.FormatMOV, domain.FormatMKV:
		return true
	default:
		return false
	}
}

func evenDimensions(width int, height int) (int, int) {
	if width%2 != 0 {
		width++
	}
	if height%2 != 0 {
		height++
	}
	return width, height
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

func svgAnimationDuration(path string, fallback float64) float64 {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	maxDuration := maxFloat(smilAnimationDuration(data), cssAnimationDuration(string(data)))
	if maxDuration <= 0 {
		return fallback
	}
	return maxDuration
}

func smilAnimationDuration(data []byte) float64 {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	maxDuration := 0.0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return maxDuration
		}
		start, ok := token.(xml.StartElement)
		if !ok || !smilAnimationElement(start.Name.Local) {
			continue
		}

		attrs := map[string]string{}
		for _, attr := range start.Attr {
			attrs[strings.ToLower(attr.Name.Local)] = attr.Value
		}
		begin, _ := clockValue(firstTimingValue(attrs["begin"]))
		duration, hasDuration := clockValue(attrs["dur"])
		repeatDuration, hasRepeatDuration := clockValue(attrs["repeatdur"])
		if hasRepeatDuration {
			maxDuration = maxFloat(maxDuration, begin+repeatDuration)
			continue
		}
		if !hasDuration {
			continue
		}
		if repeatCount, ok := repeatCount(attrs["repeatcount"]); ok {
			maxDuration = maxFloat(maxDuration, begin+duration*repeatCount)
			continue
		}
		maxDuration = maxFloat(maxDuration, begin+duration)
	}
	return maxDuration
}

func cssAnimationDuration(content string) float64 {
	lower := strings.ToLower(content)
	maxDuration := 0.0
	durationRE := regexp.MustCompile(`animation-duration\s*:\s*([^;}]*)`)
	for _, match := range durationRE.FindAllStringSubmatch(lower, -1) {
		for _, part := range strings.Split(match[1], ",") {
			if duration, ok := clockValue(strings.TrimSpace(part)); ok {
				maxDuration = maxFloat(maxDuration, duration)
			}
		}
	}
	shorthandRE := regexp.MustCompile(`animation\s*:\s*([^;}]*)`)
	timeRE := regexp.MustCompile(`[-+]?\d*\.?\d+(?:ms|s|min|h)?`)
	for _, match := range shorthandRE.FindAllStringSubmatch(lower, -1) {
		for _, animation := range strings.Split(match[1], ",") {
			for _, token := range timeRE.FindAllString(animation, -1) {
				if duration, ok := clockValue(token); ok {
					maxDuration = maxFloat(maxDuration, duration)
					break
				}
			}
		}
	}
	return maxDuration
}

func smilAnimationElement(name string) bool {
	switch strings.ToLower(name) {
	case "animate", "set", "animatemotion", "animatetransform", "animatecolor":
		return true
	default:
		return false
	}
}

func firstTimingValue(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.Index(value, ";"); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return value
}

func repeatCount(value string) (float64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "indefinite" {
		return 0, false
	}
	count, err := strconv.ParseFloat(value, 64)
	if err != nil || count <= 0 {
		return 0, false
	}
	return count, true
}

func clockValue(value string) (float64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "indefinite" {
		return 0, false
	}
	if strings.Contains(value, ":") {
		parts := strings.Split(value, ":")
		seconds := 0.0
		for _, part := range parts {
			value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil {
				return 0, false
			}
			seconds = seconds*60 + value
		}
		return seconds, seconds > 0
	}
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "ms"):
		multiplier = 0.001
		value = strings.TrimSuffix(value, "ms")
	case strings.HasSuffix(value, "min"):
		multiplier = 60
		value = strings.TrimSuffix(value, "min")
	case strings.HasSuffix(value, "s"):
		value = strings.TrimSuffix(value, "s")
	case strings.HasSuffix(value, "h"):
		multiplier = 3600
		value = strings.TrimSuffix(value, "h")
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || seconds <= 0 {
		return 0, false
	}
	return seconds * multiplier, true
}

func maxFloat(left float64, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func svgIntrinsicSize(path string) (string, bool) {
	size, ok, err := scan.SVGSize(path)
	return size, err == nil && ok
}
