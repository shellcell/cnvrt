package toolconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type Result struct {
	Converters   []ports.Converter
	InstallHints map[string][]ports.InstallSuggestion
}

type Config struct {
	Formats []FormatConfig `json:"formats"`
	Tools   []ToolConfig   `json:"tools"`
}

type FormatConfig struct {
	Name       string   `json:"name"`
	Aliases    []string `json:"aliases"`
	Extensions []string `json:"extensions"`
	Category   string   `json:"category"`
}

type ToolConfig struct {
	ID           string             `json:"id"`
	Commands     []string           `json:"commands"`
	Capabilities []CapabilityConfig `json:"capabilities"`
	Convert      CommandTemplate    `json:"convert"`
	Install      []InstallConfig    `json:"install"`
}

type CapabilityConfig struct {
	Input              string `json:"input"`
	Output             string `json:"output"`
	PreservesAnimation bool   `json:"preserves_animation"`
	Lossy              bool   `json:"lossy"`
	Priority           int    `json:"priority"`
}

type CommandTemplate struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Dir     string   `json:"dir"`
}

type InstallConfig struct {
	Manager string `json:"manager"`
	Package string `json:"package"`
	Command string `json:"command"`
}

type DynamicConverter struct {
	id       string
	commands []string
	caps     []domain.ConversionCapability
	template CommandTemplate
	runner   ports.CommandRunner
}

func Load(runner ports.CommandRunner) (Result, error) {
	paths, err := configPaths()
	if err != nil {
		return Result{}, err
	}

	result := Result{InstallHints: map[string][]ports.InstallSuggestion{}}
	for _, path := range paths {
		config, err := readConfig(path)
		if err != nil {
			return result, err
		}

		loaded, err := build(config, runner)
		if err != nil {
			return result, fmt.Errorf("%s: %w", path, err)
		}
		result.Converters = append(result.Converters, loaded.Converters...)
		for command, hints := range loaded.InstallHints {
			result.InstallHints[command] = append(result.InstallHints[command], hints...)
		}
	}

	return result, nil
}

func WriteUserConfig(name string, config Config) (string, error) {
	root, err := userToolsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}

	if strings.TrimSpace(name) == "" {
		name = "tool"
	}
	fileName := sanitizeFileName(name)
	if fileName == "" {
		fileName = "tool"
	}
	path := filepath.Join(root, fileName+".json")
	if _, err := os.Stat(path); err == nil {
		path = filepath.Join(root, fmt.Sprintf("%s-%d.json", fileName, time.Now().Unix()))
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}

	return path, nil
}

func (c *DynamicConverter) ID() string { return c.id }

func (c *DynamicConverter) RequiredCommands() []string {
	return append([]string(nil), c.commands...)
}

func (c *DynamicConverter) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *DynamicConverter) CanConvert(input domain.Format, output domain.Format) bool {
	for _, cap := range c.caps {
		if cap.Input == input && cap.Output == output {
			return true
		}
	}
	return false
}

func (c *DynamicConverter) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	command := c.template.Command
	if command == "" && len(c.commands) > 0 {
		command = c.commands[0]
	}
	if command == "" {
		return domain.ConversionResult{}, fmt.Errorf("%s has no command", c.id)
	}

	argTemplates := argsOrDefault(c.template.Args)
	args := make([]string, 0, len(argTemplates))
	for _, arg := range argTemplates {
		args = append(args, expand(arg, job))
	}

	result, err := c.runner.Run(ctx, ports.Command{
		Name: expand(command, job),
		Args: args,
		Dir:  expand(c.template.Dir, job),
	})
	if err != nil {
		if result.Stderr != "" {
			return domain.ConversionResult{}, fmt.Errorf("%w: %s", err, result.Stderr)
		}
		return domain.ConversionResult{}, err
	}

	return domain.ConversionResult{Job: job, Backend: c.id, OutputPath: job.OutputPath}, nil
}

func configPaths() ([]string, error) {
	var paths []string

	if env := os.Getenv("CONVERT_TOOL_CONFIG"); env != "" {
		for _, path := range filepath.SplitList(env) {
			if path != "" {
				paths = append(paths, path)
			}
		}
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	paths = appendIfExists(paths, filepath.Join(wd, "convert.tools.json"))
	paths = appendGlob(paths, filepath.Join(wd, "convert.tools.d", "*.json"))

	if root, err := userConfigRoot(); err == nil {
		paths = appendIfExists(paths, filepath.Join(root, "tools.json"))
		paths = appendGlob(paths, filepath.Join(root, "tools.d", "*.json"))
	}

	paths = dedupeStrings(paths)
	sort.Strings(paths)
	return paths, nil
}

func userConfigRoot() (string, error) {
	userConfig, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userConfig, "convert"), nil
}

