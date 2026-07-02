package converters

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shellcell/convert/internal/domain"
)

type Archive struct {
	caps []domain.ConversionCapability
}

func NewArchive() *Archive {
	return &Archive{caps: []domain.ConversionCapability{
		{Input: domain.FormatZIP, Output: domain.FormatDir, Priority: 100},
		{Input: domain.FormatTAR, Output: domain.FormatDir, Priority: 100},
		{Input: domain.FormatTGZ, Output: domain.FormatDir, Priority: 100},
		{Input: domain.FormatTBZ2, Output: domain.FormatDir, Priority: 90},
		{Input: domain.FormatDir, Output: domain.FormatZIP, Priority: 100},
		{Input: domain.FormatDir, Output: domain.FormatTAR, Priority: 100},
		{Input: domain.FormatDir, Output: domain.FormatTGZ, Priority: 100},
	}}
}

func (c *Archive) ID() string { return "archive" }

func (c *Archive) RequiredCommands() []string { return nil }

func (c *Archive) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *Archive) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *Archive) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	select {
	case <-ctx.Done():
		return domain.ConversionResult{}, ctx.Err()
	default:
	}

	if job.OutputFormat == domain.FormatDir {
		if err := os.MkdirAll(job.OutputPath, 0o755); err != nil {
			return domain.ConversionResult{}, err
		}
		switch job.InputFormat {
		case domain.FormatZIP:
			if err := extractZip(job.InputPath, job.OutputPath); err != nil {
				return domain.ConversionResult{}, err
			}
		case domain.FormatTAR:
			if err := extractTar(job.InputPath, job.OutputPath, nil); err != nil {
				return domain.ConversionResult{}, err
			}
		case domain.FormatTGZ:
			if err := extractTar(job.InputPath, job.OutputPath, gzipReader); err != nil {
				return domain.ConversionResult{}, err
			}
		case domain.FormatTBZ2:
			if err := extractTar(job.InputPath, job.OutputPath, bzip2Reader); err != nil {
				return domain.ConversionResult{}, err
			}
		default:
			return domain.ConversionResult{}, fmt.Errorf("unsupported archive extraction: %s", job.InputFormat)
		}

		return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
	}

	if job.InputFormat != domain.FormatDir {
		return domain.ConversionResult{}, fmt.Errorf("archive creation requires directory input")
	}

	switch job.OutputFormat {
	case domain.FormatZIP:
		if err := createZip(job.InputPath, job.OutputPath); err != nil {
			return domain.ConversionResult{}, err
		}
	case domain.FormatTAR:
		if err := createTar(job.InputPath, job.OutputPath, nil); err != nil {
			return domain.ConversionResult{}, err
		}
	case domain.FormatTGZ:
		if err := createTar(job.InputPath, job.OutputPath, gzipWriter); err != nil {
			return domain.ConversionResult{}, err
		}
	default:
		return domain.ConversionResult{}, fmt.Errorf("unsupported archive creation: %s", job.OutputFormat)
	}

	return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
}

type readerWrap func(io.Reader) (io.ReadCloser, error)
type writerWrap func(io.Writer) (io.WriteCloser, error)

func gzipReader(r io.Reader) (io.ReadCloser, error) { return gzip.NewReader(r) }

func bzip2Reader(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(bzip2.NewReader(r)), nil
}

func gzipWriter(w io.Writer) (io.WriteCloser, error) { return gzip.NewWriter(w), nil }

func extractZip(input string, outputDir string) error {
	reader, err := zip.OpenReader(input)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		target, err := safeJoin(outputDir, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		if err := writeFileFromReader(target, source, file.FileInfo().Mode()); err != nil {
			source.Close()
			return err
		}
		if err := source.Close(); err != nil {
			return err
		}
	}

	return nil
}

func extractTar(input string, outputDir string, wrap readerWrap) error {
	file, err := os.Open(input)
	if err != nil {
		return err
	}
	defer file.Close()

	var reader io.Reader = file
	if wrap != nil {
		wrapped, err := wrap(file)
		if err != nil {
			return err
		}
		defer wrapped.Close()
		reader = wrapped
	}

	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		target, err := safeJoin(outputDir, header.Name)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFileFromReader(target, tarReader, os.FileMode(header.Mode)); err != nil {
				return err
			}
		}
	}
}

func createZip(inputDir string, output string) error {
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	return walkArchive(inputDir, func(path string, rel string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		return copyFileToWriter(path, writer)
	})
}

func createTar(inputDir string, output string, wrap writerWrap) error {
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	defer file.Close()

	var writer io.Writer = file
	if wrap != nil {
		wrapped, err := wrap(file)
		if err != nil {
			return err
		}
		defer wrapped.Close()
		writer = wrapped
	}

	tarWriter := tar.NewWriter(writer)
	defer tarWriter.Close()

	return walkArchive(inputDir, func(path string, rel string, info os.FileInfo) error {
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		return copyFileToWriter(path, tarWriter)
	})
}

func walkArchive(inputDir string, visit func(path string, rel string, info os.FileInfo) error) error {
	root := filepath.Clean(inputDir)
	rootName := filepath.Base(root)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(root), path)
		if err != nil {
			return err
		}
		if rel == "." {
			rel = rootName
		}
		return visit(path, rel, info)
	})
}

func writeFileFromReader(path string, reader io.Reader, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, reader)
	return err
}

func copyFileToWriter(path string, writer io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(writer, file)
	return err
}

func safeJoin(root string, name string) (string, error) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(cleanRoot, name))
	if err != nil {
		return "", err
	}
	if target != cleanRoot && !strings.HasPrefix(target, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry escapes destination: %s", name)
	}
	return target, nil
}
