# Architecture

`convert` uses clean layered architecture. The CLI and external tools are adapters. The application core owns conversion decisions, batch reporting, dependency checks, and output naming rules.

## Dependency Direction

```text
cmd/convert
  -> internal/bootstrap
  -> internal/adapters
  -> internal/app
  -> internal/ports
  -> internal/domain
```

Domain imports nothing from app, adapters, CLI, Huh, Lip Gloss, filesystem code, or external command code.

## Packages

```text
cmd/convert/main.go
internal/domain
internal/ports
internal/app
internal/adapters/cli
internal/adapters/prompt
internal/adapters/fs
internal/adapters/exec
internal/adapters/install
internal/adapters/toolconfig
internal/adapters/converters
internal/bootstrap
```

## Domain Layer

The domain layer defines stable conversion concepts:

- `Format`
- `FileRef`
- `ConvertJob`
- `ConvertOptions`
- `ConversionCapability`
- `ConversionResult`
- `TransformAction`
- `ArchiveAction`
- `ToolOptions`
- domain errors

Formats are open-ended. Built-ins are registered in code, but unknown normalized format names are accepted so config-defined tools can introduce new formats without recompilation.

## Application Layer

The app layer owns use cases and policies:

- interactive conversion flow after prompt choices are returned
- direct conversion flow
- tolerant batch conversion
- job building and validation
- output name generation
- same-format action handling
- archive extraction/compression routing
- backend selection
- command preview confirmation before a job starts
- dependency status, runtime availability, and install hints
- output format availability choices for interactive prompts
- final run report data

Important types:

- `Service`
- `ConvertRequest`
- `InteractiveRequest`
- `RunReport`
- `JobReport`
- `DependencyStatus`
- `FormatChoice`

Batch conversion does not abort on the first unsupported pair. Each input becomes a report item with status `converted`, `skipped`, or `failed`.

## Ports

Ports are interfaces owned by the core:

- `Converter`
- `FileDiscovery`
- `FileSystem`
- `Prompt`
- `CommandRunner`
- `InstallAdvisor`
- `ProgressReporter`
- `RuntimeDependencyAware`
- `CommandPreviewer`

Adapters implement these interfaces.

## Adapters

### CLI

The CLI adapter parses flags and commands, calls the application service, and renders final status reports.

Supported command shapes:

```bash
convert
convert input.svg output.png
convert input.svg
convert -i svg -o png abc.svg cde.svg
convert -o png
convert -o png ../../
convert -o zip ./directory
convert archive.tar.gz
convert add-format
convert add-tool
```

### Prompt

The prompt adapter uses Huh v2 for simple terminal prompts and small Bubble Tea models where custom selector behavior is needed. It keeps a numeric stdin/stdout fallback for non-terminal input. It is not a full-screen TUI.

Terminal controls:

- up/down or j/k moves through options
- pageup/ctrl+up/option+up and pagedown/ctrl+down/option+down move by one visible page
- gg/ctrl+a jumps to the beginning
- G/shift+g/ctrl+e jumps to the end
- space or x selects/toggles where selection is available
- enter/right opens directories in the file picker, left goes up one level, and enter confirms other selections
- `/` filters select and multi-select options
- esc clears the active filter
- q aborts the interactive prompt
- a selects all currently filtered file-picker entries
- c clears the current file-picker selection
- `CONVERT_ACCESSIBLE=1` enables Huh accessible mode

The file picker shows supported files and directories. Directories can be opened with enter and selected with space/x, which supports directory-to-archive workflows from the interactive picker.

The file picker preserves cursor, scroll offset, and filter text per visited directory, so going deeper and back restores the previous position.

The output format picker receives `FormatChoice` values from the application layer. Unavailable formats are still displayed and dimmed with the missing dependency reason, but remain selectable. If selected, conversion continues into normal backend selection so the final report can include install hints for missing tools.

Before each terminal job starts, the prompt adapter shows the selected backend and command preview. Option flags are split onto indented colored lines to avoid unreadable wrapping. Supported previews can choose which command is editable; edited commands run through the shell for that job. Non-terminal runs proceed without the confirmation prompt.

If a generated output path already exists and overwrite is not enabled, terminal mode asks for a different output path. Non-terminal mode keeps the strict failure behavior.

Interactive flow for `convert`:

```text
list files from current directory
-> select one or more inputs
-> select output format
-> select whether outputs go to current directory or source directories
-> optionally select same-format action
-> convert supported jobs
-> print report
```

Interactive flow for a single archive:

```text
detect archive
-> offer extract, choose output format, or cancel
```

Interactive same-format image flow:

```text
input format equals output format
-> offer compress, resize, or convert/copy
```

### Filesystem

The filesystem adapter lists directory entries, checks paths, detects directories, and creates output directories.

### Exec

The exec adapter wraps `os/exec`, captures stdout/stderr, and normalizes exit failures.

### Install Advisor

The install adapter suggests installation commands for missing converter tools.

Preference order:

- Use Homebrew if `brew` exists, even on Linux.
- Otherwise use the native package manager detected on the host.
- Fall back to configured ecosystem install hints or a manual hint.

Supported config managers:

```text
brew, apt, dnf, yum, pacman, zypper, apk,
npm, pnpm, yarn, pipx, pip, go, cargo, gem, composer
```

### Tool Config

The tool config adapter loads and writes JSON config files for additional formats and tools.

Load paths:

