package converters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type LibreOffice struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewLibreOffice(runner ports.CommandRunner) *LibreOffice {
	return &LibreOffice{
		runner: runner,
		caps: []domain.ConversionCapability{
			{Input: domain.FormatDOCX, Output: domain.FormatPDF, Priority: 80},
			{Input: domain.FormatDOCX, Output: domain.FormatHTML, Priority: 80},
			{Input: domain.FormatDOCX, Output: domain.FormatTXT, Priority: 80},
			{Input: domain.FormatDOCX, Output: domain.FormatRTF, Priority: 80},
			{Input: domain.FormatDOCX, Output: domain.FormatODT, Priority: 80},
			{Input: domain.FormatODT, Output: domain.FormatPDF, Priority: 80},
			{Input: domain.FormatODT, Output: domain.FormatDOCX, Priority: 80},
			{Input: domain.FormatODT, Output: domain.FormatRTF, Priority: 80},
			{Input: domain.FormatRTF, Output: domain.FormatPDF, Priority: 80},
			{Input: domain.FormatRTF, Output: domain.FormatDOCX, Priority: 80},
			{Input: domain.FormatXLSX, Output: domain.FormatPDF, Priority: 80},
			{Input: domain.FormatXLSX, Output: domain.FormatCSV, Priority: 80},
			{Input: domain.FormatXLSX, Output: domain.FormatODS, Priority: 80},
			{Input: domain.FormatODS, Output: domain.FormatXLSX, Priority: 80},
			{Input: domain.FormatODS, Output: domain.FormatCSV, Priority: 80},
			{Input: domain.FormatODS, Output: domain.FormatPDF, Priority: 80},
			{Input: domain.FormatCSV, Output: domain.FormatXLSX, Priority: 80},
			{Input: domain.FormatCSV, Output: domain.FormatODS, Priority: 80},
			{Input: domain.FormatCSV, Output: domain.FormatPDF, Priority: 80},
			{Input: domain.FormatHTML, Output: domain.FormatPDF, Priority: 80},
			{Input: domain.FormatPPTX, Output: domain.FormatPDF, Priority: 80},
			{Input: domain.FormatPPTX, Output: domain.FormatHTML, Priority: 80},
		},
	}
}

func (c *LibreOffice) ID() string { return "libreoffice" }

func (c *LibreOffice) RequiredCommands() []string { return []string{"libreoffice"} }

func (c *LibreOffice) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *LibreOffice) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *LibreOffice) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	tmpDir, err := os.MkdirTemp("", "convert-libreoffice-*")
	if err != nil {
		return domain.ConversionResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	target := libreOfficeTarget(job.OutputFormat)
	args := []string{"--headless", "--convert-to", target, "--outdir", tmpDir, job.InputPath}
	result, err := c.runner.Run(ctx, ports.Command{Name: "libreoffice", Args: args})
	if err != nil {
		return domain.ConversionResult{}, commandError(result, err)
	}

	converted, err := findLibreOfficeOutput(tmpDir, job.InputPath, job.OutputFormat)
	if err != nil {
		return domain.ConversionResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0o755); err != nil {
		return domain.ConversionResult{}, err
	}
	if err := moveFile(converted, job.OutputPath); err != nil {
		return domain.ConversionResult{}, err
	}

	return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
}

func libreOfficeTarget(format domain.Format) string {
	if format == domain.FormatHTML {
		return "html"
	}
	return format.Extension()
}

func findLibreOfficeOutput(tmpDir string, input string, output domain.Format) (string, error) {
	ext := "." + output.Extension()
	expected := filepath.Join(tmpDir, strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))+ext)
	if _, err := os.Stat(expected); err == nil {
		return expected, nil
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			return filepath.Join(tmpDir, entry.Name()), nil
		}
	}

	return "", fmt.Errorf("libreoffice did not produce a %s file", output)
}
