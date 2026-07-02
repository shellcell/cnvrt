package converters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shellcell/convert/internal/domain"
)

func TestStructuredDataConvertsCSVToJSON(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "people.csv")
	output := filepath.Join(dir, "people.json")
	if err := os.WriteFile(input, []byte("name,age\nAda,36\nGrace,85\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	converter := NewStructuredData()
	_, err := converter.Convert(context.Background(), domain.ConvertJob{
		InputPath:    input,
		OutputPath:   output,
		InputFormat:  domain.FormatCSV,
		OutputFormat: domain.FormatJSON,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]string
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0]["name"] != "Ada" || rows[1]["age"] != "85" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}