func userToolsDir() (string, error) {
	root, err := userConfigRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "tools.d"), nil
}

func appendIfExists(paths []string, path string) []string {
	if _, err := os.Stat(path); err == nil {
		return append(paths, path)
	}
	return paths
}

func appendGlob(paths []string, pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return paths
	}
	return append(paths, matches...)
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}
	return config, nil
}

func build(config Config, runner ports.CommandRunner) (Result, error) {
	result := Result{InstallHints: map[string][]ports.InstallSuggestion{}}
	for _, formatConfig := range config.Formats {
		values := append([]string{}, formatConfig.Aliases...)
		values = append(values, formatConfig.Extensions...)
		if _, err := domain.RegisterFormat(formatConfig.Name, values...); err != nil {
			return result, err
		}
	}

	for _, tool := range config.Tools {
		converter, err := buildConverter(tool, runner)
		if err != nil {
			return result, err
		}
		result.Converters = append(result.Converters, converter)
		for _, command := range tool.Commands {
			for _, install := range tool.Install {
				result.InstallHints[command] = append(result.InstallHints[command], ports.InstallSuggestion{
					Manager: install.Manager,
					Package: install.Package,
					Command: installCommand(install),
				})
			}
		}
	}

	return result, nil
}

func buildConverter(tool ToolConfig, runner ports.CommandRunner) (*DynamicConverter, error) {
	if tool.ID == "" {
		return nil, fmt.Errorf("tool id is required")
	}
	if len(tool.Commands) == 0 && tool.Convert.Command == "" {
		return nil, fmt.Errorf("%s: command is required", tool.ID)
	}

	caps := make([]domain.ConversionCapability, 0, len(tool.Capabilities))
	for _, capability := range tool.Capabilities {
		input, err := domain.ParseFormat(capability.Input)
		if err != nil {
			return nil, err
		}
		output, err := domain.ParseFormat(capability.Output)
		if err != nil {
			return nil, err
		}
		caps = append(caps, domain.ConversionCapability{
			Input:              input,
			Output:             output,
			PreservesAnimation: capability.PreservesAnimation,
			Lossy:              capability.Lossy,
			Priority:           capability.Priority,
		})
	}

	return &DynamicConverter{
		id:       tool.ID,
		commands: append([]string(nil), tool.Commands...),
		caps:     caps,
		template: tool.Convert,
		runner:   runner,
	}, nil
}

func argsOrDefault(args []string) []string {
	if len(args) == 0 {
		return []string{"{input}", "{output}"}
	}
	return args
}

func expand(value string, job domain.ConvertJob) string {
	value = strings.ReplaceAll(value, "{input}", job.InputPath)
	value = strings.ReplaceAll(value, "{output}", job.OutputPath)
	value = strings.ReplaceAll(value, "{input_format}", job.InputFormat.String())
	value = strings.ReplaceAll(value, "{output_format}", job.OutputFormat.String())
	value = strings.ReplaceAll(value, "{quality}", strconv.Itoa(job.Options.Quality))
	value = strings.ReplaceAll(value, "{resize}", job.Options.Resize)
	value = strings.ReplaceAll(value, "{action}", job.Options.Action.String())
	return value
}

func installCommand(install InstallConfig) string {
	if install.Command != "" {
		return install.Command
	}
	if install.Manager != "" && install.Package != "" {
		switch install.Manager {
		case "brew":
			return "brew install " + install.Package
		case "apt":
			return "sudo apt install " + install.Package
		case "dnf":
			return "sudo dnf install " + install.Package
		case "yum":
			return "sudo yum install " + install.Package
		case "pacman":
			return "sudo pacman -S " + install.Package
		case "zypper":
			return "sudo zypper install " + install.Package
		case "apk":
			return "sudo apk add " + install.Package
		case "npm":
			return "npm install -g " + install.Package
		case "pnpm":
			return "pnpm add -g " + install.Package
		case "yarn":
			return "yarn global add " + install.Package
		case "pipx":
			return "pipx install " + install.Package
		case "pip":
			return "python -m pip install " + install.Package
		case "go":
			return "go install " + install.Package
		case "cargo":
			return "cargo install " + install.Package
		case "gem":
			return "gem install " + install.Package
		case "composer":
			return "composer global require " + install.Package
		}
	}
	return ""
}

func sanitizeFileName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		if r == ' ' || r == '/' || r == '\\' || r == ':' {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
