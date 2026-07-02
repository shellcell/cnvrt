package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type Format string

const (
	FormatPNG         Format = "png"
	FormatJPEG        Format = "jpeg"
	FormatWebP        Format = "webp"
	FormatBMP         Format = "bmp"
	FormatTIFF        Format = "tiff"
	FormatGIF         Format = "gif"
	FormatAPNG        Format = "apng"
	FormatAVIF        Format = "avif"
	FormatHEIC        Format = "heic"
	FormatICO         Format = "ico"
	FormatICNS        Format = "icns"
	FormatPSD         Format = "psd"
	FormatJP2         Format = "jp2"
	FormatXPM         Format = "xpm"
	FormatXBM         Format = "xbm"
	FormatPNM         Format = "pnm"
	FormatPPM         Format = "ppm"
	FormatPGM         Format = "pgm"
	FormatPBM         Format = "pbm"
	FormatTGA         Format = "tga"
	FormatDDS         Format = "dds"
	FormatEXR         Format = "exr"
	FormatQOI         Format = "qoi"
	FormatSVG         Format = "svg"
	FormatPDF         Format = "pdf"
	FormatPS          Format = "ps"
	FormatEPS         Format = "eps"
	FormatTXT         Format = "txt"
	FormatMD          Format = "md"
	FormatRST         Format = "rst"
	FormatORG         Format = "org"
	FormatHTML        Format = "html"
	FormatRTF         Format = "rtf"
	FormatTEX         Format = "tex"
	FormatODT         Format = "odt"
	FormatODS         Format = "ods"
	FormatPPTX        Format = "pptx"
	FormatDOCX        Format = "docx"
	FormatXLSX        Format = "xlsx"
	FormatCSV         Format = "csv"
	FormatEPUB        Format = "epub"
	FormatFB2         Format = "fb2"
	FormatMOBI        Format = "mobi"
	FormatAZW3        Format = "azw3"
	FormatDJVU        Format = "djvu"
	FormatMP4         Format = "mp4"
	FormatMOV         Format = "mov"
	FormatAVI         Format = "avi"
	FormatWebM        Format = "webm"
	FormatMKV         Format = "mkv"
	FormatM4V         Format = "m4v"
	FormatMPG         Format = "mpg"
	FormatMPEG        Format = "mpeg"
	FormatFLV         Format = "flv"
	FormatOGV         Format = "ogv"
	FormatMP3         Format = "mp3"
	FormatWAV         Format = "wav"
	FormatFLAC        Format = "flac"
	FormatAAC         Format = "aac"
	FormatM4A         Format = "m4a"
	FormatOGG         Format = "ogg"
	FormatOPUS        Format = "opus"
	FormatWMA         Format = "wma"
	FormatAIFF        Format = "aiff"
	FormatZIP         Format = "zip"
	FormatTAR         Format = "tar"
	FormatTGZ         Format = "tar.gz"
	FormatTBZ2        Format = "tar.bz2"
	FormatTXZ         Format = "tar.xz"
	FormatTZST        Format = "tar.zst"
	FormatGZ          Format = "gz"
	FormatBZ2         Format = "bz2"
	FormatXZ          Format = "xz"
	FormatZST         Format = "zst"
	Format7Z          Format = "7z"
	FormatRAR         Format = "rar"
	FormatDEB         Format = "deb"
	FormatRPM         Format = "rpm"
	FormatAR          Format = "ar"
	FormatCPIO        Format = "cpio"
	FormatISO         Format = "iso"
	FormatJAR         Format = "jar"
	FormatWAR         Format = "war"
	FormatEAR         Format = "ear"
	FormatAPK         Format = "apk"
	FormatAAR         Format = "aar"
	FormatIPA         Format = "ipa"
	FormatWHL         Format = "whl"
	FormatEGG         Format = "egg"
	FormatNUPKG       Format = "nupkg"
	FormatVSIX        Format = "vsix"
	FormatXPI         Format = "xpi"
	FormatGem         Format = "gem"
	FormatCrate       Format = "crate"
	FormatArchPackage Format = "pkg.tar.zst"
	FormatTTF         Format = "ttf"
	FormatOTF         Format = "otf"
	FormatWOFF        Format = "woff"
	FormatWOFF2       Format = "woff2"
	FormatEOT         Format = "eot"
	FormatBDF         Format = "bdf"
	FormatPCF         Format = "pcf"
	FormatFON         Format = "fon"
	FormatPFA         Format = "pfa"
	FormatPFB         Format = "pfb"
	FormatJSON        Format = "json"
	FormatJSONL       Format = "jsonl"
	FormatYAML        Format = "yaml"
	FormatXML         Format = "xml"
	FormatTOML        Format = "toml"
	FormatINI         Format = "ini"
	FormatENV         Format = "env"
	FormatPLIST       Format = "plist"
	FormatTSV         Format = "tsv"
	FormatDOT         Format = "dot"
	FormatMermaid     Format = "mmd"
	FormatIPYNB       Format = "ipynb"
	FormatPY          Format = "py"
	FormatSQL         Format = "sql"
	FormatSQLite      Format = "sqlite"
	FormatParquet     Format = "parquet"
	FormatAvro        Format = "avro"
	FormatORC         Format = "orc"
	FormatArrow       Format = "arrow"
	FormatFeather     Format = "feather"
	FormatBSON        Format = "bson"
	FormatMsgpack     Format = "msgpack"
	FormatCBOR        Format = "cbor"
	FormatGeoJSON     Format = "geojson"
	FormatTopoJSON    Format = "topojson"
	FormatKML         Format = "kml"
	FormatKMZ         Format = "kmz"
	FormatGPX         Format = "gpx"
	FormatSHP         Format = "shp"
	FormatGPKG        Format = "gpkg"
	FormatGML         Format = "gml"
	FormatOSM         Format = "osm"
	FormatPBF         Format = "pbf"
	FormatMBTiles     Format = "mbtiles"
	FormatPMTiles     Format = "pmtiles"
	FormatMVT         Format = "mvt"
	FormatWKT         Format = "wkt"
	FormatWKB         Format = "wkb"
	FormatLAS         Format = "las"
	FormatLAZ         Format = "laz"
	FormatHGT         Format = "hgt"
	FormatOpenAPI     Format = "openapi"
	FormatSwagger     Format = "swagger"
	FormatJSONSchema  Format = "jsonschema"
	FormatAsyncAPI    Format = "asyncapi"
	FormatGraphQL     Format = "graphql"
	FormatProto       Format = "proto"
	FormatProtoSet    Format = "protoset"
	FormatThrift      Format = "thrift"
	FormatAvroSchema  Format = "avsc"
	FormatFlatBuffers Format = "fbs"
	FormatCapnp       Format = "capnp"
	FormatWSDL        Format = "wsdl"
	FormatXSD         Format = "xsd"
	FormatRAW         Format = "raw"
	FormatIMG         Format = "img"
	FormatQCOW2       Format = "qcow2"
	FormatQCOW        Format = "qcow"
	FormatQED         Format = "qed"
	FormatVDI         Format = "vdi"
	FormatVMDK        Format = "vmdk"
	FormatVHD         Format = "vhd"
	FormatVHDX        Format = "vhdx"
	FormatVPC         Format = "vpc"
	FormatCOW         Format = "cow"
	FormatDMG         Format = "dmg"
	FormatOVA         Format = "ova"
	FormatOVF         Format = "ovf"
	FormatVBox        Format = "vbox"
	FormatHDD         Format = "hdd"
	FormatVagrantBox  Format = "box"
	FormatDir         Format = "directory"
)

