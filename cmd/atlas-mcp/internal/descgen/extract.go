package descgen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"strings"
)

// ToolDesc is one tool's extracted metadata for auto-generated MCP tool
// descriptions. InputSchema is the JSON Schema as a Go value (map[string]any
// for an object schema, nil for tools with no input). We deliberately use any
// instead of json.RawMessage so that the AST extractor and unit tests can
// operate on the schema as Go-typed values (e.g. []string for "required")
// without a json.Marshal/Unmarshal round trip that would lose slice element
// types. The schema's runtime invariant (always map[string]any or nil) is
// enforced by buildSchema() in extract.go and the schemaMap() test helper.
type ToolDesc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

func Extract(toolsDir string) (map[string]ToolDesc, error) {
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", toolsDir, err)
	}

	fset := token.NewFileSet()
	var files []*ast.File
	fileComments := make(map[string]ast.CommentMap)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		isToolsFile := name == "tools.go" || (strings.HasPrefix(name, "tools_") && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")) ||
			name == "sampling.go" || name == "roots.go" || name == "elicitation.go"
		if !isToolsFile {
			continue
		}
		path := filepath.Join(toolsDir, name)
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		files = append(files, f)
		fileComments[path] = ast.NewCommentMap(fset, f, f.Comments)
	}

	structs := make(map[string][]structField)
	handlerTypes := make(map[string]string)
	handlerDoc := make(map[string]string)

	for _, f := range files {
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.TYPE {
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					fields := collectFields(st)
					if len(fields) > 0 || st.Fields.NumFields() == 0 {
						structs[ts.Name.Name] = fields
					}
				}
			}

			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil {
				if len(fn.Recv.List) == 0 {
					continue
				}
				recv := fn.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					if ident, ok := star.X.(*ast.Ident); ok && ident.Name == "server" {
						name := fn.Name.Name
						handlerTypes[name] = extractInputType(fn)
						handlerDoc[name] = fn.Doc.Text()
					}
				}
			}
		}
	}

	result := make(map[string]ToolDesc)
	for _, f := range files {
		cmap := fileComments[fset.File(f.Pos()).Name()]
		ast.Inspect(f, func(n ast.Node) bool {
			ce, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// Match both countedAddTool(...) (new wrapper) and mcp.AddTool(...) (legacy pattern).
			switch fun := ce.Fun.(type) {
			case *ast.SelectorExpr:
				if fun.Sel.Name != "AddTool" {
					return true
				}
			case *ast.Ident:
				if fun.Name != "countedAddTool" {
					return true
				}
			default:
				return true
			}
			if len(ce.Args) < 3 {
				return true
			}

			toolName := extractToolName(ce.Args[1])
			if toolName == "" {
				return true
			}

			handlerName := extractHandlerName(ce.Args[len(ce.Args)-1])
			if handlerName == "" {
				return true
			}

			if strings.Contains(handlerDoc[handlerName], "gen:manual-override") {
				return true
			}
			if hasManualOverride(ce, cmap) {
				return true
			}

			existingDesc := extractToolDescription(ce.Args[1])

			desc := ToolDesc{
				Name:        toolName,
				Description: buildDescription(toolName, existingDesc, handlerDoc[handlerName]),
			}

			inputType := handlerTypes[handlerName]
			desc.InputSchema = buildSchema(inputType, structs)

			result[toolName] = desc
			return true
		})
	}

	return result, nil
}

type structField struct {
	Name       string
	JSONName   string
	GoType     ast.Expr
	Tag        string
	Omitempty  bool
	Jsonschema string
	Enum       string
}

func collectFields(st *ast.StructType) []structField {
	var fields []structField
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue
		}
		tag := ""
		if f.Tag != nil {
			tag = f.Tag.Value
		}
		jsonName := extractTagValue(tag, "json")
		if jsonName == "-" {
			continue
		}
		if jsonName == "" {
			jsonName = f.Names[0].Name
		}
		cleanName := jsonName
		if comma := strings.IndexByte(jsonName, ','); comma >= 0 {
			cleanName = jsonName[:comma]
		}
		omitempty := strings.Contains(tag, ",omitempty")

		sf := structField{
			Name:       f.Names[0].Name,
			JSONName:   cleanName,
			GoType:     f.Type,
			Tag:        tag,
			Omitempty:  omitempty,
			Jsonschema: extractTagValue(tag, "jsonschema"),
			Enum:       extractTagValue(tag, "enum"),
		}
		fields = append(fields, sf)
	}
	return fields
}

func extractTagValue(rawTag, key string) string {
	tag := strings.Trim(rawTag, "`")
	prefix := key + ":\""
	_, after, ok := strings.Cut(tag, prefix)
	if !ok {
		return ""
	}
	rest := after
	before0, _, ok0 := strings.Cut(rest, "\"")
	if !ok0 {
		return ""
	}
	return before0
}

func extractInputType(fn *ast.FuncDecl) string {
	params := fn.Type.Params.List
	if len(params) < 3 {
		return ""
	}
	paramType := params[2].Type
	if st, ok := paramType.(*ast.StructType); ok && st.Fields.NumFields() == 0 {
		return ""
	}
	return typeName(paramType)
}

func typeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.SelectorExpr:
		return typeName(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeName(t.Elt)
	default:
		return ""
	}
}

