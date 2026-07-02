package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type GDAL struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewGDAL(runner ports.CommandRunner) *GDAL {
	formats := []domain.Format{
		domain.FormatGeoJSON,
		domain.FormatTopoJSON,
		domain.FormatKML,
		domain.FormatKMZ,
		domain.FormatGPX,
		domain.FormatSHP,
		domain.FormatGPKG,
		domain.FormatGML,
		domain.FormatOSM,
		domain.FormatPBF,
		domain.FormatCSV,
		domain.FormatSQLite,
	}
	return &GDAL{runner: runner, caps: capabilities(formats, formats, 85, false, false)}
}

func (c *GDAL) ID() string { return "gdal" }

func (c *GDAL) RequiredCommands() []string { return []string{"ogr2ogr"} }

func (c *GDAL) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *GDAL) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *GDAL) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "ogr2ogr", []string{job.OutputPath, job.InputPath}, job, c.ID())
}