```text
./convert.tools.json
./convert.tools.d/*.json
~/.config/convert/tools.json
~/.config/convert/tools.d/*.json
CONVERT_TOOL_CONFIG paths
```

Interactive generators:

```bash
convert add-format
convert add-tool
```

Generated files are written to:

```text
~/.config/convert/tools.d/*.json
```

## Built-In Converter Adapters

Current built-ins:

- `archive`: stdlib zip/tar/tar.gz/tar.bz2 extraction and zip/tar/tar.gz creation
- `structured`: JSON, YAML, TOML, CSV, INI, XML, and PLIST conversions without external tools
- `resvg`: SVG rendering
- `animated-svg`: animated SVG rendering through a headless browser and FFmpeg
- `imagemagick`: broad image conversion, resize, and quality options
- `ffmpeg`: video, audio, GIF, animated images
- `libreoffice`: office documents
- `pandoc`: text, Markdown, HTML, RTF, TeX, DOCX, EPUB
- `calibre`: EPUB, MOBI, AZW3, FB2, e-book workflows
- `gdal`: geo/vector map conversions
- `qemu-img`: VM and disk images
- `7z`: broad archive/package extraction and 7z/zip creation
- `fontforge`: outline and bitmap font conversions

Config-defined converters are appended after built-ins.

Converters can implement `InputCapabilityAware` to expose capabilities based on a specific input file. The animated SVG backend uses this to show video/animation outputs only when an SVG contains animation markers.

Converters that execute external tools should implement `CommandPreviewer` so the app can show the backend command before execution. Converters with internal setup steps can implement `CommandOverrideConverter` to run an edited command inside the normal pipeline. Built-in in-process converters report that no external command is used.

## Backend Selection

The app selects the first backend that:

- declares a matching `input -> output` capability
- has all required commands available on `PATH`
- has runtime sub-dependencies available when the backend declares them, such as Pandoc PDF engines

If no backend supports a pair, the job is `skipped`.

If a backend supports the pair but dependencies are missing, the job is `failed` with install hints.

## Input Detection

Input format detection prefers explicit `-i/--input-format`, then directories, then registered file extensions. If a file has no extension or only an unknown unsupported extension and its content looks like text, it is treated as `txt`. Custom formats with declared converter capabilities are preserved.

## Converter Options

Interactive converter options should default to not changing source parameters when the option is optional. SVG raster/video output size follows this rule: the user is first asked whether to set a size. If skipped, converters preserve source dimensions when possible. If enabled, the input defaults to source dimensions from SVG `width`/`height`, SVG `viewBox`, or readable raster image metadata, falling back to `1024x1024`.

## Output Naming

For direct conversion:

```bash
convert input.svg output.png
```

the explicit output path is used.

For format-based conversion:

```bash
convert -o png abc.svg
```

the default output is written to the current directory:

```text
abc.png
```

Interactive mode asks whether generated outputs should be saved in the current directory or beside each source. Direct non-interactive format-based conversion uses the current directory unless `--out-dir` is set.

For same-format image actions:

```text
photo.jpg -> photo.compressed.jpg
photo.jpg -> photo.resized.jpg
```

For archive extraction:

```text
archive.tar.gz -> archive/
```

For directory archive creation:

```bash
convert -o zip ./dist
```

the default output is:

```text
dist.zip
```

Existing outputs are rejected unless `--overwrite` is set.

## Config Schema

Example:

```json
{
  "formats": [
    {
      "name": "plantuml",
      "aliases": ["puml"],
      "extensions": ["puml"],
      "category": "diagram"
    }
  ],
  "tools": [
    {
      "id": "plantuml",
      "commands": ["plantuml"],
      "capabilities": [
        { "input": "plantuml", "output": "svg", "priority": 80 }
      ],
      "convert": {
        "command": "plantuml",
        "args": ["-tsvg", "{input}"]
      },
      "install": [
        { "manager": "brew", "package": "plantuml" },
        { "manager": "apt", "package": "plantuml" },
        { "manager": "npm", "package": "node-plantuml" }
      ]
    }
  ]
}
```

Command placeholders:

```text
{input}
{output}
{input_format}
{output_format}
{quality}
{resize}
{action}
```

## Format Strategy

The built-in format catalog is intentionally broad because this tool targets developers and automation-heavy workflows.

Categories include:

- images and vectors
- documents and office files
- e-books and scanned documents
- audio and video
- archives and package archives
- fonts, including bitmap fonts
- geo/map formats
- serialization/data formats
- API/schema formats
- VM and disk images

The important rule is that format recognition and conversion capability are separate. A format can be known without having a currently installed backend.

## Roadmap

Phase 1 completed:

- Go module under `github.com/shellcell/convert`
- clean architecture skeleton
- interactive CLI with Huh v2 prompts and Lip Gloss v2 status reports
- direct conversion mode
- tolerant batch conversion mode
- final status reports
- archive extraction and directory archive creation
- same-format image action prompts
- dependency install hints
- config-defined formats and tools
- interactive config generators

Next phases:

- dedicated compressor port and compressor adapters
- recursive discovery
- dry-run conversion plans
- integration tests for installed tools
- config validation command
- richer shell-template support for config-defined tools
- browser backend for HTML screenshots/PDF
- OCR for scanned PDFs (rasterize + tesseract, or ocrmypdf)
- mouse click selection in pickers (needs reliable view-relative mouse
  coordinates in inline mode; wheel scrolling already works)
- piper/whisper.cpp engines for TTS/STT alongside say/espeak-ng and
  openai-whisper
