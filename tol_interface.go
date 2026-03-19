package lua

import (
	"fmt"
	"strconv"
	"strings"

	tolast "github.com/tos-network/tolang/tol/ast"
)

// InterfaceOptions configures .abi textual generation.
type InterfaceOptions struct {
	InterfaceName string // interface name to emit (defaults to "I"+ContractName)
	ContractName  string // which contract to render (defaults to first contract in module)
}

// CompileInterface compiles TOL source into a textual .abi interface declaration.
func CompileInterface(source []byte, name string) ([]byte, error) {
	return CompileInterfaceWithOptions(source, name, nil)
}

// CompileInterfaceWithOptions compiles TOL source into textual .abi with options.
func CompileInterfaceWithOptions(source []byte, name string, opts *InterfaceOptions) ([]byte, error) {
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	return BuildInterfaceWithOptions(mod, opts)
}

// BuildInterface renders a parsed module into a textual interface declaration.
func BuildInterface(mod *tolast.Module) ([]byte, error) {
	return BuildInterfaceWithOptions(mod, nil)
}

// BuildInterfaceWithOptions renders a parsed module into textual interface declaration.
// When opts.ContractName is set, that contract is used; otherwise the first contract.
func BuildInterfaceWithOptions(mod *tolast.Module, opts *InterfaceOptions) ([]byte, error) {
	if mod == nil {
		return nil, fmt.Errorf("interface build requires a module")
	}
	// Select which contract to render.
	var c *tolast.ContractDecl
	if opts != nil && strings.TrimSpace(opts.ContractName) != "" {
		wantName := strings.TrimSpace(opts.ContractName)
		for i := range mod.Contracts {
			if mod.Contracts[i].Name == wantName {
				c = &mod.Contracts[i]
				break
			}
		}
		if c == nil {
			return nil, fmt.Errorf("interface build: contract %q not found in module", wantName)
		}
	} else {
		c = mod.PrimaryContract()
	}
	if c == nil {
		return nil, fmt.Errorf("interface build requires a contract declaration")
	}
	return buildInterfaceForContract(mod, c, opts)
}

// BuildInterfaceForContract renders a specific ContractDecl as a .abi interface.
func BuildInterfaceForContract(mod *tolast.Module, c *tolast.ContractDecl, interfaceName string) ([]byte, error) {
	return buildInterfaceForContract(mod, c, &InterfaceOptions{InterfaceName: interfaceName})
}

