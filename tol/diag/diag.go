package diag

import "fmt"

// Severity distinguishes errors (which abort compilation) from warnings (informational only).
type Severity int

const (
	SeverityError   Severity = 0 // default zero value → error
	SeverityWarning Severity = 1
)

const (
	CodeParseUnexpected          = "TOL1001"
	CodeParseUnsupported         = "TOL1002"
	CodeSemaUnsupportedVer       = "TOL2001"
	CodeSemaMissingContract      = "TOL2002"
	CodeSemaDuplicateSlot        = "TOL2003"
	CodeSemaDuplicateFunction    = "TOL2004"
	CodeSemaCtorParams           = "TOL2005"
	CodeSemaBreakOutsideLoop     = "TOL2006"
	CodeSemaContinueOutsideLoop  = "TOL2007"
	CodeSemaInvalidSetTarget     = "TOL2008"
	CodeSemaMissingCondition     = "TOL2009"
	CodeSemaInvalidSelector      = "TOL2010"
	CodeSemaDuplicateSelector    = "TOL2011"
	CodeSemaInvalidSelectorExpr  = "TOL2012"
	CodeSemaSelectorTarget       = "TOL2013"
	CodeSemaInvalidFnModifier    = "TOL2014"
	CodeSemaConflictingModifier  = "TOL2015"
	CodeSemaDuplicateParam       = "TOL2016"
	CodeSemaInvalidReturn        = "TOL2017"
	CodeSemaStorageAccess        = "TOL2018"
	CodeSemaCallArity            = "TOL2019"
	CodeSemaInvalidAssignExpr    = "TOL2020"
	CodeSemaInvalidStmtShape     = "TOL2021"
	CodeSemaInvalidRevert        = "TOL2022"
	CodeSemaEmitArity            = "TOL2023"
	CodeSemaDuplicateEvent       = "TOL2024"
	CodeSemaUnknownEmitEvent     = "TOL2025"
	CodeSemaNameCollision        = "TOL2026"
	CodeSemaSelectorVisibility   = "TOL2027"
	CodeSemaDuplicateLocal       = "TOL2028"
	CodeSemaParamReturnCollision = "TOL2029"
	CodeSemaUnreachableStmt      = "TOL2030"
	CodeSemaUnknownCallTarget    = "TOL2031"
	CodeSemaCallVisibility       = "TOL2032"
	CodeSemaReservedName         = "TOL2033"
	CodeSemaInvalidTypeBoundsType   = "TOL2034"
	CodeSemaTestInNonTestFile       = "TOL2035"
	CodeSemaInvalidTestFnName       = "TOL2036"
	CodeSemaInspectInNonTestFile    = "TOL2037"
	CodeSemaUnknownModifier         = "TOL2038"
	CodeSemaDuplicateModifier       = "TOL2039"
	CodeSemaModifierPlaceholder     = "TOL2040"
	CodeSemaInheritCycle            = "TOL2041"
	CodeSemaInheritC3Conflict       = "TOL2042"
	CodeSemaUnknownBase             = "TOL2043"
	CodeSemaInterfaceNotImpl        = "TOL2044"
	CodeSemaOverrideSigMismatch     = "TOL2045"
	CodeSemaInvalidSuperCall        = "TOL2046"

	// Effect / mutability enforcement (P-new).
	CodeSemaPureFunctionStorageRead  = "TOL2050"
	CodeSemaPureFunctionStorageWrite = "TOL2051"
	CodeSemaPureFunctionEnvRead      = "TOL2052"
	CodeSemaPureFunctionEmit         = "TOL2053"
	CodeSemaPureFunctionExternalCall = "TOL2054"
	CodeSemaViewFunctionStorageWrite = "TOL2055"
	CodeSemaViewFunctionEmit         = "TOL2056"
	CodeSemaViewFunctionStateCall    = "TOL2057"
	CodeSemaNonPayableMsgValue       = "TOL2058"
	CodeSemaUninitializedRead        = "TOL2060"

	CodeSemaInvalidDeleteTarget     = "TOL2061"

	// Try/catch error handling.
	CodeSemaTryNonCall              = "TOL2062"
	CodeSemaDuplicateCatch          = "TOL2063"

	// Struct types.
	CodeSemaDuplicateStruct         = "TOL2064"
	CodeSemaUnknownStructType       = "TOL2065"
	CodeSemaStructLiteralArity      = "TOL2066"
	CodeSemaStructLiteralUnknown    = "TOL2067"
	CodeSemaStructFieldNotFound     = "TOL2068"

	// Abstract contracts.
	CodeSemaAbstractFunctionInConcreteContract  = "TOL2069"
	CodeSemaAbstractContractNotFullyImplemented = "TOL2070"

	// Constant declarations.
	CodeSemaDuplicateConstant       = "TOL2071"
	CodeSemaConstantInvalidType     = "TOL2072"
	CodeSemaConstantInvalidValue    = "TOL2073"
	CodeSemaConstantWriteProhibited = "TOL2074"

	// Immutable variable checks.
	CodeSemaImmutableBadType          = "TOL2080" // immutable declared with non-value type
	CodeSemaImmutableWriteOutsideCtor = "TOL2081" // immutable written outside constructor
	CodeSemaImmutableNotAssigned      = "TOL2082" // immutable never assigned in constructor

	// type(I).interfaceId and payable(...) checks.
	CodeSemaInterfaceIdUnknown      = "TOL2083" // type(I).interfaceId: I is not a known interface
	CodeSemaPayableArity            = "TOL2084" // payable(...) arity error
	CodeSemaTransferOnNonPayable    = "TOL2085" // .transfer()/.send() called on non-payable address
	CodeSemaBytesEqualityOperator   = "TOL2086" // == / != on bytes/string; use bytes_eq/string_eq

	// Import resolution errors.
	CodeSemaImportNoResolver   = "TOL2090" // import used but no resolver provided
	CodeSemaImportNotFound     = "TOL2092" // referenced file could not be read
	CodeSemaImportParseFailed  = "TOL2093" // referenced file failed to parse
	CodeSemaImportNameNotFound = "TOL2094" // named entity not found in referenced file
	CodeSemaImportCircular     = "TOL2095" // circular import detected

	// User-defined value types (UDVT).
	CodeSemaUDVTInvalidUnderlying = "TOL2096" // type X is Y — Y must be an elementary value type

	// Reserved platform namespace.
	CodeSemaTolLangReserved = "TOL2097" // package "tol.lang" is a reserved platform namespace; external packages may not declare it

	// Deprecation warnings (aligned with Solidity error codes where applicable).
	CodeWarnYearsUnit = "TOL4820" // 'years' unit denomination is deprecated (Solidity 4820)

	// Effect annotation diagnostics (TOL2200–TOL2205)
	CodeEffectUndeclared     = "TOL2200" // inferred effect not covered by declared effect set
	CodeEffectGasUnbounded   = "TOL2201" // @gas upper declared but function is UNBOUNDED
	CodeEffectGasTooLow      = "TOL2202" // declared @gas upper < inferred conservative upper bound
	CodeEffectEmptyCalls     = "TOL2204" // @effects calls:[] declared but external call found in IR
	CodeEffectSelectorNoSite = "TOL2205" // @effects calls: declared selector has no matching IR call site

	CodeLowerNotImplemented                     = "TOL3001"
	CodeLowerUnsupportedFeature  = "TOL3002"
	CodeCodegenNotImplemented    = "TOL4001"
)