func extractToolName(arg ast.Expr) string {
	ue, ok := arg.(*ast.UnaryExpr)
	if !ok || ue.Op != token.AND {
		return ""
	}
	cl, ok := ue.X.(*ast.CompositeLit)
	if !ok {
		return ""
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Name" {
			continue
		}
		lit, ok := kv.Value.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		return strings.Trim(lit.Value, `"`)
	}
	return ""
}

func extractHandlerName(arg ast.Expr) string {
	sel, ok := arg.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return sel.Sel.Name
}

func extractToolDescription(arg ast.Expr) string {
	ue, ok := arg.(*ast.UnaryExpr)
	if !ok || ue.Op != token.AND {
		return ""
	}
	cl, ok := ue.X.(*ast.CompositeLit)
	if !ok {
		return ""
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "Description" {
			continue
		}
		if lit, ok := kv.Value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			return strings.Trim(lit.Value, `"`)
		}
		if ce, ok := kv.Value.(*ast.CallExpr); ok {
			if sel, ok := ce.Fun.(*ast.Ident); ok && sel.Name == "autoDescOr" {
				if len(ce.Args) >= 2 {
					if lit, ok := ce.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						return strings.Trim(lit.Value, `"`)
					}
				}
			}
		}
	}
	return ""
}

func hasManualOverride(ce *ast.CallExpr, cmap ast.CommentMap) bool {
	for node, cgs := range cmap {
		if node.Pos() == ce.Pos() || (node.Pos() < ce.Pos() && ce.Pos()-node.Pos() < 200) {
			for _, cg := range cgs {
				for _, c := range cg.List {
					if strings.Contains(c.Text, "gen:manual-override") {
						return true
					}
				}
			}
		}
	}
	return false
}

func buildDescription(toolName, existingDesc, godoc string) string {
	if existingDesc != "" {
		return existingDesc
	}
	if godoc != "" {
		lines := strings.Split(strings.TrimSpace(godoc), "\n")
		var clean []string
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if strings.Contains(l, "gen:manual-override") {
				continue
			}
			clean = append(clean, l)
		}
		if len(clean) > 0 {
			text := strings.Join(clean, " ")
			if len(text) > 200 {
				text = text[:200]
				if lastSpace := strings.LastIndexByte(text, ' '); lastSpace > 0 {
					text = text[:lastSpace]
				}
			}
			return text
		}
	}
	return deriveDescFromName(toolName)
}

func deriveDescFromName(name string) string {
	parts := strings.Split(name, "_")
	var words []string
	for _, p := range parts {
		if p == "get" || p == "list" {
			continue
		}
		words = append(words, p)
	}
	if len(words) == 0 {
		return name
	}
	return strings.Join(words, " ") + " data."
}

func buildSchema(inputType string, structs map[string][]structField) any {
	if inputType == "" {
		return map[string]any{"type": "object"}
	}

	base := inputType
	if dot := strings.LastIndex(inputType, "."); dot >= 0 {
		base = inputType[dot+1:]
	}

	fields, ok := structs[base]
	if !ok {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}

	props := make(map[string]any)
	var required []string

	for _, f := range fields {
		prop := fieldToSchema(f, structs)
		props[f.JSONName] = prop
		if !f.Omitempty {
			required = append(required, f.JSONName)
		}
	}

	schema := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func fieldToSchema(f structField, structs map[string][]structField) map[string]any {
	prop := map[string]any{}

	if f.Jsonschema != "" {
		prop["description"] = f.Jsonschema
	}

	if f.Enum != "" {
		enumVals := strings.Split(f.Enum, ",")
		prop["enum"] = enumVals
		prop["type"] = "string"
		return prop
	}

	propType, isArr, itemType := goTypeToJSONSchemaType(f.GoType, structs)
	if isArr {
		prop["type"] = "array"
		if itemType != nil {
			prop["items"] = itemType
		}
	} else if nested, ok := propType.(map[string]any); ok {
		maps.Copy(prop, nested)
	} else {
		prop["type"] = propType
	}
	return prop
}

func goTypeToJSONSchemaType(expr ast.Expr, structs map[string][]structField) (any, bool, any) {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string":
			return "string", false, nil
		case "bool":
			return "boolean", false, nil
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64":
			return "integer", false, nil
		case "float32", "float64":
			return "number", false, nil
		default:
			if _, ok := structs[t.Name]; ok {
				return buildNestedSchema(t.Name, structs), false, nil
			}
			return "string", false, nil
		}
	case *ast.StarExpr:
		return goTypeToJSONSchemaType(t.X, structs)
	case *ast.SelectorExpr:
		if ident, ok := t.X.(*ast.Ident); ok && ident.Name == "time" {
			switch t.Sel.Name {
			case "Duration":
				return "number", false, nil
			case "Time":
				return "string", false, nil
			}
		}
		return "string", false, nil
	case *ast.ArrayType:
		inner, isArr, itemType := goTypeToJSONSchemaType(t.Elt, structs)
		if isArr {
			return inner, true, itemType
		}
		return inner, true, map[string]any{"type": inner}
	case *ast.MapType:
		return "object", false, nil
	default:
		return "string", false, nil
	}
}

func buildNestedSchema(typeName string, structs map[string][]structField) map[string]any {
	fields, ok := structs[typeName]
	if !ok {
		return map[string]any{"type": "object"}
	}
	props := make(map[string]any)
	var required []string
	for _, f := range fields {
		prop := fieldToSchema(f, structs)
		props[f.JSONName] = prop
		if !f.Omitempty {
			required = append(required, f.JSONName)
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