// BuildInterfaceForDecl renders a top-level InterfaceDecl as a .abi file.
func BuildInterfaceForDecl(mod *tolast.Module, iface *tolast.InterfaceDecl) ([]byte, error) {
	if iface == nil {
		return nil, fmt.Errorf("interface build: nil interface")
	}
	version := "0.2"
	if mod != nil && strings.TrimSpace(mod.Version) != "" {
		version = strings.TrimSpace(mod.Version)
	}
	var b strings.Builder
	b.WriteString("pragma tolang ")
	b.WriteString(version)
	b.WriteString(";\n\ninterface ")
	b.WriteString(strings.TrimSpace(iface.Name))
	b.WriteString(" {\n")
	for _, fn := range iface.Functions {
		b.WriteString("  function ")
		b.WriteString(strings.TrimSpace(fn.Name))
		b.WriteString("(")
		b.WriteString(renderInterfaceFields(fn.Params, "arg"))
		b.WriteString(")")
		for _, m := range fn.Modifiers {
			modifier := strings.TrimSpace(m)
			if modifier == "" {
				continue
			}
			b.WriteString(" ")
			b.WriteString(modifier)
		}
		if len(fn.Returns) > 0 {
			b.WriteString(" returns (")
			b.WriteString(renderInterfaceFields(fn.Returns, "ret"))
			b.WriteString(")")
		}
		b.WriteString(";\n")
	}
	for _, ev := range iface.Events {
		b.WriteString("  event ")
		b.WriteString(strings.TrimSpace(ev.Name))
		b.WriteString("(")
		for i, p := range ev.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(renderInterfaceEventField(p.Name, p.Type, p.Indexed, fmt.Sprintf("arg%d", i+1)))
		}
		b.WriteString(");\n")
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

// buildInterfaceForContract is the internal renderer for a single ContractDecl.
func buildInterfaceForContract(mod *tolast.Module, c *tolast.ContractDecl, opts *InterfaceOptions) ([]byte, error) {
	contractName := strings.TrimSpace(c.Name)
	if contractName == "" {
		return nil, fmt.Errorf("interface build requires a non-empty contract name")
	}
	version := "0.2"
	if mod != nil && strings.TrimSpace(mod.Version) != "" {
		version = strings.TrimSpace(mod.Version)
	}
	interfaceName := "I" + contractName
	if opts != nil && strings.TrimSpace(opts.InterfaceName) != "" {
		interfaceName = strings.TrimSpace(opts.InterfaceName)
	}

	var b strings.Builder
	b.WriteString("pragma tolang ")
	b.WriteString(version)
	b.WriteString(";\n\ninterface ")
	b.WriteString(interfaceName)
	b.WriteString(" {\n")

	for _, fn := range c.Functions {
		vis := functionVisibilityFromModifiers(fn.Modifiers)
		if vis != "public" && vis != "external" {
			continue
		}
		if strings.TrimSpace(fn.SelectorOverride) != "" {
			b.WriteString("  @selector(")
			b.WriteString(fmt.Sprintf("%q", fn.SelectorOverride))
			b.WriteString(")\n")
		}
		b.WriteString("  function ")
		b.WriteString(strings.TrimSpace(fn.Name))
		b.WriteString("(")
		b.WriteString(renderInterfaceFields(fn.Params, "arg"))
		b.WriteString(")")
		for _, m := range fn.Modifiers {
			modifier := strings.TrimSpace(m)
			if modifier == "" {
				continue
			}
			b.WriteString(" ")
			b.WriteString(modifier)
		}
		if len(fn.Returns) > 0 {
			b.WriteString(" returns (")
			b.WriteString(renderInterfaceFields(fn.Returns, "ret"))
			b.WriteString(")")
		}
		b.WriteString(";\n")
	}

	for _, ev := range c.Events {
		b.WriteString("  event ")
		b.WriteString(strings.TrimSpace(ev.Name))
		b.WriteString("(")
		for i, p := range ev.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(renderInterfaceEventField(p.Name, p.Type, p.Indexed, fmt.Sprintf("arg%d", i+1)))
		}
		b.WriteString(");\n")
	}

	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func renderInterfaceFields(fields []tolast.FieldDecl, fallbackPrefix string) string {
	if len(fields) == 0 {
		return ""
	}
	out := make([]string, 0, len(fields))
	for i, f := range fields {
		out = append(out, renderInterfaceField(f.Name, f.Type, fmt.Sprintf("%s%d", fallbackPrefix, i+1)))
	}
	return strings.Join(out, ", ")
}

func renderInterfaceField(name, typ, fallbackName string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = fallbackName
	}
	return fmt.Sprintf("%s %s", normalizeABIType(typ), n)
}

func renderInterfaceEventField(name, typ string, indexed bool, fallbackName string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		n = fallbackName
	}
	s := fmt.Sprintf("%s %s", normalizeABIType(typ), n)
	if indexed {
		s += " indexed"
	}
	return s
}

// ValidateInterface performs a lightweight structural validation for textual .abi content.
func ValidateInterface(data []byte) error {
	_, err := parseInterfaceInfo(data)
	return err
}

// InterfaceInfo is lightweight metadata extracted from textual .abi content.
type InterfaceInfo struct {
	Version       string
	InterfaceName string
	FunctionCount int
	EventCount    int
}

