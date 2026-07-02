package execadapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/shellcell/convert/internal/ports"
)

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}

func (r *Runner) Run(ctx context.Context, command ports.Command) (ports.CommandResult, error) {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Dir = command.Dir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := ports.CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, fmt.Errorf("exit code %d", result.ExitCode)
	}

	return result, err
}
