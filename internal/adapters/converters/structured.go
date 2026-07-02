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
		domain.FormatYAML,
		domain.FormatTOML,
		domain.FormatCSV,
		domain.FormatINI,
		domain.FormatXML,
		domain.FormatPLIST,
	}
	return &StructuredData{caps: capabilities(formats, formats, 95, false, false)}
}

func (c *StructuredData) ID() string { return "structured" }

func (c *StructuredData) RequiredCommands() []string { return nil }

func (c *StructuredData) Capabilities() []domain.ConversionCapability {
	return append([]domain.ConversionCapability(nil), c.caps...)
}

func (c *StructuredData) CanConvert(input domain.Format, output domain.Format) bool {
	return hasCapability(c.caps, input, output)
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

	value, err := decodeStructured(job.InputFormat, data)
	if err != nil {
		return domain.ConversionResult{}, err
	}
	encoded, err := encodeStructured(job.OutputFormat, normalizeStructured(value))
	if err != nil {
		return domain.ConversionResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0o755); err != nil {
		return domain.ConversionResult{}, err
	}
	if err := os.WriteFile(job.OutputPath, ensureTrailingNewline(encoded), 0o644); err != nil {
		return domain.ConversionResult{}, err
	}

	return domain.ConversionResult{Job: job, Backend: c.ID(), OutputPath: job.OutputPath}, nil
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
	case domain.FormatCSV:
		return decodeCSV(data)
	case domain.FormatINI:
		return decodeINI(data)
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
	case domain.FormatCSV:
		return encodeCSV(value)
	case domain.FormatINI:
		return encodeINI(value)
	case domain.FormatXML:
		return encodeXML(value)
	case domain.FormatPLIST:
		return plist.MarshalIndent(value, plist.XMLFormat, "  ")
	default:
		return nil, fmt.Errorf("unsupported structured output format: %s", format)
	}
}

func decodeCSV(data []byte) (interface{}, error) {
	reader := csv.NewReader(bytes.NewReader(data))
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

func encodeCSV(value interface{}) ([]byte, error) {
	rows := rowsForCSV(value)
	headers := csvHeaders(rows)
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
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
