package converters

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/shellcell/convert/internal/domain"
	"github.com/shellcell/convert/internal/ports"
	ini "gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
	"howett.net/plist"
)

type StructuredData struct {
	caps []domain.ConversionCapability
}

func NewStructuredData() *StructuredData {
	formats := []domain.Format{
		domain.FormatJSON,
		domain.FormatJSONL,
		domain.FormatYAML,
		domain.FormatTOML,
		domain.FormatCSV,
		domain.FormatTSV,
		domain.FormatINI,
		domain.FormatENV,
		domain.FormatXML,
		domain.FormatPLIST,
	}
	caps := capabilities(formats, formats, 95, false, false)
	// Structured data can also leave the structured world: txt renders the
	// parsed data as plain text (or copies the raw content), md renders a
	// Markdown table for tabular data and a fenced block otherwise.
	caps = append(caps, capabilities(formats, []domain.Format{domain.FormatTXT, domain.FormatMD}, 95, false, false)...)
	return &StructuredData{caps: caps}
}

func (c *StructuredData) ID() string { return "structured" }

func (c *StructuredData) Description() string {
	return "built-in structured data conversions (json, jsonl, yaml, toml, csv, tsv, ini, env, xml, plist) plus txt/md rendering"
}

func (c *StructuredData) RequiredCommands() []string { return nil }

func (c *StructuredData) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *StructuredData) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
}

func (c *StructuredData) OptionSpecs(input domain.Format, output domain.Format) []ports.OptionSpec {
	if output != domain.FormatTXT {
		return nil
	}
	return []ports.OptionSpec{{
		Tool:        "structured",
		Key:         "text_style",
		Title:       "Plain text style",
		Description: "pretty renders the parsed data as readable text; raw copies the original file content unchanged.",
		Default:     "pretty",
	}}
}

