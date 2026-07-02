package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shellcell/convert/internal/app"
	"github.com/shellcell/convert/internal/domain"
)

type Config struct {
	Tools map[string]map[string]json.RawMessage `json:"tools"`
	Pairs []PairConfig                          `json:"pairs"`
}

type PairConfig struct {
	Input       string                                `json:"input"`
	Output      string                                `json:"output"`
	Tools       []string                              `json:"tools"`
	ToolOptions map[string]map[string]json.RawMessage `json:"tool_options"`
	Options     map[string]map[string]json.RawMessage `json:"options"`
}

func Load() (app.Preferences, error) {
	paths, err := configPaths()
	if err != nil {
		return app.Preferences{}, err
	}

	preferences := app.Preferences{ToolOptions: domain.ToolOptions{}}
	for _, path := range paths {
		config, err := readConfig(path)
		if err != nil {
			return preferences, err
		}

		loaded, err := build(config)
		if err != nil {
			return preferences, fmt.Errorf("%s: %w", path, err)
		}
		preferences.ToolOptions = preferences.ToolOptions.Merge(loaded.ToolOptions)
		preferences.Pairs = append(preferences.Pairs, loaded.Pairs...)
	}

	return preferences, nil
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

func build(config Config) (app.Preferences, error) {
	preferences := app.Preferences{ToolOptions: decodeToolOptions(config.Tools)}
	for _, pairConfig := range config.Pairs {
		input, err := domain.ParseFormat(pairConfig.Input)
		if err != nil {
			return preferences, err
		}
		output, err := domain.ParseFormat(pairConfig.Output)
		if err != nil {
			return preferences, err
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
	return preferences, nil
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
