package main

import (
	"fmt"
	"strings"
)

// csharpAPI is the C# file: a class for each type and request, and an
// extension method on VeroClient for each request.
//
// Every property has a default, which a missing field leaves in place,
// and lists and maps read null as empty, so that a worker older than the
// app still works.
func csharpAPI(api *API, namespace string) string {
	var b strings.Builder
	b.WriteString(docLines(genHeader, "// "))
	b.WriteString("\n#nullable enable\nusing System.Collections.Generic;\nusing System.Text.Json;\nusing System.Text.Json.Serialization;\nusing System.Threading.Tasks;\nusing Vero;\n\n")
	fmt.Fprintf(&b, "namespace %s;\n", namespace)
	aliases := map[string]*TypeRef{}
	for _, t := range api.Types {
		if t.Alias != nil {
			aliases[t.Name] = t.Alias
		}
	}
	cs := func(t *TypeRef) string { return csharpType(t, aliases) }
	class := func(name, doc string, fields []*APIField) {
		b.WriteString("\n")
		b.WriteString(csharpDoc(doc, ""))
		fmt.Fprintf(&b, "public sealed class %s\n{\n", name)
		for _, f := range fields {
			prop := pascal(notIdent.ReplaceAllString(f.JSON, "_"))
			if prop == name || prop == "" || (prop[0] >= '0' && prop[0] <= '9') {
				prop = "Value" + prop
			}
			b.WriteString(csharpDoc(f.Doc, "    "))
			attrs := fmt.Sprintf("JsonPropertyName(%q)", f.JSON)
			if k := resolve(f.Type, aliases).Kind; k == "list" || k == "map" {
				attrs += fmt.Sprintf(", JsonConverter(typeof(VeroNullAsEmpty<%s>))", cs(f.Type))
			}
			fmt.Fprintf(&b, "    [%s]\n    public %s %s { get; set; } = %s;\n", attrs, cs(f.Type), prop, csharpDefault(f.Type, aliases))
		}
		b.WriteString("}\n")
	}
	for _, t := range api.Types {
		if t.Alias == nil {
			class(t.Name, t.Doc, t.Fields)
		}
	}
	for _, r := range api.Requests {
		if len(r.Fields) > 0 {
			class(pascal(r.Name)+"Request", r.Doc, r.Fields)
		}
	}

	b.WriteString("\n/// <summary>The worker's requests, each as a method on VeroClient.</summary>\npublic static class WorkerApi\n{\n")
	if api.State != nil {
		fmt.Fprintf(&b, "    /// <summary>The state the worker pushes, read from an event or from Latest().</summary>\n    public static %s ReadState(JsonElement state) => state.Deserialize<%s>() ?? new();\n\n", cs(api.State), cs(api.State))
	}
	for _, r := range api.Requests {
		b.WriteString(csharpDoc(r.Doc, "    "))
		method := pascal(r.Name) + "Async"
		params, arg := "", "new { }"
		switch {
		case r.Value != nil:
			params, arg = ", "+cs(r.Value)+" value", "value"
		case len(r.Fields) > 0:
			params, arg = ", "+pascal(r.Name)+"Request request", "request"
		}
		call := fmt.Sprintf("vero.CallAsync(%q, %s)", r.Name, arg)
		switch rep := resolve(r.Reply, aliases); rep.Kind {
		case "empty":
			fmt.Fprintf(&b, "    public static async Task %s(this VeroClient vero%s) => await %s.ConfigureAwait(false);\n\n", method, params, call)
		case "json":
			fmt.Fprintf(&b, "    public static async Task<JsonElement> %s(this VeroClient vero%s) => await %s.ConfigureAwait(false) ?? default;\n\n", method, params, call)
		default:
			fmt.Fprintf(&b, "    public static async Task<%s> %s(this VeroClient vero%s) => Read<%s>(await %s.ConfigureAwait(false))%s;\n\n",
				cs(r.Reply), method, params, cs(r.Reply), call, csharpReadDefault(r.Reply, aliases))
		}
	}
	b.WriteString(`    static T? Read<T>(JsonElement? reply) =>
        reply is null || reply.Value.ValueKind == JsonValueKind.Null ? default : reply.Value.Deserialize<T>();
}

/// <summary>Reads null as an empty list or map, as Go writes an empty one.</summary>
public sealed class VeroNullAsEmpty<T> : JsonConverter<T> where T : new()
{
    public override bool HandleNull => true;
    public override T Read(ref Utf8JsonReader reader, System.Type type, JsonSerializerOptions options) =>
        reader.TokenType == JsonTokenType.Null ? new T() : JsonSerializer.Deserialize<T>(ref reader, options) ?? new T();
    public override void Write(Utf8JsonWriter writer, T value, JsonSerializerOptions options) =>
        JsonSerializer.Serialize(writer, value, options);
}
`)
	return b.String()
}

// resolve is a type with any other name for a scalar replaced by the
// scalar, since C# has no such names across files.
func resolve(t *TypeRef, aliases map[string]*TypeRef) *TypeRef {
	if t.Kind == "named" && aliases[t.Name] != nil {
		return aliases[t.Name]
	}
	return t
}

func csharpType(t *TypeRef, aliases map[string]*TypeRef) string {
	t = resolve(t, aliases)
	switch t.Kind {
	case "bool":
		return "bool"
	case "int":
		return "long"
	case "float":
		return "double"
	case "string":
		return "string"
	case "json", "empty":
		return "JsonElement"
	case "list":
		return "List<" + csharpType(t.Elem, aliases) + ">"
	case "map":
		return "Dictionary<string, " + csharpType(t.Elem, aliases) + ">"
	case "optional":
		return csharpType(t.Elem, aliases) + "?"
	case "named":
		return t.Name
	}
	return "JsonElement"
}

func csharpDefault(t *TypeRef, aliases map[string]*TypeRef) string {
	switch resolve(t, aliases).Kind {
	case "bool":
		return "false"
	case "int", "float":
		return "0"
	case "string":
		return `""`
	case "json", "empty", "optional":
		return "default"
	}
	return "new()"
}

// csharpReadDefault is what a reply of null becomes.
func csharpReadDefault(t *TypeRef, aliases map[string]*TypeRef) string {
	switch resolve(t, aliases).Kind {
	case "optional":
		return ""
	case "string":
		return ` ?? ""`
	case "bool", "int", "float":
		return ""
	}
	return " ?? new()"
}

func csharpDoc(doc, indent string) string {
	if doc == "" {
		return ""
	}
	doc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(doc)
	return indent + "/// <summary>\n" + docLines(doc, indent+"/// ") + indent + "/// </summary>\n"
}