var (
	ErrUnknownFormat      = errors.New("unknown format")
	ErrUnsupportedConvert = errors.New("unsupported conversion")
	ErrMissingDependency  = errors.New("missing converter dependency")
	ErrInvalidJob         = errors.New("invalid conversion job")
)

type MissingDependencyError struct {
	Message  string
	Commands []string
}

func (e MissingDependencyError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if len(e.Commands) == 0 {
		return ErrMissingDependency.Error()
	}
	return fmt.Sprintf("%s: %s", ErrMissingDependency, strings.Join(e.Commands, ", "))
}

func (e MissingDependencyError) Unwrap() error {
	return ErrMissingDependency
}

var aliases = map[string]Format{
	"png":         FormatPNG,
	"jpeg":        FormatJPEG,
	"jpg":         FormatJPEG,
	"webp":        FormatWebP,
	"bmp":         FormatBMP,
	"tif":         FormatTIFF,
	"tiff":        FormatTIFF,
	"gif":         FormatGIF,
	"giff":        FormatGIF,
	"apng":        FormatAPNG,
	"avif":        FormatAVIF,
	"heic":        FormatHEIC,
	"heif":        FormatHEIC,
	"ico":         FormatICO,
	"icns":        FormatICNS,
	"psd":         FormatPSD,
	"jp2":         FormatJP2,
	"jpeg2000":    FormatJP2,
	"xpm":         FormatXPM,
	"xbm":         FormatXBM,
	"pnm":         FormatPNM,
	"ppm":         FormatPPM,
	"pgm":         FormatPGM,
	"pbm":         FormatPBM,
	"tga":         FormatTGA,
	"targa":       FormatTGA,
	"dds":         FormatDDS,
	"exr":         FormatEXR,
	"qoi":         FormatQOI,
	"svg":         FormatSVG,
	"pdf":         FormatPDF,
	"ps":          FormatPS,
	"postscript":  FormatPS,
	"eps":         FormatEPS,
	"txt":         FormatTXT,
	"text":        FormatTXT,
	"md":          FormatMD,
	"markdown":    FormatMD,
	"rst":         FormatRST,
	"org":         FormatORG,
	"html":        FormatHTML,
	"htm":         FormatHTML,
	"rtf":         FormatRTF,
	"tex":         FormatTEX,
	"latex":       FormatTEX,
	"odt":         FormatODT,
	"ods":         FormatODS,
	"pptx":        FormatPPTX,
	"docx":        FormatDOCX,
	"xlsx":        FormatXLSX,
	"csv":         FormatCSV,
	"epub":        FormatEPUB,
	"fb2":         FormatFB2,
	"mobi":        FormatMOBI,
	"azw3":        FormatAZW3,
	"djvu":        FormatDJVU,
	"djv":         FormatDJVU,
	"mp4":         FormatMP4,
	"mov":         FormatMOV,
	"avi":         FormatAVI,
	"webm":        FormatWebM,
	"mkv":         FormatMKV,
	"m4v":         FormatM4V,
	"mpg":         FormatMPG,
	"mpeg":        FormatMPEG,
	"flv":         FormatFLV,
	"ogv":         FormatOGV,
	"mp3":         FormatMP3,
	"wav":         FormatWAV,
	"flac":        FormatFLAC,
	"aac":         FormatAAC,
	"m4a":         FormatM4A,
	"ogg":         FormatOGG,
	"opus":        FormatOPUS,
	"wma":         FormatWMA,
	"aiff":        FormatAIFF,
	"aif":         FormatAIFF,
	"zip":         FormatZIP,
	"tar":         FormatTAR,
	"tar.gz":      FormatTGZ,
	"tgz":         FormatTGZ,
	"tar.bz2":     FormatTBZ2,
	"tbz2":        FormatTBZ2,
	"tbz":         FormatTBZ2,
	"tar.xz":      FormatTXZ,
	"txz":         FormatTXZ,
	"tar.zst":     FormatTZST,
	"tzst":        FormatTZST,
	"gz":          FormatGZ,
	"bz2":         FormatBZ2,
	"xz":          FormatXZ,
	"zst":         FormatZST,
	"7z":          Format7Z,
	"rar":         FormatRAR,
	"deb":         FormatDEB,
	"rpm":         FormatRPM,
	"a":           FormatAR,
	"ar":          FormatAR,
	"cpio":        FormatCPIO,
	"iso":         FormatISO,
	"jar":         FormatJAR,
	"war":         FormatWAR,
	"ear":         FormatEAR,
	"apk":         FormatAPK,
	"aar":         FormatAAR,
	"ipa":         FormatIPA,
	"whl":         FormatWHL,
	"egg":         FormatEGG,
	"nupkg":       FormatNUPKG,
	"vsix":        FormatVSIX,
	"xpi":         FormatXPI,
	"gem":         FormatGem,
	"crate":       FormatCrate,
	"pkg.tar.zst": FormatArchPackage,
	"ttf":         FormatTTF,
	"otf":         FormatOTF,
	"woff":        FormatWOFF,
	"woff2":       FormatWOFF2,
	"eot":         FormatEOT,
	"bdf":         FormatBDF,
	"pcf":         FormatPCF,
	"fon":         FormatFON,
	"pfa":         FormatPFA,
	"pfb":         FormatPFB,
	"json":        FormatJSON,
	"jsonl":       FormatJSONL,
	"ndjson":      FormatJSONL,
	"yaml":        FormatYAML,
	"yml":         FormatYAML,
	"xml":         FormatXML,
	"toml":        FormatTOML,
	"ini":         FormatINI,
	"cfg":         FormatINI,
	"env":         FormatENV,
	"dotenv":      FormatENV,
	"plist":       FormatPLIST,
	"tsv":         FormatTSV,
	"dot":         FormatDOT,
	"gv":          FormatDOT,
	"mmd":         FormatMermaid,
	"mermaid":     FormatMermaid,
	"ipynb":       FormatIPYNB,
	"py":          FormatPY,
	"sql":         FormatSQL,
	"sqlite":      FormatSQLite,
	"sqlite3":     FormatSQLite,
	"db":          FormatSQLite,
	"parquet":     FormatParquet,
	"avro":        FormatAvro,
	"orc":         FormatORC,
	"arrow":       FormatArrow,
	"feather":     FormatFeather,
	"bson":        FormatBSON,
	"msgpack":     FormatMsgpack,
	"msgp":        FormatMsgpack,
	"cbor":        FormatCBOR,
	"geojson":     FormatGeoJSON,
	"topojson":    FormatTopoJSON,
	"kml":         FormatKML,
	"kmz":         FormatKMZ,
	"gpx":         FormatGPX,
	"shp":         FormatSHP,
	"gpkg":        FormatGPKG,
	"gml":         FormatGML,
	"osm":         FormatOSM,
	"pbf":         FormatPBF,
	"mbtiles":     FormatMBTiles,
	"pmtiles":     FormatPMTiles,
	"mvt":         FormatMVT,
	"wkt":         FormatWKT,
	"wkb":         FormatWKB,
	"las":         FormatLAS,
	"laz":         FormatLAZ,
	"hgt":         FormatHGT,
	"openapi":     FormatOpenAPI,
	"swagger":     FormatSwagger,
	"jsonschema":  FormatJSONSchema,
	"schema.json": FormatJSONSchema,
	"asyncapi":    FormatAsyncAPI,
	"graphql":     FormatGraphQL,
	"gql":         FormatGraphQL,
	"proto":       FormatProto,
	"protobuf":    FormatProto,
	"protoset":    FormatProtoSet,
	"pb":          FormatProtoSet,
	"thrift":      FormatThrift,
	"avsc":        FormatAvroSchema,
	"fbs":         FormatFlatBuffers,
	"capnp":       FormatCapnp,
	"wsdl":        FormatWSDL,
	"xsd":         FormatXSD,
	"raw":         FormatRAW,
	"img":         FormatIMG,
	"qcow2":       FormatQCOW2,
	"qcow":        FormatQCOW,
	"qed":         FormatQED,
	"vdi":         FormatVDI,
	"vmdk":        FormatVMDK,
	"vhd":         FormatVHD,
	"vhdx":        FormatVHDX,
	"vpc":         FormatVPC,
	"cow":         FormatCOW,
	"dmg":         FormatDMG,
	"ova":         FormatOVA,
	"ovf":         FormatOVF,
	"vbox":        FormatVBox,
	"hdd":         FormatHDD,
	"box":         FormatVagrantBox,
	"dir":         FormatDir,
	"directory":   FormatDir,
	"folder":      FormatDir,
}

