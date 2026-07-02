package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shellcell/convert/internal/app"
	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/theme"
)

type Config struct {
	Tools          map[string]map[string]json.RawMessage `json:"tools"`
	Pairs          []PairConfig                          `json:"pairs"`
	Backends       map[string]json.RawMessage            `json:"backends"`
	Theme          string                                `json:"theme"`
	Colors         map[string]string                     `json:"colors"`
	CategoryColors map[string]string                     `json:"category_colors"`
	UI             UIConfig                              `json:"ui"`
}

type UIConfig struct {
	Theme          string            `json:"theme"`
	Colors         map[string]string `json:"colors"`
	CategoryColors map[string]string `json:"category_colors"`
	ShowHelp       *bool             `json:"show_help,omitempty"`
}

// Options are non-color UI preferences loaded from settings files.
type Options struct {
	ShowHelp bool
}

type PairConfig struct {
	Input       string                                `json:"input"`
	Output      string                                `json:"output"`
	Tools       []string                              `json:"tools"`
	ToolOptions map[string]map[string]json.RawMessage `json:"tool_options"`
	Options     map[string]map[string]json.RawMessage `json:"options"`
}

func Load() (app.Preferences, theme.Palette, Options, error) {
	options := Options{ShowHelp: true}
	paths, err := configPaths()
	if err != nil {
		return app.Preferences{}, theme.Default(), options, err
	}

	preferences := app.Preferences{ToolOptions: domain.ToolOptions{}}
	palette := theme.Default()
	for _, path := range paths {
		config, err := readConfig(path)
		if err != nil {
			return preferences, palette, options, err
		}

		loaded, loadedPalette, err := build(config)
		if err != nil {
			return preferences, palette, options, fmt.Errorf("%s: %w", path, err)
		}
		preferences.ToolOptions = preferences.ToolOptions.Merge(loaded.ToolOptions)
		preferences.Pairs = append(preferences.Pairs, loaded.Pairs...)
		if len(loaded.Backends) > 0 {
			if preferences.Backends == nil {
				preferences.Backends = map[string][]string{}
			}
			for key, tools := range loaded.Backends {
				preferences.Backends[key] = tools
			}
		}
		palette = palette.Merge(loadedPalette)
		if config.UI.ShowHelp != nil {
			options.ShowHelp = *config.UI.ShowHelp
		}
	}

	return preferences, palette, options, nil
}

// decodeBackendPreferences normalizes "input->output" or "input" keys and
// accepts a string or list of backend IDs as values.
func decodeBackendPreferences(raw map[string]json.RawMessage) (map[string][]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	result := map[string][]string{}
	for key, rawValue := range raw {
		tools := decodeStringList(rawValue)
		if len(tools) == 0 {
			continue
		}
		normalized, err := normalizeBackendKey(key)
		if err != nil {
			return nil, err
		}
		result[normalized] = tools
	}
	return result, nil
}

func normalizeBackendKey(key string) (string, error) {
	parts := strings.SplitN(key, "->", 2)
	input, err := domain.ParseFormat(parts[0])
	if err != nil {
		return "", fmt.Errorf("backends key %q: %w", key, err)
	}
	if len(parts) == 1 {
		return input.String(), nil
	}
	output, err := domain.ParseFormat(parts[1])
	if err != nil {
		return "", fmt.Errorf("backends key %q: %w", key, err)
	}
	return app.BackendKey(input, output), nil
}

// UserConfigPath is the file the config command writes.
func UserConfigPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "convert", "settings.json"), nil
}

