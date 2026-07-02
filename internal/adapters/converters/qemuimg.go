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

func (c *QemuImg) Description() string {
	return "VM/disk images: qcow2, vmdk, vdi, vhd/vhdx, raw"
}

func (c *QemuImg) RequiredCommands() []string { return []string{"qemu-img"} }

func (c *QemuImg) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *QemuImg) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *QemuImg) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	return runSimple(ctx, c.runner, "qemu-img", c.args(job), job, c.ID())
}

func (c *QemuImg) PreviewCommands(job domain.ConvertJob) ports.CommandPreview {
	return previewCommand("qemu-img", c.args(job))
}

func (c *QemuImg) args(job domain.ConvertJob) []string {
	args := []string{"convert", "-O", qemuFormat(job.OutputFormat)}
	if job.OutputFormat == domain.FormatQCOW2 && job.Options.Action == domain.ActionCompress {
		args = append(args, "-c")
	}
	args = append(args, extraArgs(job.Options.ToolOptions, "qemu-img")...)
	return append(args, job.InputPath, job.OutputPath)
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
