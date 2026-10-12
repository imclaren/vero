package main

import (
	"fmt"
	"regexp"
	"strings"
)

const genHeader = "Written by vero gen from the worker's own types. Do not edit it: change\nthe worker, then run vero gen."

// swiftAPI is the Swift file: a struct for each type, and one conforming
// to vero's NamedRequest for each request, which Vero's call sends.
//
// Every field decodes with a default when it is missing or null, so that
// a worker older than the app, which lacks a newer field, still works.
func swiftAPI(api *API) string {
	var b strings.Builder
	b.WriteString(docLines(genHeader, "// "))
	b.WriteString("\nimport Foundation\nimport Vero\n")
	for _, t := range api.Types {
		b.WriteString("\n")
		b.WriteString(docLines(t.Doc, "/// "))
		if t.Alias != nil {
			fmt.Fprintf(&b, "typealias %s = %s\n", t.Name, genSwiftType(t.Alias))
			continue
		}
		fmt.Fprintf(&b, "struct %s: Codable, Equatable, Sendable {\n", t.Name)
		swiftFields(&b, t.Fields)
		b.WriteString("}\n")
		fmt.Fprintf(&b, "\nextension %s {\n", t.Name)
		b.WriteString("    init(from decoder: Decoder) throws {\n")
		if len(t.Fields) > 0 {
			b.WriteString("        let c = try decoder.container(keyedBy: CodingKeys.self)\n")
		}
		for _, f := range t.Fields {
			name := swiftIdent(swiftName(f.JSON))
			if f.Type.Kind == "optional" {
				fmt.Fprintf(&b, "        %s = try c.decodeIfPresent(%s.self, forKey: .%s)\n", name, genSwiftType(f.Type.Elem), name)
			} else {
				fmt.Fprintf(&b, "        %s = try c.decodeIfPresent(%s.self, forKey: .%s) ?? %s\n", name, genSwiftType(f.Type), name, swiftDefault(f.Type))
			}
		}
		b.WriteString("    }\n}\n")
	}
	for _, r := range api.Requests {
		b.WriteString("\n")
		b.WriteString(docLines(r.Doc, "/// "))
		fmt.Fprintf(&b, "struct %sRequest: NamedRequest {\n", pascal(r.Name))
		fmt.Fprintf(&b, "    typealias Reply = %s\n", swiftReply(r.Reply))
		fmt.Fprintf(&b, "    static let name = %q\n", r.Name)
		switch {
		case r.Value != nil:
			fmt.Fprintf(&b, "    var value: %s = %s\n", genSwiftType(r.Value), swiftDefault(r.Value))
			b.WriteString("    func encode(to encoder: Encoder) throws {\n        var c = encoder.singleValueContainer()\n        try c.encode(value)\n    }\n")
		case len(r.Fields) > 0:
			swiftFields(&b, r.Fields)
		}
		b.WriteString("}\n")
	}
	return b.String()
}

func swiftFields(b *strings.Builder, fields []*APIField) {
	for _, f := range fields {
		b.WriteString(docLines(f.Doc, "    /// "))
		fmt.Fprintf(b, "    var %s: %s = %s\n", swiftIdent(swiftName(f.JSON)), genSwiftType(f.Type), swiftDefault(f.Type))
	}
	if len(fields) == 0 {
		return
	}
	b.WriteString("\n    enum CodingKeys: String, CodingKey {\n")
	for _, f := range fields {
		name := swiftName(f.JSON)
		if name == f.JSON {
			fmt.Fprintf(b, "        case %s\n", swiftIdent(name))
		} else {
			fmt.Fprintf(b, "        case %s = %q\n", swiftIdent(name), f.JSON)
		}
	}
	b.WriteString("    }\n")
}

func genSwiftType(t *TypeRef) string {
	switch t.Kind {
	case "bool":
		return "Bool"
	case "int":
		return "Int"
	case "float":
		return "Double"
	case "string":
		return "String"
	case "json":
		return "JSONValue"
	case "empty":
		return "EmptyReply"
	case "list":
		return "[" + genSwiftType(t.Elem) + "]"
	case "map":
		return "[String: " + genSwiftType(t.Elem) + "]"
	case "optional":
		return genSwiftType(t.Elem) + "?"
	case "named":
		return t.Name
	}
	return "JSONValue"
}

func swiftReply(t *TypeRef) string { return genSwiftType(t) }

func swiftDefault(t *TypeRef) string {
	switch t.Kind {
	case "bool":
		return "false"
	case "int", "float":
		return "0"
	case "string":
		return `""`
	case "json":
		return ".null"
	case "list":
		return "[]"
	case "map":
		return "[:]"
	case "optional":
		return "nil"
	}
	return genSwiftType(t) + "()"
}

var notIdent = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// swiftName is a JSON key as a Swift property: "client_id" is "clientId".
func swiftName(key string) string {
	name := lowerCamel(notIdent.ReplaceAllString(key, "_"))
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "_" + name
	}
	return name
}

var swiftKeywords = map[string]bool{
	"associatedtype": true, "class": true, "deinit": true, "enum": true, "extension": true, "fileprivate": true,
	"func": true, "import": true, "init": true, "inout": true, "internal": true, "let": true, "open": true,
	"operator": true, "private": true, "protocol": true, "public": true, "rethrows": true, "static": true,
	"struct": true, "subscript": true, "typealias": true, "var": true, "break": true, "case": true,
	"continue": true, "default": true, "defer": true, "do": true, "else": true, "fallthrough": true,
	"for": true, "guard": true, "if": true, "in": true, "repeat": true, "return": true, "switch": true,
	"where": true, "while": true, "as": true, "Any": true, "catch": true, "false": true, "is": true,
	"nil": true, "super": true, "self": true, "Self": true, "throw": true, "throws": true, "true": true,
	"try": true, "Type": true,
}

func swiftIdent(name string) string {
	if swiftKeywords[name] {
		return "`" + name + "`"
	}
	return name
}