func (c *StructuredData) Convert(ctx context.Context, job domain.ConvertJob) (domain.ConversionResult, error) {
	select {
	case <-ctx.Done():
		return domain.ConversionResult{}, ctx.Err()
	default:
	}

	data, err := os.ReadFile(job.InputPath)
	if err != nil {
		return domain.ConversionResult{}, err
	}

	var encoded []byte
	if job.OutputFormat == domain.FormatTXT && textStyle(job) == "raw" {
		encoded = data
	} else {
		value, err := decodeStructured(job.InputFormat, data)
		if err != nil {
			return domain.ConversionResult{}, err
		}
		encoded, err = encodeStructuredOutput(job.OutputFormat, normalizeStructured(value))
		if err != nil {
			return domain.ConversionResult{}, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0o755); err != nil {
		return domain.ConversionResult{}, err
	}
	if err := os.WriteFile(job.OutputPath, ensureTrailingNewline(encoded), 0o644); err != nil {
		return domain.ConversionResult{}, err
	}

	return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
}

func textStyle(job domain.ConvertJob) string {
	return strings.ToLower(stringOption(job.Options.ToolOptions, "structured", "text_style", "pretty"))
}

func encodeStructuredOutput(format domain.Format, value interface{}) ([]byte, error) {
	switch format {
	case domain.FormatTXT:
		var b strings.Builder
		renderPlainText(&b, value, 0)
		return []byte(b.String()), nil
	case domain.FormatMD:
		return encodeMarkdown(value)
	default:
		return encodeStructured(format, value)
	}
}

// renderPlainText writes the parsed structure as indented "key: value" lines,
// so structured files stay readable once converted to plain text.
func renderPlainText(b *strings.Builder, value interface{}, indent int) {
	switch value := value.(type) {
	case map[string]interface{}:
		if len(value) == 0 {
			writeIndent(b, indent)
			b.WriteString("(empty)\n")
			return
		}
		for _, key := range sortedMapKeys(value) {
			writeIndent(b, indent)
			b.WriteString(key)
			b.WriteString(":")
			if isScalarValue(value[key]) {
				b.WriteString(" ")
				b.WriteString(flatStructuredValue(value[key]))
				b.WriteString("\n")
				continue
			}
			b.WriteString("\n")
			renderPlainText(b, value[key], indent+1)
		}
	case []interface{}:
		if len(value) == 0 {
			writeIndent(b, indent)
			b.WriteString("(empty list)\n")
			return
		}
		for _, item := range value {
			if isScalarValue(item) {
				writeIndent(b, indent)
				b.WriteString("- ")
				b.WriteString(flatStructuredValue(item))
				b.WriteString("\n")
				continue
			}
			writeIndent(b, indent)
			b.WriteString("-\n")
			renderPlainText(b, item, indent+1)
		}
	default:
		writeIndent(b, indent)
		b.WriteString(flatStructuredValue(value))
		b.WriteString("\n")
	}
}

func writeIndent(b *strings.Builder, indent int) {
	for range indent {
		b.WriteString("  ")
	}
}

func isScalarValue(value interface{}) bool {
	switch value.(type) {
	case nil, string, bool, int, int64, uint64, float64, json.Number:
		return true
	default:
		return false
	}
}

// encodeMarkdown renders tabular data (a list of flat objects, e.g. a parsed
// CSV) as a Markdown table and everything else as a fenced JSON block.
func encodeMarkdown(value interface{}) ([]byte, error) {
	if rows, ok := markdownTableRows(value); ok {
		headers := csvHeaders(rows)
		var b strings.Builder
		writeMarkdownRow(&b, headers, func(header string) string { return header })
		writeMarkdownRow(&b, headers, func(string) string { return "---" })
		for _, row := range rows {
			writeMarkdownRow(&b, headers, func(header string) string {
				return markdownCell(flatStructuredValue(row[header]))
			})
		}
		return []byte(b.String()), nil
	}

	pretty, err := encodeStructured(domain.FormatJSON, value)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("```json\n")
	b.Write(pretty)
	if len(pretty) > 0 && pretty[len(pretty)-1] != '\n' {
		b.WriteString("\n")
	}
	b.WriteString("```\n")
	return []byte(b.String()), nil
}

func markdownTableRows(value interface{}) ([]map[string]interface{}, bool) {
	list, ok := value.([]interface{})
	if !ok || len(list) == 0 {
		return nil, false
	}
	rows := make([]map[string]interface{}, 0, len(list))
	for _, item := range list {
		row, ok := item.(map[string]interface{})
		if !ok {
			return nil, false
		}
		for _, cell := range row {
			if !isScalarValue(cell) {
				return nil, false
			}
		}
		rows = append(rows, row)
	}
	return rows, true
}

func writeMarkdownRow(b *strings.Builder, headers []string, cell func(string) string) {
	b.WriteString("|")
	for _, header := range headers {
		b.WriteString(" ")
		b.WriteString(cell(header))
		b.WriteString(" |")
	}
	b.WriteString("\n")
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", `\|`)
	value = strings.ReplaceAll(value, "\r\n", "<br>")
	value = strings.ReplaceAll(value, "\n", "<br>")
	return value
}

func decodeStructured(format domain.Format, data []byte) (interface{}, error) {
	switch format {
	case domain.FormatJSON:
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		var value interface{}
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	case domain.FormatYAML:
		var value interface{}
		if err := yaml.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		return value, nil
	case domain.FormatTOML:
		var value map[string]interface{}
		if err := toml.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		return value, nil
	case domain.FormatJSONL:
		return decodeJSONLines(data)
	case domain.FormatCSV:
		return decodeCSV(data, ',')
	case domain.FormatTSV:
		return decodeCSV(data, '\t')
	case domain.FormatINI:
		return decodeINI(data)
	case domain.FormatENV:
		return decodeEnv(data)
	case domain.FormatXML:
		return decodeXML(data)
	case domain.FormatPLIST:
		var value interface{}
		if _, err := plist.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, fmt.Errorf("unsupported structured input format: %s", format)
	}
}

func encodeStructured(format domain.Format, value interface{}) ([]byte, error) {
	switch format {
	case domain.FormatJSON:
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case domain.FormatYAML:
		return yaml.Marshal(value)
	case domain.FormatTOML:
		return toml.Marshal(tomlDocument(value))
	case domain.FormatJSONL:
		return encodeJSONLines(value)
	case domain.FormatCSV:
		return encodeCSV(value, ',')
	case domain.FormatTSV:
		return encodeCSV(value, '\t')
	case domain.FormatINI:
		return encodeINI(value)
	case domain.FormatENV:
		return encodeEnv(value)
	case domain.FormatXML:
		return encodeXML(value)
	case domain.FormatPLIST:
		return plist.MarshalIndent(value, plist.XMLFormat, "  ")
	default:
		return nil, fmt.Errorf("unsupported structured output format: %s", format)
	}
}

// decodeJSONLines reads JSON Lines / NDJSON into a list; the streaming
// decoder also tolerates records that span multiple lines.
func decodeJSONLines(data []byte) (interface{}, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	rows := []interface{}{}
	for {
		var value interface{}
		if err := decoder.Decode(&value); err == io.EOF {
			return rows, nil
		} else if err != nil {
			return nil, err
		}
		rows = append(rows, value)
	}
}

func encodeJSONLines(value interface{}) ([]byte, error) {
	rows, ok := value.([]interface{})
	if !ok {
		rows = []interface{}{value}
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// decodeEnv parses dotenv-style KEY=VALUE lines into a flat object.
func decodeEnv(data []byte) (interface{}, error) {
	result := map[string]interface{}{}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid env line: %s", line)
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		result[key] = value
	}
	return result, nil
}

// encodeEnv writes a flat object as dotenv lines. Nested values are encoded
// as compact JSON since env values are plain strings.
func encodeEnv(value interface{}) ([]byte, error) {
	root, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("env output requires an object with key/value pairs")
	}
	var b strings.Builder
	for _, key := range sortedMapKeys(root) {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(envValue(flatStructuredValue(root[key])))
		b.WriteString("\n")
	}
	return []byte(b.String()), nil
}

func envValue(value string) string {
	if value == "" || strings.ContainsAny(value, " \t\n\"'#$\\") {
		return strconv.Quote(value)
	}
	return value
}

func decodeCSV(data []byte, delimiter rune) (interface{}, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return []interface{}{}, nil
	}

	headers := normalizeCSVHeaders(records[0])
	rows := make([]interface{}, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]interface{}{}
		for i, header := range headers {
			value := ""
			if i < len(record) {
				value = record[i]
			}
			row[header] = value
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func encodeCSV(value interface{}, delimiter rune) ([]byte, error) {
	rows := rowsForCSV(value)
	headers := csvHeaders(rows)
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.Comma = delimiter
	if err := writer.Write(headers); err != nil {
		return nil, err
	}
	for _, row := range rows {
		record := make([]string, len(headers))
		for i, header := range headers {
			record[i] = csvCell(row[header])
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buf.Bytes(), writer.Error()
}

func decodeINI(data []byte) (interface{}, error) {
	file, err := ini.LoadSources(ini.LoadOptions{Insensitive: false}, data)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{}
	for _, section := range file.Sections() {
		values := map[string]interface{}{}
		for _, key := range section.Keys() {
			values[key.Name()] = key.Value()
		}
		if section.Name() == ini.DefaultSection {
			for key, value := range values {
				result[key] = value
			}
			continue
		}
		result[section.Name()] = values
	}
	return result, nil
}

func encodeINI(value interface{}) ([]byte, error) {
	file := ini.Empty()
	root, ok := value.(map[string]interface{})
	if !ok {
		root = map[string]interface{}{"value": value}
	}

	for _, key := range sortedMapKeys(root) {
		value := root[key]
		if sectionValues, ok := value.(map[string]interface{}); ok {
			section := file.Section(key)
			for _, sectionKey := range sortedMapKeys(sectionValues) {
				section.Key(sectionKey).SetValue(flatStructuredValue(sectionValues[sectionKey]))
			}
			continue
		}
		file.Section("").Key(key).SetValue(flatStructuredValue(value))
	}

	var buf bytes.Buffer
	_, err := file.WriteTo(&buf)
	return buf.Bytes(), err
}

type xmlElement struct {
	Name     string
	Attrs    map[string]string
	Children []xmlElement
	Text     string
}

func decodeXML(data []byte) (interface{}, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var stack []*xmlElement
	var root *xmlElement
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			element := &xmlElement{Name: token.Name.Local, Attrs: map[string]string{}}
			for _, attr := range token.Attr {
				element.Attrs[attr.Name.Local] = attr.Value
			}
			stack = append(stack, element)
		case xml.CharData:
			if len(stack) > 0 {
				text := strings.TrimSpace(string(token))
				if text != "" {
					if stack[len(stack)-1].Text != "" {
						stack[len(stack)-1].Text += " "
					}
					stack[len(stack)-1].Text += text
				}
			}
		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			element := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(stack) == 0 {
				root = element
				continue
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, *element)
		}
	}
	if root == nil {
		return nil, fmt.Errorf("empty XML document")
	}
	return map[string]interface{}{root.Name: xmlElementValue(*root)}, nil
}

func encodeXML(value interface{}) ([]byte, error) {
	rootName := "root"
	content := value
	if root, ok := value.(map[string]interface{}); ok && len(root) == 1 {
		for key, value := range root {
			rootName = xmlName(key)
			content = value
		}
	}

	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	encoder := xml.NewEncoder(&buf)
	encoder.Indent("", "  ")
	if err := encodeXMLElement(encoder, rootName, content); err != nil {
		return nil, err
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func xmlElementValue(element xmlElement) interface{} {
	if len(element.Attrs) == 0 && len(element.Children) == 0 {
		return element.Text
	}
	result := map[string]interface{}{}
	for _, key := range sortedStringMapKeys(element.Attrs) {
		result["@"+key] = element.Attrs[key]
	}
	for _, child := range element.Children {
		value := xmlElementValue(child)
		if existing, ok := result[child.Name]; ok {
			switch list := existing.(type) {
			case []interface{}:
				result[child.Name] = append(list, value)
			default:
				result[child.Name] = []interface{}{existing, value}
			}
		} else {
			result[child.Name] = value
		}
	}
	if element.Text != "" {
		result["#text"] = element.Text
	}
	return result
}

func encodeXMLElement(encoder *xml.Encoder, name string, value interface{}) error {
	start := xml.StartElement{Name: xml.Name{Local: xmlName(name)}}
	if values, ok := value.(map[string]interface{}); ok {
		for _, key := range sortedMapKeys(values) {
			if strings.HasPrefix(key, "@") {
				start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: xmlName(strings.TrimPrefix(key, "@"))}, Value: flatStructuredValue(values[key])})
			}
		}
	}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}

	if values, ok := value.(map[string]interface{}); ok {
		if text, ok := values["#text"]; ok {
			if err := encoder.EncodeToken(xml.CharData([]byte(flatStructuredValue(text)))); err != nil {
				return err
			}
		}
		for _, key := range sortedMapKeys(values) {
			if strings.HasPrefix(key, "@") || key == "#text" {
				continue
			}
			if err := encodeXMLChild(encoder, key, values[key]); err != nil {
				return err
			}
		}
	} else {
		if err := encoder.EncodeToken(xml.CharData([]byte(flatStructuredValue(value)))); err != nil {
			return err
		}
	}

	return encoder.EncodeToken(start.End())
}

