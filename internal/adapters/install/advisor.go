package installadapter

import (
	"os/exec"
	"runtime"
	"sort"

	"github.com/shellcell/convert/internal/ports"
)

type Advisor struct {
	configured map[string][]ports.InstallSuggestion
}

func NewAdvisor(configured map[string][]ports.InstallSuggestion) *Advisor {
	return &Advisor{configured: configured}
}

func (a *Advisor) Suggestions(command string) []ports.InstallSuggestion {
	manager := preferredManager()
	var suggestions []ports.InstallSuggestion

	if configured := a.configured[command]; len(configured) > 0 {
		suggestions = append(suggestions, filterSuggestions(configured, manager)...)
	}

	if pkg := packageName(command, manager); pkg != "" {
		suggestions = append(suggestions, ports.InstallSuggestion{
			Manager: manager,
			Package: pkg,
			Command: installCommand(manager, pkg),
		})
	}

	if len(suggestions) == 0 {
		if pkg := genericPackageName(command); pkg != "" {
			suggestions = append(suggestions, ports.InstallSuggestion{
				Manager: "manual",
				Package: pkg,
				Command: "install " + pkg + " with your package manager",
			})
		}
	}

	return dedupeSuggestions(suggestions)
}

func preferredManager() string {
	if _, err := exec.LookPath("brew"); err == nil {
		return "brew"
	}

	if runtime.GOOS == "darwin" {
		return "brew"
	}

	for _, manager := range []string{"apt", "dnf", "yum", "pacman", "zypper", "apk"} {
		if _, err := exec.LookPath(manager); err == nil {
			return manager
		}
	}

	return "manual"
}

func filterSuggestions(suggestions []ports.InstallSuggestion, manager string) []ports.InstallSuggestion {
	var exact []ports.InstallSuggestion
	var fallback []ports.InstallSuggestion
	for _, suggestion := range suggestions {
		if suggestion.Manager == manager {
			exact = append(exact, suggestion)
			continue
		}
		fallback = append(fallback, suggestion)
	}
	if len(exact) > 0 {
		return exact
	}
	return fallback
}

