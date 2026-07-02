package progressadapter

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/shellcell/convert/internal/domain"
	"golang.org/x/term"
)

type Reporter struct {
	out      io.Writer
	terminal bool

	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
	message string

	okStyle   lipgloss.Style
	failStyle lipgloss.Style
	skipStyle lipgloss.Style
	dimStyle  lipgloss.Style
}

func New(out io.Writer) *Reporter {
	return &Reporter{
		out:       out,
		terminal:  isTerminal(out),
		okStyle:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42")),
		failStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		skipStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")),
		dimStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	}
}

func (r *Reporter) Start(total int) {
	if total > 0 {
		fmt.Fprintf(r.out, "Converting %d job(s)\n", total)
	}
}

func (r *Reporter) JobStart(index int, total int, job domain.ConvertJob, backend string) {
	message := fmt.Sprintf("[%d/%d] %s -> %s (%s)", index, total, job.InputPath, job.OutputPath, backend)
	if !r.terminal {
		fmt.Fprintf(r.out, "RUN  %s\n", message)
		return
	}
	r.startSpinner(message)
}

func (r *Reporter) JobSuccess(index int, total int, job domain.ConvertJob, backend string) {
	message := fmt.Sprintf("[%d/%d] %s -> %s (%s)", index, total, job.InputPath, job.OutputPath, backend)
	r.stopSpinner()
	fmt.Fprintf(r.out, "%s %s\n", r.okStyle.Render("OK"), message)
}

func (r *Reporter) JobSkipped(index int, total int, input string, output string, message string) {
	r.stopSpinner()
	target := input
	if output != "" {
		target += " -> " + output
	}
	fmt.Fprintf(r.out, "%s [%d/%d] %s %s\n", r.skipStyle.Render("SKIP"), index, total, target, r.dimStyle.Render(message))
}

func (r *Reporter) JobFailed(index int, total int, job domain.ConvertJob, backend string, err error) {
	message := fmt.Sprintf("[%d/%d] %s", index, total, job.InputPath)
	if job.OutputPath != "" {
		message += " -> " + job.OutputPath
	}
	if backend != "" {
		message += " (" + backend + ")"
	}
	r.stopSpinner()
	fmt.Fprintf(r.out, "%s %s %s\n", r.failStyle.Render("FAIL"), message, r.dimStyle.Render(err.Error()))
}

func (r *Reporter) Finish() {
	r.stopSpinner()
}

func (r *Reporter) startSpinner(message string) {
	r.stopSpinner()
	r.mu.Lock()
	r.message = message
	r.stop = make(chan struct{})
	r.done = make(chan struct{})
	stop := r.stop
	done := r.done
	r.mu.Unlock()

	go func() {
		defer close(done)
		frames := []string{"-", "\\", "|", "/"}
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-stop:
				fmt.Fprint(r.out, "\r\033[2K")
				return
			case <-ticker.C:
				fmt.Fprintf(r.out, "\r\033[2K%s %s", frames[frame%len(frames)], message)
				frame++
			}
		}
	}()
}

func (r *Reporter) stopSpinner() {
	r.mu.Lock()
	stop := r.stop
	done := r.done
	r.stop = nil
	r.done = nil
	r.mu.Unlock()
	if stop != nil {
		close(stop)
		<-done
	}
}

func isTerminal(out io.Writer) bool {
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}