func encodeXMLChild(encoder *xml.Encoder, name string, value interface{}) error {
	if list, ok := value.([]interface{}); ok {
		for _, item := range list {
			if err := encodeXMLElement(encoder, name, item); err != nil {
				return err
			}
		}
		return nil
	}
	return encodeXMLElement(encoder, name, value)
}

func normalizeStructured(value interface{}) interface{} {
	switch value := value.(type) {
	case map[string]interface{}:
		result := map[string]interface{}{}
		for key, item := range value {
			result[key] = normalizeStructured(item)
		}
		return result
	case map[interface{}]interface{}:
		result := map[string]interface{}{}
		for key, item := range value {
			result[fmt.Sprint(key)] = normalizeStructured(item)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(value))
		for i, item := range value {
			result[i] = normalizeStructured(item)
		}
		return result
	case []map[string]interface{}:
		result := make([]interface{}, len(value))
		for i, item := range value {
			result[i] = normalizeStructured(item)
		}
		return result
	case json.Number:
		if intValue, err := value.Int64(); err == nil {
			return intValue
		}
		if floatValue, err := value.Float64(); err == nil {
			return floatValue
		}
		return value.String()
	default:
		return value
	}
}

func tomlDocument(value interface{}) interface{} {
	switch value := value.(type) {
	case map[string]interface{}:
		return value
	case []interface{}:
		return map[string]interface{}{"items": value}
	default:
		return map[string]interface{}{"value": value}
	}
}

