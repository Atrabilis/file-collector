package tabular

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
)

type xmlNode struct {
	Name     string
	Text     strings.Builder
	Attrs    map[string]string
	Children []*xmlNode
}

func readXMLFileSummary(path string, previewRows int, opts TypedReadOptions) (*TypedFileSummary, error) {
	values, parseErrors, err := extractXMLValues(path, opts)
	if err != nil {
		return nil, err
	}

	fieldOrder := orderedXMLFieldNames(opts.XMLFields)
	summary := &TypedFileSummary{
		Delimiter:          0,
		Header:             append([]string(nil), fieldOrder...),
		RowCount:           1,
		WellFormedRowCount: 1,
		ParseErrorCount:    len(parseErrors),
		ParseErrors:        limitedParseErrors(parseErrors, previewRows),
		PreviewRows:        make([]TypedRow, 0, min(previewRows, 1)),
	}

	if previewRows > 0 {
		rowValues := make(map[string]any, len(values))
		raw := make([]string, 0, len(fieldOrder))
		for _, fieldName := range fieldOrder {
			value, ok := values[fieldName]
			if !ok || value == nil {
				continue
			}
			rowValues[fieldName] = value
			raw = append(raw, fmt.Sprint(value))
		}
		summary.PreviewRows = append(summary.PreviewRows, TypedRow{
			LineNumber: 1,
			Raw:        raw,
			Values:     rowValues,
		})
	}

	return summary, nil
}

