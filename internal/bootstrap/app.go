package bootstrap

import (
	"context"
	"fmt"
	"io"

	"github.com/shellcell/convert/internal/adapters/cli"
	"github.com/shellcell/convert/internal/adapters/converters"
	execadapter "github.com/shellcell/convert/internal/adapters/exec"
	fsadapter "github.com/shellcell/convert/internal/adapters/fs"
	installadapter "github.com/shellcell/convert/internal/adapters/install"
	progressadapter "github.com/shellcell/convert/internal/adapters/progress"
	promptadapter "github.com/shellcell/convert/internal/adapters/prompt"
	"github.com/shellcell/convert/internal/adapters/settings"
	"github.com/shellcell/convert/internal/adapters/toolconfig"
	"github.com/shellcell/convert/internal/app"
	"github.com/shellcell/convert/internal/ports"
)

type App struct{}

func New() *App {
	return &App{}
}

func (a *App) Run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	runner := execadapter.NewRunner()
	fs := fsadapter.NewFileSystem()
	discovery := fsadapter.NewDiscovery()
	preferences, palette, uiOptions, err := settings.Load()
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not load settings: %v\n", err)
	}
	prompt := promptadapter.New(stdin, stdout, palette)
	prompt.SetShowHelp(uiOptions.ShowHelp)
	progress := progressadapter.New(stdout, palette)
	configured, err := toolconfig.Load(runner)
	if err != nil {
		fmt.Fprintf(stderr, "warning: could not load tool config: %v\n", err)
	}
	advisor := installadapter.NewAdvisor(configured.InstallHints)

	converterList := []ports.Converter{
		converters.NewStructuredData(),
		converters.NewArchive(),
		converters.NewResvg(runner),
		converters.NewAnimatedSVG(runner),
		converters.NewFFmpeg(runner),
		converters.NewLibreOffice(runner),
		converters.NewCalibre(runner),
		converters.NewGDAL(runner),
		converters.NewQemuImg(runner),
		converters.NewSevenZip(runner),
		converters.NewFontForge(runner),
		converters.NewPandoc(runner),
		converters.NewInkscape(runner),
		converters.NewGhostscript(runner),
		converters.NewDjVuLibre(runner),
		converters.NewGraphviz(runner),
		converters.NewMermaid(runner),
		converters.NewJupyter(runner),
		converters.NewPoppler(runner),
		converters.NewTesseract(runner),
		converters.NewTTS(runner),
		converters.NewWhisper(runner),
		converters.NewImageMagick(runner),
	}
	converterList = append(converterList, configured.Converters...)

	service := app.NewService(converterList, discovery, fs, prompt, runner, advisor, preferences, progress)
	cliRunner := cli.NewRunner(service, fs, stdin, stdout, stderr, palette)
	return cliRunner.Run(ctx, args)
}
