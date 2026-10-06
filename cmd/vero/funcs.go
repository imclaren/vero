package main

import (
	"strings"
	"text/template"
	"unicode"
)

var funcs = template.FuncMap{
	"pascal": pascal, "camel": camel, "lower": strings.ToLower,
	"swiftType": swiftType, "swiftReqType": swiftReqType, "csType": csType, "ktType": ktType,
	"pkgPath": func(id string) string { return strings.ReplaceAll(id, ".", "/") },
	"ident":   ident,
	"join":    strings.Join,
}

// pascal is "restartJob" or "restart-job" as "RestartJob".
func pascal(s string) string {
	var b strings.Builder
	up := true
	for _, r := range s {
		if r == '-' || r == '_' || r == ' ' || r == '.' {
			up = true
			continue
		}
		if up {
			b.WriteRune(unicode.ToUpper(r))
			up = false
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func camel(s string) string {
	p := pascal(s)
	if p == "" {
		return p
	}
	return strings.ToLower(p[:1]) + p[1:]
}

// ident is a name safe as an identifier: letters, digits and underscores.
func ident(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func swiftType(f Field) string {
	switch f.Kind {
	case "string":
		return "String"
	case "int":
		return "Int"
	case "float":
		return "Double"
	case "bool":
		return "Bool"
	case "struct":
		return f.Elem
	case "list":
		if f.Elem != "" {
			return "[" + f.Elem + "]"
		}
		return "[JSONValue]"
	}
	return "JSONValue"
}

func csType(f Field) string {
	switch f.Kind {
	case "string":
		return "string?"
	case "int":
		return "long"
	case "float":
		return "double"
	case "bool":
		return "bool"
	case "struct":
		return f.Elem + "?"
	case "list":
		if f.Elem != "" {
			return f.Elem + "[]?"
		}
		return "System.Text.Json.JsonElement[]?"
	}
	return "System.Text.Json.JsonElement?"
}

func ktType(f Field) string {
	switch f.Kind {
	case "string":
		return "String"
	case "int":
		return "Long"
	case "float":
		return "Double"
	case "bool":
		return "Boolean"
	}
	return "Any?"
}

// swiftReqType is a request field's type: scalars as they are, and
// anything else as JSON, which the starter sends as null.
func swiftReqType(f Field) string {
	if f.Scalar {
		return swiftType(f)
	}
	return "JSONValue"
}