// Position describes a line/column position in a source file.
type Position struct {
	Line   int
	Column int
}

// Span describes a source range.
type Span struct {
	File  string
	Start Position
	End   Position
}

// Diagnostic is a structured compile-time message (error or warning).
type Diagnostic struct {
	Code     string
	Message  string
	Span     Span
	Severity Severity // SeverityError (default) or SeverityWarning
}

func (d Diagnostic) Error() string {
	if d.Span.File == "" || d.Span.Start.Line <= 0 || d.Span.Start.Column <= 0 {
		return fmt.Sprintf("[%s] %s", d.Code, d.Message)
	}
	return fmt.Sprintf("%s:%d:%d: [%s] %s",
		d.Span.File,
		d.Span.Start.Line,
		d.Span.Start.Column,
		d.Code,
		d.Message,
	)
}

// Diagnostics is an ordered diagnostic list.
type Diagnostics []Diagnostic

func (ds Diagnostics) Error() string {
	if len(ds) == 0 {
		return ""
	}
	if len(ds) == 1 {
		return ds[0].Error()
	}
	return fmt.Sprintf("%s (and %d more error(s))", ds[0].Error(), len(ds)-1)
}

// HasErrors returns true if any diagnostic has SeverityError.
// Warning-only diagnostic lists do not constitute compilation failure.
func (ds Diagnostics) HasErrors() bool {
	for _, d := range ds {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// HasWarnings returns true if any diagnostic has SeverityWarning.
func (ds Diagnostics) HasWarnings() bool {
	for _, d := range ds {
		if d.Severity == SeverityWarning {
			return true
		}
	}
	return false
}
