package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/shellcell/cnvrt/internal/bootstrap"
)

// version is the release tag (vX.Y.Z), injected at build time via
// -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "version", "-v", "-version", "--version":
			fmt.Printf("cnvrt %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
			return
		}
	}
	app := bootstrap.New()
	os.Exit(app.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