// SaveUserConfig merges the given theme/show-help/backend-preference values
// into the user settings file, preserving any other keys already present.
func SaveUserConfig(themeName string, showHelp bool, backendPrefs map[string]string) (string, error) {
	path, err := UserConfigPath()
	if err != nil {
		return "", err
	}

	raw := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &raw); err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
	}

	if len(backendPrefs) > 0 {
		backends := map[string]json.RawMessage{}
		if existing, ok := raw["backends"]; ok {
			_ = json.Unmarshal(existing, &backends)
		}
		for key, backend := range backendPrefs {
			normalized, err := normalizeBackendKey(key)
			if err != nil {
				return "", err
			}
			encoded, err := json.Marshal(backend)
			if err != nil {
				return "", err
			}
			backends[normalized] = encoded
		}
		encoded, err := json.Marshal(backends)
		if err != nil {
			return "", err
		}
		raw["backends"] = encoded
	}

	ui := map[string]json.RawMessage{}
	if existing, ok := raw["ui"]; ok {
		_ = json.Unmarshal(existing, &ui)
	}
	if themeName != "" {
		encoded, err := json.Marshal(themeName)
		if err != nil {
			return "", err
		}
		ui["theme"] = encoded
	}
	encodedShowHelp, err := json.Marshal(showHelp)
	if err != nil {
		return "", err
	}
	ui["show_help"] = encodedShowHelp

	encodedUI, err := json.Marshal(ui)
	if err != nil {
		return "", err
	}
	raw["ui"] = encodedUI

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func configPaths() ([]string, error) {
	var paths []string

	if userConfig, err := os.UserConfigDir(); err == nil {
		paths = appendIfExists(paths, filepath.Join(userConfig, "convert", "settings.json"))
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	paths = appendIfExists(paths, filepath.Join(wd, "convert.settings.json"))

	if env := os.Getenv("CONVERT_SETTINGS"); env != "" {
		for _, path := range filepath.SplitList(env) {
			if path != "" {
				paths = append(paths, path)
			}
		}
	}

	paths = dedupeStrings(paths)
	return paths, nil
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

func build(config Config) (app.Preferences, theme.Palette, error) {
	preferences := app.Preferences{ToolOptions: decodeToolOptions(config.Tools)}
	backends, err := decodeBackendPreferences(config.Backends)
	if err != nil {
		return preferences, theme.Default(), err
	}
	preferences.Backends = backends
	for _, pairConfig := range config.Pairs {
		input, err := domain.ParseFormat(pairConfig.Input)
		if err != nil {
			return preferences, theme.Default(), err
		}
		output, err := domain.ParseFormat(pairConfig.Output)
		if err != nil {
			return preferences, theme.Default(), err
		}

		toolOptions := decodeToolOptions(pairConfig.ToolOptions)
		toolOptions = toolOptions.Merge(decodeToolOptions(pairConfig.Options))
		preferences.Pairs = append(preferences.Pairs, app.PairPreference{
			Input:       input,
			Output:      output,
			Tools:       normalizeList(pairConfig.Tools),
			ToolOptions: toolOptions,
		})
	}
	return preferences, buildPalette(config), nil
}

func buildPalette(config Config) theme.Palette {
	name := strings.TrimSpace(config.Theme)
	if strings.TrimSpace(config.UI.Theme) != "" {
		name = config.UI.Theme
	}
	palette := theme.Named(name)
	palette = palette.Merge(theme.Palette{
		Title:           config.Colors["title"],
		Number:          config.Colors["number"],
		Hint:            config.Colors["hint"],
		Flag:            config.Colors["flag"],
		BadgeForeground: config.Colors["badge_foreground"],
		BadgeBackground: config.Colors["badge_background"],
		Prompt:          config.Colors["prompt"],
		OK:              config.Colors["ok"],
		Skip:            config.Colors["skip"],
		Fail:            config.Colors["fail"],
		Dim:             config.Colors["dim"],
		Selected:        config.Colors["selected"],
		Error:           config.Colors["error"],
		Unavailable:     config.Colors["unavailable"],
		Categories:      config.CategoryColors,
	})
	return palette.Merge(theme.Palette{
		Title:           config.UI.Colors["title"],
		Number:          config.UI.Colors["number"],
		Hint:            config.UI.Colors["hint"],
		Flag:            config.UI.Colors["flag"],
		BadgeForeground: config.UI.Colors["badge_foreground"],
		BadgeBackground: config.UI.Colors["badge_background"],
		Prompt:          config.UI.Colors["prompt"],
		OK:              config.UI.Colors["ok"],
		Skip:            config.UI.Colors["skip"],
		Fail:            config.UI.Colors["fail"],
		Dim:             config.UI.Colors["dim"],
		Selected:        config.UI.Colors["selected"],
		Error:           config.UI.Colors["error"],
		Unavailable:     config.UI.Colors["unavailable"],
		Categories:      config.UI.CategoryColors,
	})
}

func decodeToolOptions(raw map[string]map[string]json.RawMessage) domain.ToolOptions {
	options := domain.ToolOptions{}
	for tool, values := range raw {
		tool = strings.ToLower(strings.TrimSpace(tool))
		if tool == "" {
			continue
		}
		if options[tool] == nil {
			options[tool] = map[string][]string{}
		}
		for key, rawValue := range values {
			key = strings.ToLower(strings.TrimSpace(key))
			decoded := decodeStringList(rawValue)
			if key != "" && len(decoded) > 0 {
				options[tool][key] = decoded
			}
		}
	}
	return options
}

func decodeStringList(raw json.RawMessage) []string {
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return normalizeList(list)
	}

	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return normalizeList([]string{one})
	}

	return nil
}

func normalizeList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func appendIfExists(paths []string, path string) []string {
	if _, err := os.Stat(path); err == nil {
		return append(paths, path)
	}
	return paths
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
