package lua

import (
	"fmt"
	"strings"
)

func parseAddressString(raw string) (LAddress, error) {
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
	return LAddress("0x" + hex), nil
}

func parseAddressValue(v LValue) (LAddress, error) {
	switch lv := v.(type) {
	case LAddress:
		return parseAddressString(string(lv))
	case LString:
		return parseAddressString(string(lv))
	default:
		return "", fmt.Errorf("expected address string")
	}
}