// registeredFormats indexes the canonical formats from the aliases map so
// membership checks stay O(1); it is rebuilt lazily after registrations.
var registeredFormats map[Format]bool

func registeredFormatSet() map[Format]bool {
	if registeredFormats == nil {
		registeredFormats = make(map[Format]bool, len(aliases))
		for _, format := range aliases {
			registeredFormats[format] = true
		}
	}
	return registeredFormats
}

var compoundExtensions = []string{
	"pkg.tar.zst",
	"schema.json",
	"tar.gz",
	"tar.bz2",
	"tar.xz",
	"tar.zst",
	"tgz",
	"tbz2",
	"tbz",
	"txz",
	"tzst",
}

func ParseFormat(value string) (Format, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, ".")
	if value == "" {
		return "", fmt.Errorf("%w: empty format", ErrUnknownFormat)
	}

	format, ok := aliases[value]
	if !ok {
		if !validCustomFormat(value) {
			return "", fmt.Errorf("%w: %s", ErrUnknownFormat, value)
		}
		return Format(value), nil
	}

	return format, nil
}

func FormatFromPath(path string) (Format, error) {
	lower := strings.ToLower(path)
	for _, ext := range compoundExtensions {
		if strings.HasSuffix(lower, "."+ext) {
			return ParseFormat(ext)
		}
	}

	ext := filepath.Ext(path)
	if ext == "" {
		return "", fmt.Errorf("%w: %s has no extension", ErrUnknownFormat, path)
	}

	return ParseFormat(ext)
}