func forEachXMLRow(path string, opts TypedReadOptions, fn func(TypedRow) error) error {
	values, _, err := extractXMLValues(path, opts)
	if err != nil {
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	return fn(TypedRow{
		LineNumber:     1,
		ByteOffset:     0,
		NextByteOffset: info.Size(),
		Values:         values,
	})
}

func extractXMLValues(path string, opts TypedReadOptions) (map[string]any, []ParseError, error) {
	root, err := parseXMLDocument(path)
	if err != nil {
		return nil, nil, err
	}

	values := make(map[string]any, len(opts.XMLFields))
	parseErrors := make([]ParseError, 0)

	for _, field := range opts.XMLFields {
		nodes, attrName := resolveXMLPath(root, field.Path)
		if len(nodes) == 0 {
			continue
		}
		value, errs := extractXMLFieldValue(nodes, attrName, field, opts)
		parseErrors = append(parseErrors, errs...)
		if value != nil {
			values[field.Name] = value
		}
	}

	return values, parseErrors, nil
}

func extractXMLFieldValue(nodes []*xmlNode, attrName string, field XMLFieldSpec, opts TypedReadOptions) (any, []ParseError) {
	parseErrors := make([]ParseError, 0)
	if len(nodes) == 0 {
		return nil, parseErrors
	}

	resolveRaw := func(node *xmlNode) string {
		if attrName != "" {
			return strings.TrimSpace(nodeAttr(node, attrName))
		}
		return strings.TrimSpace(nodeText(node))
	}

	parseScalar := func(raw string) (any, bool) {
		if raw == "" {
			return nil, false
		}

		switch field.Type {
		case ColumnTypeTimestamp:
			ts, err := parseTimestampValue(raw, opts.TimestampLayouts, opts.TimestampLocation)
			if err != nil {
				parseErrors = append(parseErrors, ParseError{
					LineNumber:   1,
					ColumnName:   field.Name,
					ExpectedType: ColumnTypeTimestamp,
					Raw:          raw,
					Error:        err.Error(),
				})
				return nil, false
			}
			return ts, true
		case ColumnTypeFloat64:
			fv, err := parseFloatValue(raw, opts.DecimalComma)
			if err != nil {
				parseErrors = append(parseErrors, ParseError{
					LineNumber:   1,
					ColumnName:   field.Name,
					ExpectedType: ColumnTypeFloat64,
					Raw:          raw,
					Error:        err.Error(),
				})
				return nil, false
			}
			return fv, true
		case ColumnTypeInt64:
			iv, err := parseIntValue(raw, opts.DecimalComma)
			if err != nil {
				parseErrors = append(parseErrors, ParseError{
					LineNumber:   1,
					ColumnName:   field.Name,
					ExpectedType: ColumnTypeInt64,
					Raw:          raw,
					Error:        err.Error(),
				})
				return nil, false
			}
			return iv, true
		case ColumnTypeBool:
			bv, err := parseBoolValue(raw)
			if err != nil {
				parseErrors = append(parseErrors, ParseError{
					LineNumber:   1,
					ColumnName:   field.Name,
					ExpectedType: ColumnTypeBool,
					Raw:          raw,
					Error:        err.Error(),
				})
				return nil, false
			}
			return bv, true
		case ColumnTypeString:
			return raw, true
		default:
			return raw, true
		}
	}

	if len(nodes) == 1 {
		raw := resolveRaw(nodes[0])
		if raw == "" && field.Type != "json" && field.Type != "jsonb" {
			return nil, parseErrors
		}
		if field.Type == "json" || field.Type == "jsonb" {
			payload, err := marshalXMLNodesWithAttr(nodes, attrName)
			if err != nil {
				parseErrors = append(parseErrors, ParseError{
					LineNumber:   1,
					ColumnName:   field.Name,
					ExpectedType: field.Type,
					Raw:          field.Path,
					Error:        err.Error(),
				})
				return nil, parseErrors
			}
			return payload, parseErrors
		}
		value, ok := parseScalar(raw)
		if !ok {
			return nil, parseErrors
		}
		return value, parseErrors
	}

	switch field.Type {
	case ColumnTypeString, "json", "jsonb":
		payload, err := marshalXMLNodesWithAttr(nodes, attrName)
		if err != nil {
			parseErrors = append(parseErrors, ParseError{
				LineNumber:   1,
				ColumnName:   field.Name,
				ExpectedType: field.Type,
				Raw:          field.Path,
				Error:        err.Error(),
			})
			return nil, parseErrors
		}
		return payload, parseErrors
	default:
		value, ok := parseScalar(resolveRaw(nodes[0]))
		if !ok {
			return nil, parseErrors
		}
		return value, parseErrors
	}
}

func parseXMLDocument(path string) (*xmlNode, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := xml.NewDecoder(file)
	var root *xmlNode
	stack := make([]*xmlNode, 0, 16)

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch tok := token.(type) {
		case xml.StartElement:
			node := &xmlNode{Name: tok.Name.Local, Attrs: make(map[string]string, len(tok.Attr))}
			for _, attr := range tok.Attr {
				node.Attrs[attr.Name.Local] = attr.Value
			}
			if len(stack) == 0 {
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			text := strings.TrimSpace(string(tok))
			if text == "" {
				continue
			}
			stack[len(stack)-1].Text.WriteString(text)
		}
	}

	if root == nil {
		return nil, fmt.Errorf("empty xml document")
	}

	return root, nil
}

func resolveXMLPath(root *xmlNode, path string) ([]*xmlNode, string) {
	segments := splitXMLPath(path)
	if len(segments) == 0 {
		return nil, ""
	}
	attrName := ""
	lastIdx := len(segments) - 1
	if idx := strings.LastIndex(segments[lastIdx], "@"); idx >= 0 {
		attrName = strings.TrimSpace(segments[lastIdx][idx+1:])
		segments[lastIdx] = strings.TrimSpace(segments[lastIdx][:idx])
	}
	if strings.EqualFold(segments[0], root.Name) {
		segments = segments[1:]
	}
	current := []*xmlNode{root}
	for _, segment := range segments {
		next := make([]*xmlNode, 0)
		for _, node := range current {
			for _, child := range node.Children {
				if child.Name == segment {
					next = append(next, child)
				}
			}
		}
		current = next
		if len(current) == 0 {
			return nil, attrName
		}
	}
	return current, attrName
}

func splitXMLPath(path string) []string {
	parts := strings.Split(path, ".")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func nodeText(node *xmlNode) string {
	if node == nil {
		return ""
	}
	return node.Text.String()
}

func nodeAttr(node *xmlNode, attrName string) string {
	if node == nil || strings.TrimSpace(attrName) == "" {
		return ""
	}
	if node.Attrs == nil {
		return ""
	}
	return node.Attrs[attrName]
}

func nodeToValue(node *xmlNode) any {
	if node == nil {
		return nil
	}
	if len(node.Children) == 0 {
		if len(node.Attrs) == 0 {
			return strings.TrimSpace(node.Text.String())
		}
		values := make(map[string]any, len(node.Attrs)+1)
		for name, value := range node.Attrs {
			values["@"+name] = value
		}
		text := strings.TrimSpace(node.Text.String())
		if text != "" {
			values["#text"] = text
		}
		return values
	}

	values := make(map[string]any)
	for name, value := range node.Attrs {
		values["@"+name] = value
	}
	grouped := make(map[string][]*xmlNode)
	for _, child := range node.Children {
		grouped[child.Name] = append(grouped[child.Name], child)
	}

	for childName, children := range grouped {
		if len(children) == 1 {
			values[childName] = nodeToValue(children[0])
			continue
		}
		items := make([]any, 0, len(children))
		for _, child := range children {
			items = append(items, nodeToValue(child))
		}
		values[childName] = items
	}

	return values
}

func marshalXMLNodes(nodes []*xmlNode) (string, error) {
	return marshalXMLNodesWithAttr(nodes, "")
}

func marshalXMLNodesWithAttr(nodes []*xmlNode, attrName string) (string, error) {
	if len(nodes) == 0 {
		return "[]", nil
	}
	if len(nodes) == 1 {
		var payloadValue any
		if attrName != "" {
			payloadValue = nodeAttr(nodes[0], attrName)
		} else {
			payloadValue = nodeToValue(nodes[0])
		}
		payload, err := json.Marshal(payloadValue)
		if err != nil {
			return "", err
		}
		return string(payload), nil
	}

	items := make([]any, 0, len(nodes))
	for _, node := range nodes {
		if attrName != "" {
			items = append(items, nodeAttr(node, attrName))
		} else {
			items = append(items, nodeToValue(node))
		}
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func orderedXMLFieldNames(fields []XMLFieldSpec) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" {
			continue
		}
		names = append(names, field.Name)
	}
	return names
}

func limitedParseErrors(errors []ParseError, max int) []ParseError {
	if max <= 0 || len(errors) == 0 {
		return nil
	}
	if len(errors) <= max {
		return append([]ParseError(nil), errors...)
	}
	return append([]ParseError(nil), errors[:max]...)
}
