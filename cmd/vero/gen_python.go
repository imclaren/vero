package main

import (
	"fmt"
	"strings"
)

// pythonAPI is the Python module: a dataclass for each type, and a Worker
// class with a method for each request, which takes the request's fields
// as keyword arguments.
//
// Every field decodes with a default when it is missing or null, so that
// a worker older than the app still works.
func pythonAPI(api *API) string {
	var b strings.Builder
	al := map[string]*TypeRef{}
	for _, t := range api.Types {
		if t.Alias != nil {
			al[t.Name] = t.Alias
		}
	}
	b.WriteString(docLines(genHeader, "# "))
	b.WriteString(`
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Callable


def _value(default: Any) -> Callable[[Any], Any]:
    return lambda v: default if v is None else v


def _list(f: Callable[[Any], Any]) -> Callable[[Any], Any]:
    return lambda v: [f(x) for x in (v or [])]


def _map(f: Callable[[Any], Any]) -> Callable[[Any], Any]:
    return lambda v: {k: f(x) for k, x in (v or {}).items()}


def _optional(f: Callable[[Any], Any]) -> Callable[[Any], Any]:
    return lambda v: None if v is None else f(v)


def _same(v: Any) -> Any:
    return v


def _json(v: Any) -> Any:
    return v.to_json()
`)
	for _, t := range api.Types {
		b.WriteString("\n\n")
		if t.Alias != nil {
			b.WriteString(docLines(t.Doc, "# "))
			fmt.Fprintf(&b, "%s = %s\n", t.Name, pythonType(t.Alias))
			continue
		}
		fmt.Fprintf(&b, "@dataclass\nclass %s:\n", t.Name)
		if t.Doc != "" {
			fmt.Fprintf(&b, "    %s\n\n", pythonDocstring(t.Doc, "    "))
		}
		for _, f := range t.Fields {
			fmt.Fprintf(&b, "    %s: %s = %s\n", pythonName(f.JSON), pythonType(f.Type), pythonDefault(f.Type, true, al))
			if f.Doc != "" {
				fmt.Fprintf(&b, "    %s\n", pythonDocstring(f.Doc, "    "))
			}
		}
		if len(t.Fields) == 0 {
			b.WriteString("    pass\n")
		}
		fmt.Fprintf(&b, "\n    @classmethod\n    def from_json(cls, d: Any) -> %s:\n        d = d or {}\n        return cls(\n", t.Name)
		for _, f := range t.Fields {
			fmt.Fprintf(&b, "            %s=%s(d.get(%q)),\n", pythonName(f.JSON), pythonDecoder(f.Type, al), f.JSON)
		}
		b.WriteString("        )\n\n    def to_json(self) -> dict[str, Any]:\n        return {\n")
		for _, f := range t.Fields {
			fmt.Fprintf(&b, "            %q: %s(self.%s),\n", f.JSON, pythonEncoder(f.Type, al), pythonName(f.JSON))
		}
		b.WriteString("        }\n")
	}

	b.WriteString("\n\nclass Worker:\n    \"\"\"The worker's requests, each a method, on a vero.Vero.\"\"\"\n\n    def __init__(self, vero: Any) -> None:\n        self.vero = vero\n")
	if api.State != nil {
		fmt.Fprintf(&b, "\n    def state(self, event: Any = None) -> %s:\n        \"\"\"The state the worker pushes: from an event, or the latest.\"\"\"\n        return %s(self.vero.latest() if event is None else event)\n",
			pythonType(api.State), pythonDecoder(api.State, al))
	}
	for _, r := range api.Requests {
		method := snake(r.Name)
		if method == "state" || method == "vero" || pythonKeywords[method] {
			method += "_request"
		}
		var params []string
		payload := "{}"
		switch {
		case r.Value != nil:
			params = append(params, "value: "+pythonType(r.Value))
			payload = pythonEncoder(r.Value, al) + "(value)"
		case len(r.Fields) > 0:
			params = append(params, "*")
			var items []string
			for _, f := range r.Fields {
				name := pythonName(f.JSON)
				if k := resolve(f.Type, al).Kind; k == "list" || k == "map" {
					params = append(params, fmt.Sprintf("%s: %s | None = None", name, pythonType(f.Type)))
					items = append(items, fmt.Sprintf("%q: %s(%s or %s)", f.JSON, pythonEncoder(f.Type, al), name, pythonDefault(f.Type, false, al)))
				} else {
					params = append(params, fmt.Sprintf("%s: %s = %s", name, pythonType(f.Type), pythonDefault(f.Type, false, al)))
					items = append(items, fmt.Sprintf("%q: %s(%s)", f.JSON, pythonEncoder(f.Type, al), name))
				}
			}
			payload = "{" + strings.Join(items, ", ") + "}"
		}
		sig := "self"
		if len(params) > 0 {
			sig += ", " + strings.Join(params, ", ")
		}
		fmt.Fprintf(&b, "\n    def %s(%s) -> %s:\n", method, sig, pythonType(r.Reply))
		if r.Doc != "" {
			fmt.Fprintf(&b, "        %s\n", pythonDocstring(r.Doc, "        "))
		}
		if resolve(r.Reply, al).Kind == "empty" {
			fmt.Fprintf(&b, "        self.vero.call(%q, %s)\n", r.Name, payload)
		} else {
			fmt.Fprintf(&b, "        return %s(self.vero.call(%q, %s))\n", pythonDecoder(r.Reply, al), r.Name, payload)
		}
	}
	return b.String()
}