// InspectInterface validates and extracts lightweight metadata from textual .abi content.
func InspectInterface(data []byte) (*InterfaceInfo, error) {
	return parseInterfaceInfo(data)
}

func parseInterfaceInfo(data []byte) (*InterfaceInfo, error) {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return nil, fmt.Errorf("interface text is empty")
	}
	lines := strings.Split(s, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("interface text is empty")
	}

	first := ""
	start := 0
	for i, raw := range lines {
		line := normalizeInterfaceLine(raw)
		if line == "" {
			continue
		}
		first = line
		start = i + 1
		break
	}
	if first == "" {
		return nil, fmt.Errorf("interface text is empty")
	}
	if !strings.HasPrefix(first, "pragma ") {
		return nil, fmt.Errorf("interface text must start with 'pragma <lang> <version>;' header")
	}
	// Accept: pragma tolang <version>; OR pragma solidity <version>; etc.
	// Extract the last whitespace-separated token before ';' as the version.
	rest := strings.TrimSpace(strings.TrimPrefix(first, "pragma "))
	rest = strings.TrimSuffix(rest, ";")
	parts := strings.Fields(rest)
	version := ""
	if len(parts) > 0 {
		version = parts[len(parts)-1]
	}
	if version == "" {
		return nil, fmt.Errorf("interface header missing version")
	}

	info := &InterfaceInfo{Version: version}
	seenInterface := false
	inInterface := false
	selectorPending := false
	fnSeen := map[string]struct{}{}
	eventSeen := map[string]struct{}{}

	for i := start; i < len(lines); i++ {
		line := normalizeInterfaceLine(lines[i])
		if line == "" {
			continue
		}
		if !seenInterface {
			if !strings.HasPrefix(line, "interface ") {
				return nil, fmt.Errorf("interface text must contain interface declaration")
			}
			nameAndBrace := strings.TrimSpace(strings.TrimPrefix(line, "interface "))
			if !strings.HasSuffix(nameAndBrace, "{") {
				return nil, fmt.Errorf("interface declaration must end with '{'")
			}
			name := strings.TrimSpace(strings.TrimSuffix(nameAndBrace, "{"))
			if name == "" {
				return nil, fmt.Errorf("interface name not found")
			}
			info.InterfaceName = name
			seenInterface = true
			inInterface = true
			continue
		}
		if !inInterface {
			return nil, fmt.Errorf("interface text has unexpected content after interface block")
		}

		if line == "}" {
			inInterface = false
			continue
		}
		if strings.HasPrefix(line, "interface ") {
			return nil, fmt.Errorf("interface text supports exactly one interface declaration")
		}
		if strings.HasPrefix(line, "@selector(") {
			if selectorPending {
				return nil, fmt.Errorf("interface selector annotation must be followed by function declaration")
			}
			if err := validateSelectorAnnotation(line); err != nil {
				return nil, err
			}
			selectorPending = true
			continue
		}
		if strings.HasPrefix(line, "function ") || strings.HasPrefix(line, "fn ") {
			if !strings.HasSuffix(line, ";") {
				return nil, fmt.Errorf("interface function declaration must end with ';'")
			}
			if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
				return nil, fmt.Errorf("interface function declaration must contain parameter list")
			}
			name, err := parseInterfaceFnName(line)
			if err != nil {
				return nil, err
			}
			if _, exists := fnSeen[name]; exists {
				return nil, fmt.Errorf("interface duplicate function declaration '%s'", name)
			}
			if _, exists := eventSeen[name]; exists {
				return nil, fmt.Errorf("interface name collision between function and event '%s'", name)
			}
			fnSeen[name] = struct{}{}
			info.FunctionCount++
			selectorPending = false
			continue
		}
		if strings.HasPrefix(line, "event ") {
			if selectorPending {
				return nil, fmt.Errorf("interface selector annotation must be followed by function declaration")
			}
			if !strings.HasSuffix(line, ";") {
				return nil, fmt.Errorf("interface event declaration must end with ';'")
			}
			if !strings.Contains(line, "(") || !strings.Contains(line, ")") {
				return nil, fmt.Errorf("interface event declaration must contain parameter list")
			}
			indexedCount, err := countEventIndexedFields(line)
			if err != nil {
				return nil, err
			}
			if indexedCount > 3 {
				return nil, fmt.Errorf("interface event declaration has %d indexed field(s); at most 3 are allowed", indexedCount)
			}
			name, err := parseInterfaceEventName(line)
			if err != nil {
				return nil, err
			}
			if _, exists := eventSeen[name]; exists {
				return nil, fmt.Errorf("interface duplicate event declaration '%s'", name)
			}
			if _, exists := fnSeen[name]; exists {
				return nil, fmt.Errorf("interface name collision between event and function '%s'", name)
			}
			eventSeen[name] = struct{}{}
			info.EventCount++
			continue
		}
		return nil, fmt.Errorf("interface block contains unsupported line: %q", line)
	}

	if !seenInterface {
		return nil, fmt.Errorf("interface text must contain interface declaration")
	}
	if inInterface {
		return nil, fmt.Errorf("interface text has unbalanced braces")
	}
	if selectorPending {
		return nil, fmt.Errorf("interface selector annotation must be followed by function declaration")
	}
	return info, nil
}