func RegisterFormat(name string, values ...string) (Format, error) {
	format, err := ParseFormat(name)
	if err != nil {
		return "", err
	}

	aliases[format.String()] = format
	registeredFormats = nil
	if strings.Contains(format.String(), ".") {
		registerCompoundExtension(format.String())
	}
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		value = strings.TrimPrefix(value, ".")
		if value == "" {
			continue
		}
		if !validCustomFormat(value) {
			return "", fmt.Errorf("%w: %s", ErrUnknownFormat, value)
		}
		aliases[value] = format
		if strings.Contains(value, ".") {
			registerCompoundExtension(value)
		}
	}

	return format, nil
}

func registerCompoundExtension(ext string) {
	for _, existing := range compoundExtensions {
		if existing == ext {
			return
		}
	}
	compoundExtensions = append(compoundExtensions, ext)
	sort.SliceStable(compoundExtensions, func(i, j int) bool {
		return len(compoundExtensions[i]) > len(compoundExtensions[j])
	})
}

func validCustomFormat(value string) bool {
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '.', '-', '_', '+':
			continue
		default:
			return false
		}
	}
	return true
}

func AllFormats() []Format {
	set := registeredFormatSet()
	formats := make([]Format, 0, len(set))
	for format := range set {
		formats = append(formats, format)
	}
	sort.Slice(formats, func(i, j int) bool { return formats[i] < formats[j] })
	return formats
}

