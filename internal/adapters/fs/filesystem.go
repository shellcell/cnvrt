package fsadapter

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"

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
