# cnvrt

`cnvrt` is an interactive Go CLI for file conversion, extraction, archive creation, image resize/compression, and developer-oriented format tooling.

Run it with no arguments to pick files and an output format interactively, or pass paths and flags to convert straight away. Conversions are delegated to external tools when appropriate; structured data and archives are handled in-process with no dependencies.

```sh
# From source (Go 1.26+)
go install github.com/shellcell/cnvrt/cmd/cnvrt@latest

# Homebrew
brew install shellcell/tap/cnvrt
```

Build from a clone with `make build` or `make build-debug` (unstripped). Both produce `./cnvrt`.

## Usage

```bash
cnvrt                                       # pick inputs and output format interactively
cnvrt input.svg output.png                  # direct, by explicit path
cnvrt input.svg                             # pick the output format for one file
cnvrt -i svg -o png a.svg b.svg             # batch, by format
cnvrt -o png ../../                         # every supported file in a directory
cnvrt -o zip ./directory                    # create an archive
cnvrt archive.tar.gz                        # extract an archive
cnvrt --compress -o jpg photo.jpg           # same-format compress
cnvrt --resize 800x -o png image.png        # same-format resize
cnvrt -o json config.yaml                   # structured data, no external tool
cnvrt -o mp3 video.mp4                      # extract audio
cnvrt --opt ffmpeg.audio_bitrate=320k -o mp3 song.wav
```

### Commands

| Command | Purpose |
|---|---|
| `doctor` | Which backends are installed, what they are for, how to install the missing ones |
| `formats` | Known formats, grouped by category |
| `backends` | Each backend's description, required commands, and supported pairs |
| `config` | Edit and save user settings interactively |
| `add-format` / `add-tool` | Register a new format or converter tool via config |

### Flags

| Flag | Purpose |
|---|---|
| `-i`, `--input-format` | Override the detected input format |
| `-o`, `--output-format` | Output format |
| `--out-dir` | Output directory |
| `--overwrite` | Overwrite existing output files |
| `--quality` | Best-effort quality value for supported backends |
| `--compress` | Compress same-format image output |
| `--resize` | Resize value for supported image and video backends, e.g. `800x`, `50%` |
| `--action` | Same-format action: `convert`, `compress`, or `resize` |
| `--opt` | Backend option as `tool.key=value`, repeatable |
| `-v`, `--version` | Print version and exit (also the `version` command) |

## How It Works

- **Output** goes to the current directory by default; interactive mode offers to write beside each source instead.
- **Batches are tolerant.** Supported files convert; unsupported and failed ones are listed in the final report, along with the exact command used for each job.
- **Backends are chosen by rank,** but when several can handle a pair, interactive mode asks once per pair. Pin a choice with the `backends` setting to skip the question.
- **Commands are previewed** before they run, and a single job's command can be edited. Non-terminal runs never prompt.
- **Missing tools don't hide formats.** Unavailable outputs stay selectable, greyed out with the reason, and the report prints install commands for your host.
- **Prompts adapt to the input:** extraction for a lone archive, archive creation for a directory, compress/resize/copy when input and output formats match, video and animation outputs for animated SVGs.
- **Backend options** (audio bitrate, GIF fps, OCR language, 7z level, …) are offered interactively and scriptable via `--opt`.
- **Selectors are inline,** not a full-screen TUI: `j`/`k` or arrows to move, `/` to filter, `space` to select, `s` to sort, `q` to quit. Non-terminal stdin falls back to numeric prompts.

## Backends

| Backend | Command | Typical Use |
|---|---|---|
| structured | *(none)* | JSON, JSONL, YAML, TOML, CSV, TSV, INI, ENV, XML, PLIST, plus plain-text/Markdown rendering |
| archive | *(none)* | `zip`, `tar`, `tar.gz`, `tar.bz2` extraction/creation |
| resvg | `resvg` | `svg -> png` |
| animated-svg | `ffmpeg` + browser | animated SVG to video/animation formats |
| ImageMagick | `magick` | common and legacy image conversions, resize, compression, background fill |
| FFmpeg | `ffmpeg` | video, audio, GIF with palette, audio extraction from video |
| LibreOffice | `libreoffice` | office documents |
| Pandoc | `pandoc` | text, Markdown, HTML, EPUB, DOCX, TeX, RTF, RST, Org |
| Calibre | `ebook-convert` | EPUB, MOBI, AZW3, FB2, PDF e-book workflows |
| GDAL | `ogr2ogr` | geo/vector map formats |
| QEMU | `qemu-img` | VM and disk image formats |
| 7-Zip | `7z` | broad archive/package extraction, compression level control |
| FontForge | `fontforge` | font conversions, including outline-to-bitmap (bdf/pcf/fon) |
| Inkscape | `inkscape` | `svg -> pdf/eps/ps`, png fallback |
| Ghostscript | `gs` | `ps/eps -> pdf`, `pdf -> ps`, PDF re-compression |
| DjVuLibre | `ddjvu`/`djvutxt` | `djvu -> pdf/tiff/txt` |
| Poppler | `pdftotext` | PDF text layer extraction to txt/html |
| Tesseract | `tesseract` | OCR: image -> txt or searchable PDF |
| Graphviz | `dot` | DOT diagrams to svg/png/pdf |
| Mermaid | `mmdc` | Mermaid diagrams to svg/png/pdf |
| Jupyter | `jupyter` | notebooks to html/markdown/script/pdf |
| TTS | `say`/`espeak-ng` | text to speech: txt/md -> wav/aiff/m4a |
| Whisper | `whisper` | speech to text: audio/video -> txt |

