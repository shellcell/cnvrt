package converters

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

func hasCapability(capabilities []domain.ConversionCapability, input domain.Format, output domain.Format) bool {
	for _, capability := range capabilities {
		if capability.Input == input && capability.Output == output {
			return true
		}
	}
	return false
}

func capabilities(inputs []domain.Format, outputs []domain.Format, priority int, lossy bool, preservesAnimation bool) []domain.ConversionCapability {
	result := make([]domain.ConversionCapability, 0, len(inputs)*len(outputs))
	for _, input := range inputs {
		for _, output := range outputs {
			result = append(result, domain.ConversionCapability{
				Input:              input,
				Output:             output,
				Priority:           priority,
				Lossy:              lossy,
				PreservesAnimation: preservesAnimation,
			})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Input == result[j].Input {
			return result[i].Output < result[j].Output
		}
		return result[i].Input < result[j].Input
	})
	return result
}

func qualityArgs(job domain.ConvertJob) []string {
	if job.Options.Quality <= 0 {
		return nil
	}
	return []string{"-quality", strconv.Itoa(job.Options.Quality)}
}

func commandError(result ports.CommandResult, err error) error {
	if err == nil {
		return nil
	}
	if result.Stderr != "" {
		return fmt.Errorf("%w: %s", err, result.Stderr)
	}
	if result.Stdout != "" {
		return fmt.Errorf("%w: %s", err, result.Stdout)
	}
	return err
}

func runSimple(ctx context.Context, runner ports.CommandRunner, command string, args []string, job domain.ConvertJob, backend string) (domain.ConversionResult, error) {
	result, err := runner.Run(ctx, ports.Command{Name: command, Args: args})
	if err != nil {
		return domain.ConversionResult{}, commandError(result, err)
	}

	return domain.ConversionResult{Job: job, Backend: backend, OutputPath: job.OutputPath}, nil
}

func moveFile(from string, to string) error {
	if err := os.Rename(from, to); err == nil {
		return nil
	}

	input, err := os.Open(from)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}

	output, err := os.Create(to)
	if err != nil {
		return err
	}
	defer output.Close()

	if _, err := io.Copy(output, input); err != nil {
		return err
	}

	return os.Remove(from)
}
