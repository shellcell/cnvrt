package app

import (
	"context"
	"testing"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

func TestDetectInputFormatFallsBackToTextWithoutExtension(t *testing.T) {
	service := NewService(nil, nil, testFileSystem{text: map[string]bool{"README": true}}, nil, nil, nil, Preferences{}, nil)

	format, err := service.detectInputFormat("README", "")
	if err != nil {
		t.Fatal(err)
	}
	if format != domain.FormatTXT {
		t.Fatalf("expected txt, got %s", format)
	}
}

func TestDetectInputFormatFallsBackToTextForUnsupportedExtension(t *testing.T) {
	service := NewService(nil, nil, testFileSystem{text: map[string]bool{"notes.weird": true}}, nil, nil, nil, Preferences{}, nil)

	format, err := service.detectInputFormat("notes.weird", "")
	if err != nil {
		t.Fatal(err)
	}
	if format != domain.FormatTXT {
		t.Fatalf("expected txt, got %s", format)
	}
}

func TestDetectInputFormatKeepsCustomFormatWithCapability(t *testing.T) {
	custom := domain.Format("weird")
	service := NewService([]ports.Converter{testConverter{input: custom}}, nil, testFileSystem{text: map[string]bool{"notes.weird": true}}, nil, nil, nil, Preferences{}, nil)

	format, err := service.detectInputFormat("notes.weird", "")
	if err != nil {
		t.Fatal(err)
	}
	if format != custom {
		t.Fatalf("expected %s, got %s", custom, format)
	}
}

type testFileSystem struct {
	dirs map[string]bool
	text map[string]bool
}

func (fs testFileSystem) CurrentDir() (string, error)          { return ".", nil }
func (fs testFileSystem) Abs(path string) (string, error)      { return path, nil }
func (fs testFileSystem) Exists(path string) (bool, error)     { return true, nil }
func (fs testFileSystem) IsDir(path string) (bool, error)      { return fs.dirs[path], nil }
func (fs testFileSystem) IsTextFile(path string) (bool, error) { return fs.text[path], nil }
func (fs testFileSystem) EnsureDir(string) error               { return nil }

type testConverter struct {
	input domain.Format
}

func (c testConverter) ID() string                 { return "test" }
func (c testConverter) RequiredCommands() []string { return nil }
func (c testConverter) Capabilities() []domain.ConversionCapability {
	return []domain.ConversionCapability{{Input: c.input, Output: domain.FormatTXT}}
}
func (c testConverter) CanConvert(input domain.Format, output domain.Format) bool {
	return input == c.input && output == domain.FormatTXT
}
func (c testConverter) Convert(context.Context, domain.ConvertJob) (domain.ConversionResult, error) {
	return domain.ConversionResult{}, nil
}