func rowsForCSV(value interface{}) []map[string]interface{} {
	if root, ok := value.(map[string]interface{}); ok {
		if items, ok := root["items"].([]interface{}); ok {
			return rowsForCSV(items)
		}
		return []map[string]interface{}{root}
	}
	if list, ok := value.([]interface{}); ok {
		rows := make([]map[string]interface{}, 0, len(list))
		for _, item := range list {
			if row, ok := item.(map[string]interface{}); ok {
				rows = append(rows, row)
			} else {
				rows = append(rows, map[string]interface{}{"value": item})
			}
		}
		return rows
	}
	return []map[string]interface{}{{"value": value}}
}

func csvHeaders(rows []map[string]interface{}) []string {
	seen := map[string]bool{}
	var headers []string
	for _, row := range rows {
		for key := range row {
			if !seen[key] {
				seen[key] = true
				headers = append(headers, key)
			}
		}
	}
	sort.Strings(headers)
	if len(headers) == 0 {
		return []string{"value"}
	}
	return headers
}

func normalizeCSVHeaders(headers []string) []string {
	seen := map[string]int{}
	result := make([]string, len(headers))
	for i, header := range headers {
		header = strings.TrimSpace(header)
		if header == "" {
			header = fmt.Sprintf("column_%d", i+1)
		}
		seen[header]++
		if seen[header] > 1 {
			header = fmt.Sprintf("%s_%d", header, seen[header])
		}
		result[i] = header
	}
	return result
}

func csvCell(value interface{}) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func flatStructuredValue(value interface{}) string {
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case bool:
		return strconv.FormatBool(value)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func sortedMapKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func xmlName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "item"
	}
	var b strings.Builder
	for i, r := range value {
		valid := r == '_' || r == '-' || r == '.' || r == ':' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')
		if !valid || (i == 0 && (r == '-' || r == '.' || r == ':' || (r >= '0' && r <= '9'))) {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "item"
	}
	return b.String()
}

func ensureTrailingNewline(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return data
	}
	return append(data, '\n')
}
