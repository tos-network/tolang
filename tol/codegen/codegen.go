package codegen

import (
	"fmt"

	lua "github.com/tos-network/tolang"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/lower"
)

func Bytecode(p *lower.Program) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("[%s] invalid lowered program", diag.CodeCodegenNotImplemented)
	}
	return lua.CompileBytecodeFromLowered(p, p.ContractName)
}