func packageName(command string, manager string) string {
	packages := map[string]map[string]string{
		"ffmpeg": {
			"brew": "ffmpeg", "apt": "ffmpeg", "dnf": "ffmpeg", "yum": "ffmpeg", "pacman": "ffmpeg", "zypper": "ffmpeg", "apk": "ffmpeg",
		},
		"chromium": {
			"brew": "chromium", "apt": "chromium-browser", "dnf": "chromium", "yum": "chromium", "pacman": "chromium", "zypper": "chromium", "apk": "chromium",
		},
		"magick": {
			"brew": "imagemagick", "apt": "imagemagick", "dnf": "ImageMagick", "yum": "ImageMagick", "pacman": "imagemagick", "zypper": "ImageMagick", "apk": "imagemagick",
		},
		"resvg": {
			"brew": "resvg", "apt": "resvg", "dnf": "resvg", "yum": "resvg", "pacman": "resvg", "zypper": "resvg", "apk": "resvg",
		},
		"libreoffice": {
			"brew": "libreoffice", "apt": "libreoffice", "dnf": "libreoffice", "yum": "libreoffice", "pacman": "libreoffice-fresh", "zypper": "libreoffice", "apk": "libreoffice",
		},
		"pandoc": {
			"brew": "pandoc", "apt": "pandoc", "dnf": "pandoc", "yum": "pandoc", "pacman": "pandoc-cli", "zypper": "pandoc", "apk": "pandoc",
		},
		"tectonic": {
			"brew": "tectonic", "apt": "tectonic", "dnf": "tectonic", "yum": "tectonic", "pacman": "tectonic", "zypper": "tectonic", "apk": "tectonic",
		},
		"typst": {
			"brew": "typst", "apt": "typst", "dnf": "typst", "yum": "typst", "pacman": "typst", "zypper": "typst", "apk": "typst",
		},
		"pdflatex": {
			"brew": "basictex", "apt": "texlive-latex-base", "dnf": "texlive-latex", "yum": "texlive-latex", "pacman": "texlive-basic", "zypper": "texlive-latex", "apk": "texlive-full",
		},
		"ebook-convert": {
			"brew": "calibre", "apt": "calibre", "dnf": "calibre", "yum": "calibre", "pacman": "calibre", "zypper": "calibre", "apk": "calibre",
		},
		"ogr2ogr": {
			"brew": "gdal", "apt": "gdal-bin", "dnf": "gdal", "yum": "gdal", "pacman": "gdal", "zypper": "gdal", "apk": "gdal-tools",
		},
		"7z": {
			"brew": "p7zip", "apt": "p7zip-full", "dnf": "p7zip", "yum": "p7zip", "pacman": "p7zip", "zypper": "p7zip", "apk": "p7zip",
		},
		"qemu-img": {
			"brew": "qemu", "apt": "qemu-utils", "dnf": "qemu-img", "yum": "qemu-img", "pacman": "qemu-base", "zypper": "qemu-tools", "apk": "qemu-img",
		},
		"fontforge": {
			"brew": "fontforge", "apt": "fontforge", "dnf": "fontforge", "yum": "fontforge", "pacman": "fontforge", "zypper": "fontforge", "apk": "fontforge",
		},
		"protoc": {
			"brew": "protobuf", "apt": "protobuf-compiler", "dnf": "protobuf-compiler", "yum": "protobuf-compiler", "pacman": "protobuf", "zypper": "protobuf-devel", "apk": "protobuf-dev",
		},
		"buf": {
			"brew": "bufbuild/buf/buf", "apt": "buf", "dnf": "buf", "yum": "buf", "pacman": "buf", "zypper": "buf", "apk": "buf",
		},
		"openapi-generator-cli": {
			"brew": "openapi-generator", "apt": "openapi-generator-cli", "dnf": "openapi-generator", "yum": "openapi-generator", "pacman": "openapi-generator", "zypper": "openapi-generator", "apk": "openapi-generator", "npm": "@openapitools/openapi-generator-cli",
		},
	}

	if byManager, ok := packages[command]; ok {
		if pkg, ok := byManager[manager]; ok {
			return pkg
		}
		return genericPackageName(command)
	}

	return ""
}

func genericPackageName(command string) string {
	switch command {
	case "magick":
		return "imagemagick"
	case "ebook-convert":
		return "calibre"
	case "ogr2ogr":
		return "gdal"
	default:
		return command
	}
}

func installCommand(manager string, pkg string) string {
	switch manager {
	case "brew":
		if pkg == "basictex" || pkg == "mactex" || pkg == "chromium" || pkg == "google-chrome" {
			return "brew install --cask " + pkg
		}
		return "brew install " + pkg
	case "apt":
		return "sudo apt install " + pkg
	case "dnf":
		return "sudo dnf install " + pkg
	case "yum":
		return "sudo yum install " + pkg
	case "pacman":
		return "sudo pacman -S " + pkg
	case "zypper":
		return "sudo zypper install " + pkg
	case "apk":
		return "sudo apk add " + pkg
	case "npm":
		return "npm install -g " + pkg
	case "pnpm":
		return "pnpm add -g " + pkg
	case "yarn":
		return "yarn global add " + pkg
	case "pipx":
		return "pipx install " + pkg
	case "pip":
		return "python -m pip install " + pkg
	case "go":
		return "go install " + pkg
	case "cargo":
		return "cargo install " + pkg
	case "gem":
		return "gem install " + pkg
	case "composer":
		return "composer global require " + pkg
	default:
		return "install " + pkg + " with your package manager"
	}
}

func dedupeSuggestions(suggestions []ports.InstallSuggestion) []ports.InstallSuggestion {
	seen := map[string]bool{}
	result := make([]ports.InstallSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		key := suggestion.Manager + "\x00" + suggestion.Command
		if seen[key] || suggestion.Command == "" {
			continue
		}
		seen[key] = true
		result = append(result, suggestion)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Manager < result[j].Manager })
	return result
}
