package app

import (
	"sort"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type Preferences struct {
	Pairs       []PairPreference
	ToolOptions domain.ToolOptions
	// Backends maps "input->output" pair keys or bare "input" format keys
	// to ordered preferred backend IDs. When a preferred backend is
	// installed, it is used without asking.
	Backends map[string][]string
}

// BackendKey builds the pair key used in Preferences.Backends.
func BackendKey(input domain.Format, output domain.Format) string {
	return input.String() + "->" + output.String()
}

type PairPreference struct {
	Input       domain.Format
	Output      domain.Format
	Tools       []string
	ToolOptions domain.ToolOptions
}

func (p Preferences) OptionsFor(input domain.Format, output domain.Format) domain.ToolOptions {
	options := p.ToolOptions.Clone()
	for _, pair := range p.Pairs {
		if pair.Input == input && pair.Output == output {
			options = options.Merge(pair.ToolOptions)
		}
	}
	return options
}

func (p Preferences) OrderConverters(input domain.Format, output domain.Format, converters []ports.Converter) []ports.Converter {
	preferred := p.preferredTools(input, output)
	if len(preferred) == 0 || len(converters) < 2 {
		return converters
	}

	rank := map[string]int{}
	for i, tool := range preferred {
		tool = strings.ToLower(strings.TrimSpace(tool))
		if tool != "" {
			rank[tool] = i
		}
	}
	if len(rank) == 0 {
		return converters
	}

	ordered := append([]ports.Converter(nil), converters...)
	original := map[string]int{}
	for i, converter := range ordered {
		original[converter.ID()] = i
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		leftRank, leftOK := rank[strings.ToLower(ordered[i].ID())]
		rightRank, rightOK := rank[strings.ToLower(ordered[j].ID())]
		if leftOK && rightOK {
			return leftRank < rightRank
		}
		if leftOK != rightOK {
			return leftOK
		}
		return original[ordered[i].ID()] < original[ordered[j].ID()]
	})
	return ordered
}

func (p Preferences) preferredTools(input domain.Format, output domain.Format) []string {
	for i := len(p.Pairs) - 1; i >= 0; i-- {
		pair := p.Pairs[i]
		if pair.Input == input && pair.Output == output && len(pair.Tools) > 0 {
			return append([]string(nil), pair.Tools...)
		}
	}
	if tools := p.Backends[BackendKey(input, output)]; len(tools) > 0 {
		return append([]string(nil), tools...)
	}
	if tools := p.Backends[input.String()]; len(tools) > 0 {
		return append([]string(nil), tools...)
	}
	return nil
}

// PreferredConverter returns the first installed converter matching a
// configured backend preference for this pair, or nil when none is set.
func (p Preferences) PreferredConverter(input domain.Format, output domain.Format, available []ports.Converter) ports.Converter {
	for _, tool := range p.preferredTools(input, output) {
		tool = strings.ToLower(strings.TrimSpace(tool))
		for _, converter := range available {
			if strings.ToLower(converter.ID()) == tool {
				return converter
			}
		}
	}
	return nil
}
