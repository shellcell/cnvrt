# convert

`convert` is an interactive Go CLI for file conversion, extraction, archive creation, image resize/compression workflows, and developer-oriented format tooling.

The module path is:

```text
github.com/shellcell/convert
```

It uses clean layered architecture and delegates actual conversion work to external tools when appropriate. The CLI is interactive, but intentionally not a full-screen TUI. Interactive selectors use Huh v2, and reports are styled with Lip Gloss v2.

## Build

```bash
go build ./cmd/convert
```

This creates `./convert` in the current directory.

## Usage

```bash
./convert
./convert input.svg output.png
./convert input.svg
./convert -i svg -o png abc.svg cde.svg
./convert -o png
./convert -o png ../../
./convert -o zip ./directory
./convert archive.tar.gz
./convert --compress -o jpg photo.jpg
./convert --resize 800x -o png image.png
./convert -o json config.yaml
./convert -o yaml data.csv
./convert doctor
./convert formats
./convert backends
./convert add-format
./convert add-tool
```

## Behavior

- `./convert` lists files from the current directory, asks which inputs to use, then asks for an output format.
- Interactive file selection shows supported files and directories. `enter`/right opens directories, left goes up one level, `space`/`x` selects files or directories, `a` selects all filtered entries, `c` clears the selection, `/` filters, `esc` clears the filter, and `q` quits. Directory cursor/filter positions are preserved while navigating.
- Output format selection uses the same navigation style: up/down or j/k, `pageup`/`ctrl+up`/`option+up`, `pagedown`/`ctrl+down`/`option+down`, `gg`/`ctrl+a` to top, `G`/`shift+g`/`ctrl+e` to bottom, `/` filtering, `space`/`x`/enter to select, `esc` to clear the filter, and `q` to quit.
- Interactive selectors use the available terminal height and scroll only when options do not fit.
- Non-terminal stdin falls back to numeric prompts, so scripted usage remains simple.
- `./convert input output` converts immediately based on file extensions.
- `./convert -i svg -o png a.svg b.svg` converts a batch immediately.
- Generated output paths default to the current directory. Interactive mode asks whether to save in the current directory or beside each source.
- Files without an extension, or with an unknown unsupported extension, are treated as `txt` when their contents look like plain text.
- Batch mode is tolerant: supported files are converted, unsupported or failed files are reported at the end.
- A status report is always printed at the end.
- If an archive is provided as the only input, interactive mode offers extraction.
- If a directory is provided with an archive output format, it creates an archive.
- If input and output image formats match, interactive mode offers compress, resize, or convert/copy.
- Unavailable output formats stay visible in the selector, are greyed out with the missing dependency reason, and remain selectable. The conversion report prints install commands for missing tools.
- Missing dependencies include install suggestions based on the host and available package managers.

## Built-In Backends

| Backend | Command | Typical Use |
|---|---|---|
| structured | none | JSON, YAML, TOML, CSV, INI, XML, and PLIST conversions |
| archive | none | `zip`, `tar`, `tar.gz`, `tar.bz2` extraction/creation |
| resvg | `resvg` | `svg -> png` |
| ImageMagick | `magick` | common image conversions, resize, best-effort compression |
| FFmpeg | `ffmpeg` | video, audio, GIF, animated WebP |
| LibreOffice | `libreoffice` | office documents |
| Pandoc | `pandoc` | text, Markdown, HTML, EPUB, DOCX, TeX, RTF |
| Calibre | `ebook-convert` | EPUB, MOBI, AZW3, FB2, PDF e-book workflows |
| GDAL | `ogr2ogr` | geo/vector map formats |
| QEMU | `qemu-img` | VM and disk image formats |
| 7-Zip | `7z` | broad archive/package extraction |
| FontForge | `fontforge` | font conversions, including bitmap fonts |

Run:

```bash
./convert doctor
```

to see which backends are available locally and how to install missing ones.

## Format Scope

The built-in format list covers common images, SVG, PDFs, office documents, text, TeX, RTF, e-books, audio, video, archives, package archives, fonts, geo/map formats, API/schema formats, serialization formats, VM images, and disk images.

Known format does not mean every conversion pair is supported. Real support depends on backend capabilities and installed tools.

Structured-data conversions are built in and do not require external commands for `json`, `yaml`/`yml`, `toml`, `csv`, `ini`/`cfg`, `xml`, and `plist`. CSV input uses the first row as headers and converts to arrays of objects for structured outputs.

Examples of known categories:

- Images: `png`, `jpg`, `webp`, `bmp`, `tiff`, `gif`, `apng`, `avif`, `heic`, `ico`, `icns`, `psd`, `jp2`, `svg`, `pdf`
- Documents: `txt`, `md`, `html`, `rtf`, `tex`, `docx`, `xlsx`, `csv`, `odt`, `ods`, `pptx`, `pdf`
- E-books: `epub`, `fb2`, `mobi`, `azw3`, `djvu`
- Audio/video: `mp3`, `wav`, `flac`, `aac`, `m4a`, `ogg`, `opus`, `mp4`, `mov`, `avi`, `webm`, `mkv`, `flv`
- Archives/packages: `zip`, `tar`, `tar.gz`, `tar.bz2`, `tar.xz`, `7z`, `rar`, `deb`, `rpm`, `a`, `ar`, `cpio`, `jar`, `apk`, `ipa`, `whl`, `nupkg`, `gem`, `crate`
- Fonts: `ttf`, `otf`, `woff`, `woff2`, `eot`, `bdf`, `pcf`, `fon`, `pfa`, `pfb`
- Geo/maps: `geojson`, `topojson`, `kml`, `kmz`, `gpx`, `shp`, `gpkg`, `gml`, `osm`, `pbf`, `mbtiles`, `pmtiles`, `mvt`, `wkt`, `wkb`, `las`, `laz`
- Schemas/APIs: `proto`, `protoset`, `openapi`, `swagger`, `jsonschema`, `asyncapi`, `graphql`, `thrift`, `avsc`, `fbs`, `capnp`, `wsdl`, `xsd`
- VM/disk images: `raw`, `img`, `qcow2`, `qcow`, `qed`, `vdi`, `vmdk`, `vhd`, `vhdx`, `vpc`, `cow`, `dmg`, `iso`, `ova`, `ovf`, `vbox`, `hdd`, `box`

## Config-Driven Extension

New formats and tools can be added without code changes.

Interactive generators:

```bash
./convert add-format
./convert add-tool
```

Config files are loaded from:

```text
./convert.tools.json
./convert.tools.d/*.json
~/.config/convert/tools.json
~/.config/convert/tools.d/*.json
```

You can also set:

```bash
CONVERT_TOOL_CONFIG=/path/to/tool.json ./convert formats
```

Example tool config:

```json
{
  "formats": [
    {
      "name": "plantuml",
      "aliases": ["puml"],
      "extensions": ["puml", "plantuml"],
      "category": "diagram"
    }
  ],
  "tools": [
    {
      "id": "plantuml",
      "commands": ["plantuml"],
      "capabilities": [
        { "input": "plantuml", "output": "svg", "priority": 80 },
        { "input": "plantuml", "output": "png", "priority": 80 }
      ],
      "convert": {
        "command": "plantuml",
        "args": ["-t{output_format}", "-o", ".", "{input}"]
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

Supported install managers in config include `brew`, `apt`, `dnf`, `yum`, `pacman`, `zypper`, `apk`, `npm`, `pnpm`, `yarn`, `pipx`, `pip`, `go`, `cargo`, `gem`, and `composer`.

## Documentation

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the layered architecture and roadmap.