func IsRegisteredFormat(format Format) bool {
	return registeredFormatSet()[format]
}

func (f Format) String() string {
	return string(f)
}

func (f Format) Extension() string {
	if f == FormatJPEG {
		return "jpg"
	}
	if f == FormatTGZ {
		return "tar.gz"
	}
	if f == FormatTBZ2 {
		return "tar.bz2"
	}
	if f == FormatTXZ {
		return "tar.xz"
	}
	if f == FormatTZST {
		return "tar.zst"
	}
	if f == FormatAR {
		return "a"
	}
	if f == FormatDir {
		return ""
	}
	return string(f)
}

func (f Format) IsArchive() bool {
	switch f {
	case FormatZIP, FormatTAR, FormatTGZ, FormatTBZ2, FormatTXZ, FormatTZST, FormatGZ, FormatBZ2, FormatXZ, FormatZST, Format7Z, FormatRAR, FormatDEB, FormatRPM, FormatAR, FormatCPIO, FormatISO, FormatJAR, FormatWAR, FormatEAR, FormatAPK, FormatAAR, FormatIPA, FormatWHL, FormatEGG, FormatNUPKG, FormatVSIX, FormatXPI, FormatGem, FormatCrate, FormatArchPackage, FormatOVA, FormatVagrantBox:
		return true
	default:
		return false
	}
}

func (f Format) IsImage() bool {
	switch f {
	case FormatPNG, FormatJPEG, FormatWebP, FormatBMP, FormatTIFF, FormatGIF, FormatAPNG, FormatAVIF, FormatHEIC, FormatICO, FormatICNS, FormatPSD, FormatJP2,
		FormatXPM, FormatXBM, FormatPNM, FormatPPM, FormatPGM, FormatPBM, FormatTGA, FormatDDS, FormatEXR, FormatQOI:
		return true
	default:
		return false
	}
}

func (f Format) IsFont() bool {
	switch f {
	case FormatTTF, FormatOTF, FormatWOFF, FormatWOFF2, FormatEOT, FormatBDF, FormatPCF, FormatFON, FormatPFA, FormatPFB:
		return true
	default:
		return false
	}
}

func (f Format) IsAudio() bool {
	switch f {
	case FormatMP3, FormatWAV, FormatFLAC, FormatAAC, FormatM4A, FormatOGG, FormatOPUS, FormatWMA, FormatAIFF:
		return true
	default:
		return false
	}
}

func (f Format) IsVideo() bool {
	switch f {
	case FormatMP4, FormatMOV, FormatAVI, FormatWebM, FormatMKV, FormatM4V, FormatMPG, FormatMPEG, FormatFLV, FormatOGV:
		return true
	default:
		return false
	}
}

func (f Format) IsDiskImage() bool {
	switch f {
	case FormatRAW, FormatIMG, FormatQCOW2, FormatQCOW, FormatQED, FormatVDI, FormatVMDK, FormatVHD, FormatVHDX, FormatVPC, FormatCOW, FormatDMG, FormatISO, FormatHDD:
		return true
	default:
		return false
	}
}