func pythonType(t *TypeRef) string {
	switch t.Kind {
	case "bool":
		return "bool"
	case "int":
		return "int"
	case "float":
		return "float"
	case "string":
		return "str"
	case "json":
		return "Any"
	case "empty":
		return "None"
	case "list":
		return "list[" + pythonType(t.Elem) + "]"
	case "map":
		return "dict[str, " + pythonType(t.Elem) + "]"
	case "optional":
		return pythonType(t.Elem) + " | None"
	case "named":
		return t.Name
	}
	return "Any"
}

// pythonDefault is a field's default: in a dataclass, lists and maps need
// a factory.
func pythonDefault(t *TypeRef, dataclass bool, al map[string]*TypeRef) string {
	t = resolve(t, al)
	switch t.Kind {
	case "bool":
		return "False"
	case "int":
		return "0"
	case "float":
		return "0.0"
	case "string":
		if t.Time {
			return `"` + goZeroTime + `"`
		}
		return `""`
	case "json", "optional", "empty":
		return "None"
	case "list":
		if dataclass {
			return "field(default_factory=list)"
		}
		return "[]"
	case "map":
		if dataclass {
			return "field(default_factory=dict)"
		}
		return "{}"
	case "named":
		if dataclass {
			return "field(default_factory=lambda: " + t.Name + ".from_json(None))"
		}
		return t.Name + ".from_json(None)"
	}
	return "None"
}

func pythonDecoder(t *TypeRef, al map[string]*TypeRef) string {
	t = resolve(t, al)
	switch t.Kind {
	case "list":
		return "_list(" + pythonDecoder(t.Elem, al) + ")"
	case "map":
		return "_map(" + pythonDecoder(t.Elem, al) + ")"
	case "optional":
		return "_optional(" + pythonDecoder(t.Elem, al) + ")"
	case "named":
		return t.Name + ".from_json"
	case "json", "empty":
		return "_same"
	}
	return "_value(" + pythonDefault(t, false, al) + ")"
}

func pythonEncoder(t *TypeRef, al map[string]*TypeRef) string {
	t = resolve(t, al)
	switch t.Kind {
	case "list":
		return "_list(" + pythonEncoder(t.Elem, al) + ")"
	case "map":
		return "_map(" + pythonEncoder(t.Elem, al) + ")"
	case "optional":
		return "_optional(" + pythonEncoder(t.Elem, al) + ")"
	case "named":
		return "_json"
	}
	return "_same"
}

func pythonDocstring(doc, indent string) string {
	doc = strings.NewReplacer(`\`, `\\`, `"""`, `\"\"\"`).Replace(doc)
	if strings.HasPrefix(doc, `"`) {
		doc = " " + doc
	}
	if strings.HasSuffix(doc, `"`) {
		doc += " "
	}
	lines := strings.Split(doc, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = indent + lines[i]
		}
	}
	if len(lines) == 1 {
		return `"""` + lines[0] + `"""`
	}
	return `"""` + strings.Join(lines, "\n") + "\n" + indent + `"""`
}

var pythonKeywords = map[string]bool{
	"False": true, "None": true, "True": true, "and": true, "as": true, "assert": true, "async": true,
	"await": true, "break": true, "class": true, "continue": true, "def": true, "del": true, "elif": true,
	"else": true, "except": true, "finally": true, "for": true, "from": true, "global": true, "if": true,
	"import": true, "in": true, "is": true, "lambda": true, "nonlocal": true, "not": true, "or": true,
	"pass": true, "raise": true, "return": true, "try": true, "while": true, "with": true, "yield": true,
	"self": true, "field": true,
}

// pythonName is a JSON key as a Python name: "fullSync" is "full_sync".
func pythonName(key string) string {
	name := snake(notIdent.ReplaceAllString(key, "_"))
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "_" + name
	}
	if pythonKeywords[name] {
		name += "_"
	}
	return name
}
