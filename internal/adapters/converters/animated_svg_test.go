package converters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shellcell/convert/internal/domain"
)

func TestAnimatedSVGCapabilitiesOnlyForAnimatedSVG(t *testing.T) {
	dir := t.TempDir()
	animated := filepath.Join(dir, "animated.svg")
	static := filepath.Join(dir, "static.svg")
	if err := os.WriteFile(animated, []byte(`<svg width="10" height="10"><circle r="2"><animate attributeName="r" values="2;4" dur="1s"/></circle></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(static, []byte(`<svg width="10" height="10"><circle r="2"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}

	converter := NewAnimatedSVG(nil)
	if len(converter.CapabilitiesForInput(static, domain.FormatSVG)) != 0 {
		t.Fatal("static SVG should not expose animated outputs")
	}

	caps := converter.CapabilitiesForInput(animated, domain.FormatSVG)
	if !hasCapability(caps, domain.FormatSVG, domain.FormatMP4) || !hasCapability(caps, domain.FormatSVG, domain.FormatGIF) {
		t.Fatalf("animated SVG should expose video/animation outputs: %#v", caps)
	}
}