// CategoryOf buckets a format into a human-facing category used for
// grouping and coloring in the CLI and pickers.
func CategoryOf(f Format) string {
	switch f {
	case FormatDir:
		return "directory"
	case FormatGIF, FormatAPNG:
		return "animation"
	}
	if f.IsImage() || f == FormatSVG {
		return "image"
	}
	if f.IsVideo() {
		return "video"
	}
	if f.IsAudio() {
		return "audio"
	}
	if f.IsArchive() {
		return "archive"
	}
	if f.IsFont() {
		return "font"
	}
	if f.IsDiskImage() {
		return "disk"
	}
	switch f {
	case FormatEPUB, FormatFB2, FormatMOBI, FormatAZW3, FormatDJVU:
		return "ebook"
	case FormatTXT, FormatMD, FormatRST, FormatORG, FormatHTML, FormatRTF, FormatTEX, FormatDOCX, FormatODT, FormatPDF, FormatPS, FormatEPS, FormatPPTX:
		return "doc"
	case FormatXLSX, FormatODS:
		return "spreadsheet"
	case FormatJSON, FormatJSONL, FormatYAML, FormatTOML, FormatCSV, FormatTSV, FormatINI, FormatENV, FormatXML, FormatPLIST, FormatSQL, FormatSQLite, FormatParquet, FormatAvro, FormatORC, FormatArrow, FormatFeather, FormatBSON, FormatMsgpack, FormatCBOR:
		return "data"
	case FormatGeoJSON, FormatTopoJSON, FormatKML, FormatKMZ, FormatGPX, FormatSHP, FormatGPKG, FormatGML, FormatOSM, FormatPBF, FormatMBTiles, FormatPMTiles, FormatMVT, FormatWKT, FormatWKB, FormatLAS, FormatLAZ, FormatHGT:
		return "geo"
	case FormatOpenAPI, FormatSwagger, FormatJSONSchema, FormatAsyncAPI, FormatGraphQL, FormatProto, FormatProtoSet, FormatThrift, FormatAvroSchema, FormatFlatBuffers, FormatCapnp, FormatWSDL, FormatXSD:
		return "schema"
	case FormatDOT, FormatMermaid:
		return "diagram"
	case FormatIPYNB, FormatPY:
		return "code"
	case FormatOVA, FormatOVF, FormatVBox, FormatVagrantBox:
		return "vm"
	}
	return "custom"
}

type TransformAction string

const (
	ActionConvert  TransformAction = "convert"
	ActionCompress TransformAction = "compress"
	ActionResize   TransformAction = "resize"
)

func (a TransformAction) String() string {
	return string(a)
}

type ArchiveAction string

const (
	ArchiveActionExtract ArchiveAction = "extract"
	ArchiveActionConvert ArchiveAction = "convert"
	ArchiveActionCancel  ArchiveAction = "cancel"
)

type FileRef struct {
	Path   string
	Name   string
	Format Format
}

type ConvertOptions struct {
	Overwrite   bool
	Quality     int
	Action      TransformAction
	Resize      string
	ToolOptions ToolOptions
}

type ToolOptions map[string]map[string][]string

func (o ToolOptions) Clone() ToolOptions {
	clone := ToolOptions{}
	for tool, values := range o {
		tool = strings.ToLower(tool)
		clone[tool] = map[string][]string{}
		for key, list := range values {
			key = strings.ToLower(key)
			clone[tool][key] = append([]string(nil), list...)
		}
	}
	return clone
}

func (o ToolOptions) Merge(other ToolOptions) ToolOptions {
	if o == nil {
		o = ToolOptions{}
	}
	for tool, values := range other {
		tool = strings.ToLower(tool)
		if o[tool] == nil {
			o[tool] = map[string][]string{}
		}
		for key, list := range values {
			key = strings.ToLower(key)
			o[tool][key] = append([]string(nil), list...)
		}
	}
	return o
}

func (o ToolOptions) Values(tool string, key string) []string {
	if o == nil {
		return nil
	}
	values := o[strings.ToLower(tool)]
	if values == nil {
		return nil
	}
	return append([]string(nil), values[strings.ToLower(key)]...)
}

type ConvertJob struct {
	InputPath    string
	OutputPath   string
	InputFormat  Format
	OutputFormat Format
	Options      ConvertOptions
}

type ConversionCapability struct {
	Input              Format
	Output             Format
	PreservesAnimation bool
	Lossy              bool
	Priority           int
}

type ConversionResult struct {
	Job        ConvertJob
	Backend    string
	OutputPath string
}
