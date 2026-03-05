package ast

type Field struct {
	Key   Expr
	Value Expr
}

type LocalAttr uint8

const (
	LocalAttrNone LocalAttr = iota
	LocalAttrConst
	LocalAttrClose
)

type LocalName struct {
	Name string
	Attr LocalAttr
}

type ParList struct {
	HasVargs bool
	Names    []string
}

type FuncName struct {
	Func     Expr
	Receiver Expr
	Method   string
}
