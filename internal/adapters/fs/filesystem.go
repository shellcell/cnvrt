package fsadapter

import (
	"context"
	"encoding/xml"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/shellcell/convert/internal/domain"
)

type FileSystem struct{}

func NewFileSystem() *FileSystem {
	return &FileSystem{}
}

func (fs *FileSystem) CurrentDir() (string, error) {
	return os.Getwd()
}

func (fs *FileSystem) Abs(path string) (string, error) {
	return filepath.Abs(path)
}

func (fs *FileSystem) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (fs *FileSystem) IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

func (fs *FileSystem) IsTextFile(path string) (bool, error) {
	return isTextFile(path)
}

func (fs *FileSystem) SourceSize(path string, format domain.Format) (string, bool, error) {
	if format == domain.FormatSVG {
		return svgSize(path)
	}
	if format.IsImage() {
		return rasterImageSize(path)
	}
	return "", false, nil
}

func (fs *FileSystem) EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

type Discovery struct{}

func NewDiscovery() *Discovery {
	return &Discovery{}
}

func (d *Discovery) ListFiles(ctx context.Context, root string) ([]domain.FileRef, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	files := make([]domain.FileRef, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		format, ok := discoveredFormat(filepath.Join(root, entry.Name()), entry.Name(), entry.IsDir())
		if !ok {
			continue
		}

		files = append(files, domain.FileRef{
			Path:   filepath.Join(root, entry.Name()),
			Name:   entry.Name(),
			Format: format,
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func discoveredFormat(path string, name string, isDir bool) (domain.Format, bool) {
	if isDir {
		return domain.FormatDir, true
	}

	format, err := domain.FormatFromPath(name)
	if err != nil {
		if text, textErr := isTextFile(path); textErr == nil && text {
			return domain.FormatTXT, true
		}
		return "", false
	}
	if !domain.IsRegisteredFormat(format) {
		if text, textErr := isTextFile(path); textErr == nil && text {
			return domain.FormatTXT, true
		}
	}
	return format, true
}

func isTextFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		return false, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	buffer := make([]byte, 8192)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return false, err
	}
	return looksLikeText(buffer[:n]), nil
}

func looksLikeText(data []byte) bool {
	if len(data) == 0 {
		return true
	}

	control := 0
	for _, b := range data {
		if b == 0 {
			return false
		}
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' && b != '\f' && b != '\b' {
			control++
		}
		if b == 0x7f {
			control++
		}
	}
	return control*100 <= len(data)*30
}

func rasterImageSize(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return "", false, nil
	}
	return strconv.Itoa(config.Width) + "x" + strconv.Itoa(config.Height), true, nil
}

func svgSize(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	decoder := xml.NewDecoder(file)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || strings.ToLower(start.Name.Local) != "svg" {
			continue
		}

		attrs := map[string]string{}
		for _, attr := range start.Attr {
			attrs[strings.ToLower(attr.Name.Local)] = attr.Value
		}
		if width, ok := svgLength(attrs["width"]); ok {
			if height, ok := svgLength(attrs["height"]); ok {
				return sizeString(width, height), true, nil
			}
		}
		if width, height, ok := svgViewBoxSize(attrs["viewbox"]); ok {
			return sizeString(width, height), true, nil
		}
		return "", false, nil
	}
}

func svgLength(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasSuffix(value, "%") {
		return 0, false
	}
	end := 0
	for end < len(value) {
		ch := value[end]
		if (ch >= '0' && ch <= '9') || ch == '.' || ch == '+' || ch == '-' {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return 0, false
	}
	number, err := strconv.ParseFloat(value[:end], 64)
	if err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

func svgViewBoxSize(value string) (float64, float64, bool) {
	fields := strings.Fields(strings.ReplaceAll(strings.TrimSpace(value), ",", " "))
	if len(fields) != 4 {
		return 0, 0, false
	}
	width, err := strconv.ParseFloat(fields[2], 64)
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err := strconv.ParseFloat(fields[3], 64)
	if err != nil || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func sizeString(width float64, height float64) string {
	return strconv.Itoa(max(1, int(math.Round(width)))) + "x" + strconv.Itoa(max(1, int(math.Round(height))))
}
