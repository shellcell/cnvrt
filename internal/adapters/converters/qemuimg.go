package converters

import (
	"context"

	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
)

type QemuImg struct {
	runner ports.CommandRunner
	caps   []domain.ConversionCapability
}

func NewQemuImg(runner ports.CommandRunner) *QemuImg {
	formats := []domain.Format{
		domain.FormatRAW,
		domain.FormatIMG,
		domain.FormatQCOW2,
		domain.FormatQCOW,
		domain.FormatQED,
		domain.FormatVDI,
		domain.FormatVMDK,
		domain.FormatVHD,
		domain.FormatVHDX,
		domain.FormatVPC,
	}
	return &QemuImg{runner: runner, caps: capabilities(formats, formats, 90, false, false)}
}

func (c *QemuImg) ID() string { return "qemu-img" }

func (c *QemuImg) RequiredCommands() []string { return []string{"qemu-img"} }

func (c *QemuImg) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *QemuImg) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *QemuImg) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	args := []string{"convert", "-O", qemuFormat(job.OutputFormat), job.InputPath, job.OutputPath}
	return runSimple(ctx, c.runner, "qemu-img", args, job, c.ID())
}

func qemuFormat(format domain.Format) string {
	switch format {
	case domain.FormatIMG:
		return "raw"
	case domain.FormatVHD:
		return "vpc"
	default:
		return format.String()
	}
}
