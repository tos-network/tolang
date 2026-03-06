package lua

import (
	"fmt"
	"strings"
)

func parseAgentString(raw string) (LAgent, error) {
	norm := strings.ToLower(strings.TrimSpace(raw))
	if !strings.HasPrefix(norm, "0x") {
		return "", fmt.Errorf("expected address with 0x prefix")
	}
	hex := norm[2:]
	if len(hex) != 64 {
		return "", fmt.Errorf("expected address with 64 hex chars")
	}
	for _, ch := range hex {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return "", fmt.Errorf("invalid address hex string")
		}
	}
	return LAgent("0x" + hex), nil
}

func parseAgentValue(v LValue) (LAgent, error) {
	switch lv := v.(type) {
	case LAgent:
		return parseAgentString(string(lv))
	case LString:
		return parseAgentString(string(lv))
	default:
		return "", fmt.Errorf("expected address string")
	}
}