func normalizeInterfaceLine(raw string) string {
	line := strings.TrimSpace(raw)
	if line == "" {
		return ""
	}
	if idx := strings.Index(line, "--"); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	return line
}

func countEventIndexedFields(line string) (int, error) {
	open := strings.Index(line, "(")
	close := strings.LastIndex(line, ")")
	if open < 0 || close <= open {
		return 0, fmt.Errorf("interface event declaration must contain parameter list")
	}
	params := strings.TrimSpace(line[open+1 : close])
	if params == "" {
		return 0, nil
	}
	count := 0
	for _, raw := range strings.Split(params, ",") {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if strings.HasSuffix(p, " indexed") {
			count++
		}
	}
	return count, nil
}

func validateSelectorAnnotation(line string) error {
	if !strings.HasPrefix(line, "@selector(") || !strings.HasSuffix(line, ")") {
		return fmt.Errorf("interface selector annotation must end with ')'")
	}
	raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "@selector("), ")"))
	if raw == "" {
		return fmt.Errorf("interface selector annotation requires string literal value")
	}
	v, err := strconv.Unquote(raw)
	if err != nil {
		return fmt.Errorf("interface selector annotation requires string literal value")
	}
	if !isValidSelectorHex(v) {
		return fmt.Errorf("interface selector annotation must be 0x followed by 8 hex chars")
	}
	return nil
}

func isValidSelectorHex(v string) bool {
	if len(v) != 10 || !strings.HasPrefix(v, "0x") {
		return false
	}
	for i := 2; i < len(v); i++ {
		ch := v[i]
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return false
	}
	return true
}

func parseInterfaceFnName(line string) (string, error) {
	var rest string
	if strings.HasPrefix(line, "function ") {
		rest = strings.TrimSpace(strings.TrimPrefix(line, "function "))
	} else {
		rest = strings.TrimSpace(strings.TrimPrefix(line, "fn "))
	}
	open := strings.Index(rest, "(")
	if open <= 0 {
		return "", fmt.Errorf("interface function declaration must contain valid function name")
	}
	name := strings.TrimSpace(rest[:open])
	if name == "" {
		return "", fmt.Errorf("interface function declaration must contain valid function name")
	}
	return name, nil
}

func parseInterfaceEventName(line string) (string, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "event "))
	open := strings.Index(rest, "(")
	if open <= 0 {
		return "", fmt.Errorf("interface event declaration must contain valid event name")
	}
	name := strings.TrimSpace(rest[:open])
	if name == "" {
		return "", fmt.Errorf("interface event declaration must contain valid event name")
	}
	return name, nil
}
