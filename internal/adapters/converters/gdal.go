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

// NewGDAL declares vector translation pairs. TopoJSON, OSM, and PBF drivers
// are read-only in GDAL, so they appear as inputs but never as outputs.
func NewGDAL(runner ports.CommandRunner) *GDAL {
	inputs := []domain.Format{
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
	outputs := []domain.Format{
		domain.FormatGeoJSON,
		domain.FormatKML,
		domain.FormatKMZ,
		domain.FormatGPX,
		domain.FormatSHP,
		domain.FormatGPKG,
		domain.FormatGML,
		domain.FormatCSV,
		domain.FormatSQLite,
	}
	return &GDAL{runner: runner, caps: capabilities(inputs, outputs, 85, false, false)}
}

func (c *GDAL) ID() string { return "gdal" }

func (c *GDAL) Description() string {
	return "geo/vector formats: geojson, kml, gpx, shp, gpkg, and more"
}

func (c *GDAL) RequiredCommands() []string { return []string{"ogr2ogr"} }

func (c *GDAL) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *GDAL) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *GDAL) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "ogr2ogr", c.args(job), job, c.ID())
}

func (c *GDAL) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("ogr2ogr", c.args(job))
}

func (c *GDAL) args(job domain.ConvertJob) []string {
	args := []string{}
	if driver := gdalDriver(job.OutputFormat); driver != "" {
		args = append(args, "-f", driver)
	}
	args = append(args, extraArgs(job.Options.ToolOptions, "gdal")...)
	args = append(args, job.OutputPath, job.InputPath)
	return args
}

func gdalDriver(format domain.Format) string {
	switch format {
	case domain.FormatGeoJSON:
		return "GeoJSON"
	case domain.FormatKML:
		return "KML"
	case domain.FormatKMZ:
		return "LIBKML"
	case domain.FormatGPKG:
		return "GPKG"
	case domain.FormatGML:
		return "GML"
	case domain.FormatGPX:
		return "GPX"
	case domain.FormatSHP:
		return "ESRI Shapefile"
	case domain.FormatCSV:
		return "CSV"
	case domain.FormatSQLite:
		return "SQLite"
	default:
		return ""
	}
}