Run `cnvrt doctor` to see what is available locally.

## Formats

A known format is not the same as a supported conversion — real support depends on which backends are installed. Run `cnvrt formats` for the full catalog and `cnvrt backends` for actual pairs.

| Category | Examples |
|---|---|
| Images | `png`, `jpg`, `webp`, `bmp`, `tiff`, `gif`, `apng`, `avif`, `heic`, `ico`, `icns`, `psd`, `jp2`, `svg`, `pdf` |
| Documents | `txt`, `md`, `html`, `rtf`, `tex`, `docx`, `xlsx`, `csv`, `odt`, `ods`, `pptx`, `pdf` |
| E-books | `epub`, `fb2`, `mobi`, `azw3`, `djvu` |
| Audio/video | `mp3`, `wav`, `flac`, `aac`, `m4a`, `ogg`, `opus`, `mp4`, `mov`, `avi`, `webm`, `mkv`, `flv` |
| Archives/packages | `zip`, `tar`, `tar.gz`, `7z`, `rar`, `deb`, `rpm`, `cpio`, `jar`, `apk`, `whl`, `gem`, `crate` |
| Fonts | `ttf`, `otf`, `woff`, `woff2`, `eot`, `bdf`, `pcf`, `fon`, `pfa`, `pfb` |
| Geo/maps | `geojson`, `kml`, `gpx`, `shp`, `gpkg`, `osm`, `mbtiles`, `pmtiles`, `wkt`, `las` |
| Schemas/APIs | `proto`, `openapi`, `jsonschema`, `graphql`, `thrift`, `avsc`, `capnp`, `xsd` |
| VM/disk images | `raw`, `img`, `qcow2`, `vdi`, `vmdk`, `vhd`, `dmg`, `iso`, `ova` |

Structured-data conversions (`json`, `yaml`, `toml`, `csv`, `ini`, `xml`, `plist`, and friends) are built in and need no external commands. CSV input uses the first row as headers.

## Settings

`cnvrt config` writes `~/.config/cnvrt/settings.json`. A project-local `./cnvrt.settings.json` and any paths in `CNVRT_SETTINGS` are layered on top.

```json
{
  "theme": "nord",
  "ui": { "show_help": true },
  "backends": { "svg->png": "resvg", "pdf": "ghostscript" },
  "tools": { "ffmpeg": { "audio_bitrate": "320k" } }
}
```

`backends` pins a preferred backend per pair or per input format, `tools` sets default backend options, `ui.show_help` toggles the key-hint lines, and `theme` is `nord` or `classic`.

## Extending With Config

New formats and tools can be added without code changes — `cnvrt add-format` and `cnvrt add-tool` generate the JSON interactively.

Tool config is loaded from `./cnvrt.tools.json`, `./cnvrt.tools.d/*.json`, `~/.config/cnvrt/tools.json`, `~/.config/cnvrt/tools.d/*.json`, and any paths in `CNVRT_TOOL_CONFIG`.

```json
{
  "formats": [
    { "name": "plantuml", "aliases": ["puml"], "extensions": ["puml"], "category": "diagram" }
  ],
  "tools": [
    {
      "id": "plantuml",
      "commands": ["plantuml"],
      "capabilities": [{ "input": "plantuml", "output": "svg", "priority": 80 }],
      "convert": { "command": "plantuml", "args": ["-tsvg", "{input}"] },
      "install": [{ "manager": "brew", "package": "plantuml" }]
    }
  ]
}
```

Placeholders: `{input}`, `{output}`, `{input_format}`, `{output_format}`, `{quality}`, `{resize}`, `{action}`. Install managers: `brew`, `apt`, `dnf`, `yum`, `pacman`, `zypper`, `apk`, `npm`, `pnpm`, `yarn`, `pipx`, `pip`, `go`, `cargo`, `gem`, `composer`.

## Documentation

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the layered architecture, ports, backend selection rules, and roadmap.
