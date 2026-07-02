package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type SevenZip struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewSevenZip(runner ports.CommandRunner) *SevenZip {
	inputs := []domain.Format{
		domain.Format7Z,
		domain.FormatZIP,
		domain.FormatRAR,
		domain.FormatTAR,
		domain.FormatTGZ,
		domain.FormatTBZ2,
		domain.FormatTXZ,
		domain.FormatTZST,
		domain.FormatGZ,
		domain.FormatBZ2,
		domain.FormatXZ,
		domain.FormatZST,
		domain.FormatDEB,
		domain.FormatRPM,
		domain.FormatAR,
		domain.FormatCPIO,
		domain.FormatISO,
		domain.FormatJAR,
		domain.FormatWAR,
		domain.FormatEAR,
		domain.FormatAPK,
		domain.FormatAAR,
		domain.FormatIPA,
		domain.FormatWHL,
		domain.FormatEGG,
		domain.FormatNUPKG,
		domain.FormatVSIX,
		domain.FormatXPI,
		domain.FormatGem,
		domain.FormatCrate,
		domain.FormatArchPackage,
		domain.FormatOVA,
		domain.FormatVagrantBox,
		domain.FormatDMG,
	}
	outputs := []domain.Format{domain.FormatDir}
	caps := capabilities(inputs, outputs, 80, false, false)
	caps = append(caps,
		domain.ConversionCapability{Input: domain.FormatDir, Output: domain.Format7Z, Priority: 80},
		domain.ConversionCapability{Input: domain.FormatDir, Output: domain.FormatZIP, Priority: 70},
	)
	return &SevenZip{runner: runner, caps: caps}
}

func (c *SevenZip) ID() string { return "7z" }

func (c *SevenZip) RequiredCommands() []string { return []string{"7z"} }

func (c *SevenZip) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *SevenZip) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *SevenZip) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	if job.OutputFormat == domain.FormatDir {
		return runSimple(ctx, c.runner, "7z", []string{"x", "-y", "-o" + job.OutputPath, job.InputPath}, job, c.ID())
	}

	return runSimple(ctx, c.runner, "7z", []string{"a", job.OutputPath, job.InputPath}, job, c.ID())
}
