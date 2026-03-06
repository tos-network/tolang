// ANTLR4 Parser Grammar for the TOL (TOS Object Language) language.
//
// TOL v0.3 / v0.4 (agent-native extension) — see docs/TOL_SPEC.md and
// docs/AGENT-NATIVE.md for the full language specification.
//
// This grammar is a specification document aligned with SolidityParser.g4
// (see docs/grammar/diff.md for the full diff analysis).
//
// The production parser is in tol/parser/parser.go (recursive-descent,
// Pratt precedence climbing for expressions).  Features marked
// "// TODO: not yet in production" are specified here but not yet
// fully implemented in parser.go.
//
// Key TOL design choices retained:
//   - 'set' makes all storage writes explicit (implicit = also accepted)
//   - 'let' declares local variables (type-first also accepted)
//   - 'pragma tolang X.Y.Z' version header is mandatory
//   - Test blocks ('test Name { … }') are first-class constructs
//   - '@effects' annotation system for formal verification
//   - '>>>' / '>>>=' logical right-shift distinct from '>>' arithmetic

parser grammar TolangParser;

options { tokenVocab = TolangLexer; }

// ============================================================
// Doc-comment annotation system  (@effects / @bounds / @gas / agent-native)
//
// TOL uses triple-slash (///) and block (/** */) doc comments to attach structured
// metadata to functions, constructors, fallback, and receive declarations.
// These are emitted as DocLineComment / DocBlockComment tokens by the lexer and
// parsed by parseDocMeta() in the production parser.
//
// Each doc-comment line that starts with '@' is a structured tag:
//
//   @notice    Free-text human description of the function.
//   @param     name description — documents one parameter.
//   @return    name description — documents one return value.
//
//   @effects   reads:  storage.x, storage.m[caller]
//   @effects   writes: storage.x
//   @effects   emits:  Transfer, Approval
//   @effects   calls:  Cap[IFace.method; max_gas=N; max_calls=M]
//              (TOL verification system; see docs/TOL_EFFECTS.md)
//
//   @bounds    param <= N              — upper bound on a parameter or loop var
//   @bounds    param == N             — exact bound
//
//   @gas       upper = N              — maximum gas cost assertion
//
//   @requires  (caller: CapabilityName)  — access control via capability bit
//                                          equivalent to: require(tos.hascap(msg.sender, CapabilityName))
//   @pay       (amount=expr)           — expected payment amount
//   @pay       (amount=expr, recipient=expr) — payment with explicit recipient
//   @pay       (expr)                  — bare form: expr is the amount
//
//   @verifiable    — marks function result as verifiable off-chain (ZK-ready)
//   @delegated     — marks function as delegation-capable (accepts delegated calls)
//
// Standalone '@' annotations (without '///') are also accepted by the parser
// immediately before a function declaration:
//   @requires(caller: Registrar)
//   @pay(1_000_000)
//   @verifiable
//   @delegated
//   @selector("0xAABBCCDD")
//
// These are parsed by parseFunctionAttributes() and populate DocMeta identically
// to the triple-slash form.  Both forms may be combined on the same declaration.
// ============================================================

// ============================================================
// Top-level source unit
// ============================================================

sourceUnit
    : pragmaDirective
      packageDeclaration?
      ( importDeclaration
      | topLevelDeclaration
      )*
      EOF
    ;

// ============================================================
// Version / pragma directive
//
// Production parser (parser.go) accepts both:
//   pragma tolang 0.3.0;               — canonical TOL form
//   pragma solidity ^0.8.0;            — Solidity-compatible
//   pragma solidity >=0.7.0 <0.9.0;   — range constraint
//
// 'pragma' is a reserved keyword (Pragma token).
// Everything between 'pragma' and ';' is consumed as pragmaToken+.
// The production lexer does NOT use a special PragmaMode; pragma content
// is tokenised in the normal mode.
// ============================================================

pragmaDirective
    : Pragma pragmaToken+ Semicolon
    ;

// pragmaToken: any token that can appear inside a pragma argument.
// Covers the language name (identifier), version numbers,
// and Solidity-style version range operators.
pragmaToken
    : Identifier
    | DecimalNumber
    | Lt | Gt | Le | Ge | Assign | BitXor | Minus | OrOr
    ;

// ============================================================
// Package declaration  (TOL-specific)
//
//   package tos.registry;
//   package com.example.myapp;
//
// Must appear at most once, immediately after the pragma directive.
// The dotted name becomes the module's canonical namespace identifier,
// stored in Module.Package and emitted as the "package" field in the
// .tor manifest.
// ============================================================

packageDeclaration
    : Package identifierPath Semicolon
    ;

// ============================================================
// Import declaration — all Solidity-compatible forms plus TOL package imports
//
//   import "path";                              — bare file import
//   import "path" as Alias;                     — file alias
//   import {A, B as C} from "path";             — named symbols
//   import * as Alias from "path";              — star alias
//   import Name from "path";                    — legacy TOL form
//   import tos.registry.AgentRegistry;          — package import  (TOL-specific)
//   import tos.registry.AgentRegistry as IFoo;  — package import with alias  (TOL-specific)
//
// Package imports use a dotted identifier path (no string literal).
// The last segment is the contract/interface name; the preceding segments
// form the package path (e.g. "tos.registry").
// The production resolver (tol_api.go OSFileResolver) maps the dotted path
// to a filesystem location: tos/registry/AgentRegistry.{tol,toi,toc,tor}.
// ============================================================

importDeclaration
    : Import (
        ( StringLiteral (As Identifier)? )                     // bare or file-alias
      | ( symbolAliases From StringLiteral )                   // {A, B as C} from "path"
      | ( Star As Identifier From StringLiteral )              // * as Alias from "path"
      | ( identifierPath As Identifier )                       // package import with alias
      | ( identifierPath )                                      // package import (bare)
      | ( Identifier From StringLiteral )                      // legacy: Name from "path"
    ) Semicolon
    ;
    // NOTE: The two package-import alternatives must appear BEFORE the legacy
    // (Identifier From StringLiteral) alternative so that ANTLR4 LL(*) prediction
    // resolves ambiguity correctly:
    //   import Foo;           -> identifierPath bare, no From  -> package import
    //   import Foo.Bar;       -> identifierPath with Dot       -> package import
    //   import Foo as X;      -> identifierPath As Identifier  -> package import with alias
    //   import Foo from "p";  -> Identifier From StringLiteral -> legacy import

symbolAliases
    : LBrace importAlias (Comma importAlias)* RBrace
    ;

importAlias
    : Identifier (As Identifier)?
    ;

// ============================================================
// Top-level declarations
// ============================================================

topLevelDeclaration
    : contractDeclaration
    | interfaceDeclaration
    | libraryDeclaration
    | structDeclaration
    | enumDeclaration
    | errorDeclaration
    | eventDeclaration
    | userDefinedValueTypeDefinition
    | constantDeclaration
    | capabilityDeclaration           // TOL agent-native: top-level shared capability set
    | functionDeclaration             // free function at file level // TODO: not yet in production
    | usingDeclaration                // top-level using              // TODO: not yet in production
    | testDeclaration
    ;
    // Note: purposeDeclaration is only valid inside a contract body (not top-level).

// ============================================================
// Contract declaration
//
//   contract Token is IERC20, Ownable { … }
//   abstract contract Base { … }
// ============================================================

contractDeclaration
    : Abstract? Contract Identifier inheritanceClause? LBrace contractMember* RBrace
    ;

inheritanceClause
    : Is inheritanceSpecifier (Comma inheritanceSpecifier)*
    ;

inheritanceSpecifier
    : identifierPath (LParen expressionList? RParen)?
    ;
    // Base constructor arguments allowed: is Base(arg1, arg2)
    // Aligned with Solidity inheritanceSpecifier.

contractMember
    : storageVariable
    | eventDeclaration
    | functionDeclaration
    | constructorDeclaration
    | fallbackDeclaration
    | receiveDeclaration
    | errorDeclaration
    | enumDeclaration
    | userDefinedValueTypeDefinition
    | modifierDeclaration
    | immutableDeclaration
    | constantDeclaration
    | usingDeclaration
    | structDeclaration
    | capabilityDeclaration           // TOL agent-native: capability Foo;
    | purposeDeclaration              // TOL agent-native: purpose WorkEscrow;
    | agentNativeStorageDeclaration   // TOL agent-native: oracle<T>, vote<T> storage slots
    | manifestDeclaration             // TOL agent-native: manifest {} metadata block
    ;

// ============================================================
// Interface declaration
//
//   interface IERC20 {
//     function transfer(agent to, uint256 amount) external returns (bool ok);
//   }
// ============================================================

interfaceDeclaration
    : Interface Identifier inheritanceClause? LBrace interfaceMember* RBrace
    ;

interfaceMember
    : functionDeclaration
    | eventDeclaration
    | errorDeclaration
    | enumDeclaration
    | userDefinedValueTypeDefinition
    | structDeclaration
    | usingDeclaration
    ;
    // Interfaces may not contain state variables or constructors.
    // Aligned with Solidity: interfaces share contractBodyElement (with restrictions enforced by sema).

// ============================================================
// Library declaration
//
//   library Math {
//     function max(uint256 a, uint256 b) internal pure returns (uint256 c) { … }
//   }
// ============================================================

libraryDeclaration
    : Library Identifier LBrace contractMember* RBrace
    ;
    // Libraries use the full contractMember set (restrictions enforced by sema:
    // no state variables, no fallback/receive, no non-constant state vars).

// ============================================================
// Struct declaration  (top-level or inside a contract)
//
//   struct Point { uint256 x; uint256 y; }
// ============================================================

structDeclaration
    : Struct Identifier LBrace structField* RBrace
    ;

structField
    : typeName Identifier Semicolon?
    ;

// ============================================================
// User-defined value type  (Solidity-aligned)
//
//   type Price is uint256;
// ============================================================

userDefinedValueTypeDefinition
    : Type Identifier Is elementaryTypeName Semicolon
    ;

// ============================================================
// Storage variable  (directly in contract body, Solidity-style)
//
//   uint256 totalSupply;
//   mapping(agent => u256) balances;
//   uint256 public maxSupply = 1000000;
//   transient uint256 lockStatus;
// ============================================================

storageVariable
    : Transient? typeName
      stateVariableModifier*
      Identifier
      (Assign expression)?
      Semicolon
    ;

stateVariableModifier
    : Public
    | Private
    | Internal
    | Constant
    | Immutable
    | overrideSpecifier
    ;
    // Aligned with Solidity stateVariableDeclaration modifiers.
    // Note: Constant and Immutable here express the Solidity-style form
    // (e.g. uint256 public constant MAX = 100;).
    // The separate constantDeclaration / immutableDeclaration productions
    // express the legacy TOL-native form (constant MAX: uint256 = 100;).

// ============================================================
// Immutable and constant declarations  (both TOL-native and Solidity-style)
//
//   TOL-native:   immutable owner: agent;
//   Solidity:     address immutable owner;  — normalised to agent (backward compat)
//
//   TOL-native:   constant MAX: uint256 = 1000000;
//   Solidity:     uint256 constant MAX = 1000000;  (via storageVariable above)
// ============================================================

immutableDeclaration
    : Immutable Identifier Colon typeName Semicolon
    ;

constantDeclaration
    : Constant Identifier Colon typeName Assign expression Semicolon
    ;

// ============================================================
// Override specifier
//
//   override
//   override(Base1, Base2)
// ============================================================

overrideSpecifier
    : Override (LParen identifierPath (Comma identifierPath)* RParen)?
    ;

// ============================================================
// Event declaration
//
//   event Transfer(agent indexed from, agent indexed to, uint256 value);
//   event Approval(agent owner, agent spender, uint256 value) anonymous;
// ============================================================

eventDeclaration
    : Event Identifier LParen eventParameterList? RParen Anonymous? Semicolon?
    ;

eventParameterList
    : eventParameter (Comma eventParameter)*
    ;

eventParameter
    : typeName Indexed? Identifier?
    ;
    // Indexed is now a reserved keyword token (Solidity-aligned).
    // Optional name after optional indexed.

// ============================================================
// Error declaration
//
//   error InsufficientBalance(agent account, uint256 needed);
// ============================================================

errorDeclaration
    : Error Identifier LParen errorParameterList? RParen Semicolon
    ;

errorParameterList
    : errorParameter (Comma errorParameter)*
    ;

errorParameter
    : typeName Identifier?
    ;

// ============================================================
// Enum declaration
//
//   enum Status { Pending, Active, Closed }
// ============================================================

enumDeclaration
    : Enum Identifier LBrace enumValueList? RBrace
    ;

enumValueList
    : Identifier (Comma Identifier)* Comma?
    ;
    // Trailing comma accepted (TOL extension; Solidity does not allow it).

// ============================================================
// Using declaration  (Solidity-aligned, extended)
//
//   using Math for uint256;
//   using SafeMath for *;
//   using { add, sub } for uint256;
//   using { addImpl as + } for uint256 global;
// ============================================================

usingDeclaration
    : Using (
        identifierPath
      | (LBrace usingAlias (Comma usingAlias)* RBrace)
    ) For (Star | typeName) Global? Semicolon
    ;

usingAlias
    : identifierPath (As userDefinableOperator)?
    ;

userDefinableOperator
    : BitAnd | BitNot | BitOr | BitXor
    | Plus | Slash | Percent | Star | Minus
    | Eq | Gt | Ge | Lt | Le | Ne
    ;

// ============================================================
// Capability declaration  (TOL agent-native)
//
//   capability Resolver;
//   capability Poster;
//
// ONE capability name per declaration (multiple capabilities require separate lines).
// May appear at top-level (shared across contracts) or inside a contract body.
// 'capability' is a contextual keyword (Identifier with literal "capability").
// Production: parseCapabilityDecl() in parser.go.
// ============================================================

capabilityDeclaration
    : Identifier Identifier Semicolon
    ;
    // First Identifier = "capability"; second Identifier = capability name.
    // Example: capability Registrar;

// ============================================================
// Purpose declaration  (TOL agent-native)
//
//   purpose WorkEscrow;
//   purpose RewardPool;
//
// Declares a named escrow purpose bucket.  The compiler assigns ordinals 0–255
// in declaration order.  References as the third argument of escrow()/release()/slash().
// 'purpose' is a contextual keyword (Identifier with literal "purpose").
// Production: parsePurposeDecl() in parser.go.
// ============================================================

purposeDeclaration
    : Identifier Identifier Semicolon
    ;
    // First Identifier = "purpose"; second Identifier = purpose name.

// ============================================================
// Agent-native storage slot declarations  (TOL agent-native)
//
//   oracle<uint256> price;
//   oracle<bytes32> jobHash;
//   vote<uint8>     proposal;
//   agent           admin;          — agent handle (no type parameter)
//
// 'oracle', 'vote', 'task', 'agent' are contextual keywords (Identifier tokens).
// Optional visibility modifier (public/private/internal) and override may follow
// the type before the slot name.
// Storage slots of type mapping(K => task<T>) use the regular storageVariable rule.
// Production: parseAgentTypeSlot() in parser.go.
// ============================================================

agentNativeStorageDeclaration
    : Identifier (Lt typeName Gt)? stateVariableModifier* Identifier Semicolon
    ;
    // Leading Identifier: 'oracle' | 'vote' | 'task' | 'agent'
    // Lt typeName Gt is present for oracle<T>/vote<T>/task<T>; absent for bare 'agent'.
    // stateVariableModifier allows public/private/internal/override (same as storageVariable).

// ============================================================
// Manifest declaration  (TOL agent-native)
//
//   manifest {
//     name:         "AgentProtocol";
//     version:      "1.0.0";
//     capabilities: [Resolver, Poster, Worker];
//     min_stake:    1000000000000000000;
//   }
//
// Key–value separator: COLON (not equals).
// Field terminator: either ';' or ',' (both optional before '}'.
// Values: string literal, decimal number, or array of identifiers/strings.
// Production: parseManifestDecl() in parser.go.
// ============================================================

manifestDeclaration
    : Identifier LBrace manifestField* RBrace
    ;
    // Leading Identifier must equal "manifest" (production: contextual).

manifestField
    : Identifier Colon manifestFieldValue (Semicolon | Comma)?
    ;
    // Key: Colon separator (NOT Assign '=').

manifestFieldValue
    : StringLiteral
    | DecimalNumber
    | HexNumber
    | LBracket (Identifier | StringLiteral) (Comma (Identifier | StringLiteral))* RBracket
    ;
    // Array form: [A, B, C] or ["a", "b", "c"]

// ============================================================
// Modifier declaration
//
//   modifier onlyOwner() { require(msg.sender == owner, "NOT_OWNER"); _; }
//   modifier onlyOwner  { require(msg.sender == owner, "NOT_OWNER"); _; }
//   modifier abstract virtual;
// ============================================================

modifierDeclaration
    : Modifier Identifier (LParen parameterList? RParen)?
      (Virtual | overrideSpecifier)*
      (block | Semicolon)
    ;
    // Parentheses are optional when the modifier has no parameters (Solidity-aligned).
    // Semicolon body = abstract modifier (no implementation).

modifierBody
    : modifierBodyElement*
    ;

modifierBodyElement
    : statement
    | placeholderStatement
    ;

placeholderStatement
    : Identifier Semicolon
    ;
    // The Identifier must be '_'.

// ============================================================
// Function declaration
//
//   function transfer(agent to, u256 amount) external returns (bool ok) { … }
//   function name() virtual;   — abstract
// ============================================================

functionDeclaration
    : functionAttribute*
      Function Identifier LParen parameterList? RParen
      functionModifier*
      (Returns LParen namedReturnList RParen)?
      (block | Semicolon)
    ;

functionAttribute
    : At Identifier (LParen attributeArgumentList? RParen)?
    ;
    // TOL-specific attributes (all forms accepted):
    //   Test / fuzz:   @skip, @tag("name"), @fuzz, @fuzz(count=N), @timeout(ms), @cases
    //   ABI:           @selector("0xAABBCCDD")
    //   Agent-native:  @requires(caller: CapabilityName)
    //                  @pay(amount=expr, recipient=expr) or @pay(expr)
    //                  @verifiable    — marks function as verifiably deterministic
    //                  @delegated     — marks function as delegation-capable
    //   Effects:       @effects(reads: [...], writes: [...], emits: [...])
    //                  @gas(upper=N)
    //                  @bounds(param: min..max)
    //
    // attributeArgument handles both positional (expr) and named (key=expr) forms.
    // Agent-native annotations may appear as standalone attributes BEFORE the
    // function keyword (no triple-slash doc comment required).

attributeArgumentList
    : attributeArgument (Comma attributeArgument)*
    ;

attributeArgument
    : expression
    | Identifier Assign expression
    ;

// ============================================================
// Constructor, fallback, and receive
// ============================================================

constructorDeclaration
    : Constructor LParen parameterList? RParen functionModifier* block
    ;

fallbackDeclaration
    : Fallback LParen parameterList? RParen
      (External | functionModifier)*
      (Returns LParen namedReturnList RParen)?
      (block | Semicolon)
    ;
    // Aligned with Solidity: fallback can have parameters and returns clause,
    // and may be abstract (Semicolon).

receiveDeclaration
    : Receive LParen RParen
      (External | functionModifier)*
      (block | Semicolon)
    ;
    // Receive is now a reserved keyword token (Solidity-aligned).

// ============================================================
// Function modifiers (visibility, mutability, virtual, override, invocations)
// ============================================================

functionModifier
    : visibilityModifier
    | stateMutabilityModifier
    | Virtual
    | overrideSpecifier
    | modifierInvocation
    ;

visibilityModifier
    : Public | External | Internal | Private
    ;

stateMutabilityModifier
    : Pure | View | Payable
    ;

modifierInvocation
    : identifierPath (LParen expressionList? RParen)?
    ;

// ============================================================
// Parameter lists
// ============================================================

parameterList
    : parameter (Comma parameter)*
    ;

parameter
    : typeName dataLocation? Identifier?
    ;

dataLocation
    : Memory | Storage | Calldata
    ;

namedReturnList
    : namedReturn (Comma namedReturn)*
    ;

namedReturn
    : typeName Identifier?
    ;

// ============================================================
// Qualified identifier path  (Solidity-aligned)
//
//   A           — simple identifier
//   A.B         — member access path
//   A.B.C       — nested path
// ============================================================

identifierPath
    : Identifier (Dot Identifier)*
    ;

// ============================================================
// Types
// ============================================================

typeName
    : elementaryTypeName
    | functionTypeName
    | mappingType
    | genericTypeName                            // oracle<T>, task<T>, vote<T>
    | userDefinedTypeName
    | typeName LBracket expression? RBracket    // T[] or T[N]
    ;

// Generic agent-native type names  (TOL agent-native)
//
//   oracle<uint256>       — oracle storage slot type
//   task<bytes32>         — task slot type (used inside mapping value position)
//   vote<uint8>           — vote slot type
//   agent                 — agent handle type (no type parameter)
//
// 'oracle', 'task', 'vote', 'agent' are contextual keywords (Identifier tokens).
// Production: parseField() and parseStatement() detect these by literal value.
//
// NOTE: In expressions, task<T>.new(...) is parsed as Identifier Lt typeName Gt
//       Dot Identifier callArgumentList (handled specially in parsePrefixExpr).

genericTypeName
    : Identifier Lt typeName Gt     // oracle<T>, task<T>, vote<T>
    | Identifier                    // agent (bare, no type param)
    ;
    // Leading Identifier: 'oracle' | 'task' | 'vote' | 'agent'

elementaryTypeName
    : UnsignedIntegerType
    | SignedIntegerType
    | FixedBytesType
    | Bool
    | Bytes
    | String
    | Fixed
    | Ufixed
    // NOTE: 'agent' is NOT listed here — it is an Identifier token (contextual
    // keyword) parsed as a genericTypeName or userDefinedTypeName.
    // 'address' and 'address payable' are accepted by the production lexer for
    // backward compatibility and silently normalised to 'agent'.
    ;

userDefinedTypeName
    : identifierPath
    ;
    // Covers contracts, interfaces, structs, enums, and UDVTs.
    // Qualified paths (A.B.C) allow cross-contract / cross-library references.

mappingType
    : Mapping LParen mappingKeyType Identifier? FatArrow typeName Identifier? RParen
    ;
    // Named key and named value are optional (Solidity 0.8.18+, EIP-4200):
    //   mapping(agent from => u256 balance)
    // The second Identifier (after key type) is the optional key name;
    // the Identifier after typeName is the optional value name.

mappingKeyType
    : UnsignedIntegerType
    | SignedIntegerType
    | FixedBytesType
    | Bool
    | String
    | Bytes
    | identifierPath              // user-defined types, enums, and 'agent' as mapping keys
    ;
    // 'agent' is recognised via identifierPath (it is a contextual Identifier token).
    // 'address' is still accepted by the production parser and normalised to 'agent'.

// Function type
//
//   function(agent, u256) external returns (bool)
//   function(bytes memory) internal pure
functionTypeName
    : Function LParen parameterList? RParen
      (visibilityModifier | stateMutabilityModifier)*
      (Returns LParen parameterList RParen)?
    ;

// ============================================================
// Statements
// ============================================================

block
    : LBrace statement* RBrace
    ;

statement
    : block
    | letStatement
    | letTupleStatement
    | variableDeclarationStatement    // type-first local var (Solidity-compatible)
    | setStatement
    | prefixIncDecStatement           // ++x; / --x; (prefix-only, not an expression)
    | ifStatement
    | whileStatement
    | forStatement
    | doWhileStatement
    | breakStatement
    | continueStatement
    | returnStatement
    | revertStatement
    | requireStatement
    | assertStatement
    | emitStatement
    | tryCatchStatement
    | uncheckedStatement
    | deleteStatement
    | expressionStatement
    ;
    // Note: assemblyStatement is not yet supported in TOL (no Yul backend).

// ============================================================
// let — TOL-native local variable declaration
//
//   let x: uint256 = 1;
//   let x: uint256;           — zero-initialised
//   let (a, b): (T1, T2) = abi.decode(data);
// ============================================================

letStatement
    : Let Identifier (Colon typeName)? (Assign expression)? Semicolon
    ;
    // Type annotation is OPTIONAL — production accepts both:
    //   let x: uint256 = 1;   — type-annotated
    //   let x = 1;            — type inferred (no colon+typeName)
    //   let x: uint256;       — zero-initialised, explicit type

letTupleStatement
    : Let LParen Identifier (Comma Identifier)+ RParen
      (Colon LParen typeName (Comma typeName)+ RParen)?
      Assign expression
      Semicolon
    ;

// ============================================================
// Type-first local variable declaration  (Solidity-compatible)
//
//   uint256 x = 1;
//   agent owner;
//   (uint256 a, bool b) = abi.decode(data, (uint256, bool));
// ============================================================

variableDeclarationStatement
    : (typeName dataLocation? Identifier (Assign expression)?)
      Semicolon
    | (LParen variableDeclarationTupleElem (Comma variableDeclarationTupleElem)+ RParen
       Assign expression)
      Semicolon
    ;

variableDeclarationTupleElem
    : typeName Identifier
    | /* empty slot */
    ;

// ============================================================
// set — explicit storage / local write  (TOL-specific)
//
//   set x = expr;
//   set mapping[key] = expr;
//   set x += 1;
//   set i++;
// ============================================================

setStatement
    : Set setTarget (assignmentOperator expression | PlusPlus | MinusMinus) Semicolon
    ;

setTarget
    : Identifier setAccessor*
    ;

setAccessor
    : Dot Identifier
    | LBracket expression RBracket
    ;

assignmentOperator
    : Assign
    | PlusAssign | MinusAssign | MulAssign | DivAssign | ModAssign
    | AndAssign  | OrAssign    | XorAssign
    | ShlAssign  | SarAssign   | ShrAssign
    ;

// ============================================================
// Control flow
// ============================================================

ifStatement
    : If LParen expression RParen statement (Else statement)?
    ;
    // Non-block bodies allowed (Solidity-aligned):
    //   if (cond) doSomething();   — single statement, no braces
    // The production parser currently still enforces blocks; this production
    // is the target behaviour.

whileStatement
    : While LParen expression RParen statement
    ;

forStatement
    : For LParen forInitializer? Semicolon expression? Semicolon forPost? RParen statement
    ;

forInitializer
    : letStatement
    | variableDeclarationStatement
    | setStatement
    | expressionStatement
    ;

forPost
    : expression
    ;

doWhileStatement
    : Do block While LParen expression RParen Semicolon
    ;

breakStatement
    : Break Semicolon
    ;

continueStatement
    : Continue Semicolon
    ;

returnStatement
    : Return expression? Semicolon
    ;

// ============================================================
// revert
//
//   revert;                              — bare revert
//   revert InsufficientBalance(a, n);    — named custom error
//   revert("message");                   — string reason (legacy)
// ============================================================

revertStatement
    : Revert (expression callArgumentList?)? Semicolon
    ;

// ============================================================
// require / assert  (TOL-specific statement forms)
//
//   require(cond);                  — no message; emits Panic(0x01)
//   require(cond, "message");       — with message; emits Error(string)
//   assert(cond, "message");        — always-on check
// ============================================================

requireStatement
    : Require LParen expression (Comma StringLiteral)? RParen Semicolon
    ;
    // Message is OPTIONAL (Solidity-aligned: require(cond) valid).

assertStatement
    : Assert LParen expression (Comma StringLiteral)? RParen Semicolon
    ;

// ============================================================
// emit
//
//   emit Transfer(from, to, amount);
// ============================================================

emitStatement
    : Emit expression Semicolon
    ;
    // 'expression' includes the event call and its argument list:
    //   emit Transfer(from, to, amount);  — expression = Transfer(from, to, amount)
    // Production: parseUnaryCallLikeStatement("emit", ...) consumes Emit + expression.

// ============================================================
// delete  (Solidity-aligned statement form)
//
//   delete arr[i];
//   delete mapping[key];
// ============================================================

deleteStatement
    : Delete expression Semicolon
    ;
    // 'delete' is a reserved keyword.
    // NOTE: 'delete' is a STATEMENT-ONLY construct in TOL — it cannot appear
    // as a sub-expression (no DeleteExpr in expression grammar).

// ============================================================
// Prefix increment / decrement statement  (TOL-specific form)
//
//   ++x;    — equivalent to x = x + 1
//   --y;    — equivalent to y = y - 1
//
// Production: parsePrefixIncDecStatement() in parser.go.
// NOTE: prefix '++' / '--' are STATEMENT-ONLY — they are not valid
// as sub-expressions in TOL (unlike Solidity / C).
// Postfix form (x++, x--) IS valid as an expression (PostfixOp).
// ============================================================

prefixIncDecStatement
    : (PlusPlus | MinusMinus) expression Semicolon
    ;

uncheckedStatement
    : Unchecked block
    ;
    // 'unchecked' is now a reserved keyword (Solidity-aligned).
    // In TOL v0.3 unchecked has no runtime effect; provided for source compatibility.

expressionStatement
    : expression Semicolon
    ;

// ============================================================
// Call argument list  (Solidity-aligned)
//
//   (a, b, c)                — positional
//   ({to: alice, amount: 100}) — named (Solidity-style)
// ============================================================

callArgumentList
    : LParen (
        (expression (Comma expression)*)?
      | (LBrace namedArgument (Comma namedArgument)* RBrace)?
    ) RParen
    ;

namedArgument
    : Identifier Colon expression
    ;

// ============================================================
// try / catch statement  (Solidity-aligned)
//
//   try token.transfer(to, amount) returns (bool ok) {
//     …
//   } catch Error(string memory reason) {
//     …
//   } catch Panic(uint256 code) {
//     …
//   } catch (bytes memory err) {
//     …
//   } catch {
//     …
//   }
// ============================================================

tryCatchStatement
    : Try expression (Returns LParen parameterList RParen)? block catchClause+
    ;
    // At least ONE catch clause required (Solidity-aligned; was 0+ in v0.2).
    // Returns clause binds the external call's return values.

catchClause
    : Catch (Identifier? LParen parameterList RParen)? block
    ;
    // Forms:
    //   catch Error(string memory reason) { }   — Identifier = 'Error'
    //   catch Panic(uint256 code)         { }   — Identifier = 'Panic'
    //   catch (bytes memory err)          { }   — no Identifier
    //   catch                             { }   — bare

// ============================================================
// Agent-native builtin statements  (TOL agent-native)
//
// The following builtins are expressed as expression statements (expressionStatement).
// They look like function calls but have special semantic rules enforced by sema.
//
//   escrow(agent, amount)                  — 2-arg form; purpose defaults to 0
//   escrow(agent, amount, purpose)         — 3-arg form; explicit purpose literal
//   release(agent, amount)                 — 2-arg form; purpose defaults to 0
//   release(agent, amount, purpose)        — 3-arg form
//   slash(agent, amount)                   — 2-arg form; recipient & purpose default to 0
//   slash(agent, amount, recipient)        — 3-arg form; purpose defaults to 0
//   slash(agent, amount, recipient, purpose) — 4-arg form
//
//   oracle_fulfill(oracleSlot, value)      — fulfill an oracle slot (via slot.fulfill(v))
//   vote_cast(voteSlot, value)             — cast a vote (via slot.cast(v))
//   task_transition(taskBase, tid, from, to, extra) — task state machine transition
//
// Agent property access:
//   agent(addr).stake                      → __tol_agent_prop(addr, "stake")
//   agent(addr).is_active                  → composite active check
//   agent(addr).reputation                 → __tol_agent_prop(addr, "reputation")
//   agent(addr).rating_count               → __tol_agent_prop(addr, "rating_count")
//   agent(addr).suspended                  → __tol_agent_prop(addr, "suspended") ~= 0
//
// Oracle OOP member interface (on oracle<T> storage slots):
//   price.fulfill(v)                       → oracle_fulfill(price_val_slot, price_set_slot, v)
//   price.is_set                           → __tol_oracle_is_set(price_set_slot)
//   price.value                            → __tol_oracle_value(price_val_slot)
//
// Task OOP member interface (on mapping(K => task<T>) storage slots):
//   tasks[tid] = task<T>.new(poster, reward, deadline)  — create new task
//   tasks[tid].accept(worker)              — transition Open → Accepted
//   tasks[tid].submit(data)                — transition Accepted → Submitted
//   tasks[tid].approve()                   — transition Submitted → Approved
//   tasks[tid].reject()                    — transition Submitted → Rejected
//   tasks[tid].dispute()                   — transition → Disputed
//   tasks[tid].cancel()                    — transition → Cancelled
//   tasks[tid].worker                      — read worker agent
//   tasks[tid].poster                      — read poster agent
//   tasks[tid].reward                      — read reward amount
//   tasks[tid].is_expired                  — deadline < block.timestamp
//
// Task local handle:
//   task<bytes32> t = tasks[tid];          — bind task handle to local variable
//   t.approve();                           — call method on local handle
// ============================================================

// ============================================================
// Test block  (TOL-specific)
//
//   test MyTests {
//     setup { … }
//     teardown { … }
//     mock Counter : ICounter { function increment() { … } }
//
//     @tag("erc20")
//     function test_transfer() {
//       deploy Token(1000000) -> tok;
//       with msg.sender = alice { … }
//     }
//
//     @fuzz(count=500)
//     function fuzz_deposit(uint256 amount) { … }
//   }
// ============================================================

testDeclaration
    : Test Identifier LBrace testMember* RBrace
    ;

testMember
    : testLifecycleFunction
    | mockDeclaration
    | testFunction
    | letStatement
    ;

testLifecycleFunction
    : Identifier (Returns LParen namedReturnList RParen)? testBlock   // setup / setup_suite
    | Identifier (LParen parameterList? RParen)? testBlock            // teardown / teardown_suite
    ;

mockDeclaration
    : Identifier Identifier (Colon Identifier)? LBrace mockMethod* RBrace
    ;

mockMethod
    : Function Identifier LParen parameterList? RParen
      functionModifier*
      (Returns LParen namedReturnList RParen)?
      block
    ;

testFunction
    : functionAttribute* Function Identifier LParen parameterList? RParen
      testBlock casesTable?
    ;

casesTable
    : Identifier LBrace casesHeaderRow casesDataRow* RBrace
    ;
    // Identifier = "cases" (contextual).
    // Full syntax:
    //   cases {
    //     | col1   | col2   |          ← header: identifiers only
    //     | expr1  | expr2  |          ← data rows: expressions
    //     | expr3  | expr4  |
    //   }

casesHeaderRow
    : BitOr (Identifier BitOr)+
    ;
    // Column names are identifiers (not expressions).

casesDataRow
    : BitOr (expression BitOr)+
    ;
    // One expression per column, leading and trailing | required.

testBlock
    : LBrace testStatement* RBrace
    ;

testStatement
    : deployStatement
    | withStatement
    | assertRevertStatement
    | assertAllStatement
    | assertInstructionsLeStatement
    | statement
    ;

deployStatement
    : (Identifier | Deploy) Identifier (LParen expressionList? RParen)? (Arrow Identifier)? Semicolon
    ;
    // Leading token: Identifier with literal "deploy" OR the Deploy keyword token.
    // Both forms are equivalent; the production parser accepts either.
    //   deploy Counter(0) -> c;
    //   deploy Token(1_000_000) -> tok;

withStatement
    : Identifier expression testBlock
    ;
    // Identifier = "with" (contextual).
    // expression is an assignment expression overriding a context variable:
    //   with msg.sender = alice { ... }
    //   with msg.value = 1 ether { ... }
    // Production: parseWithStatement() in parser.go.

assertRevertStatement
    : Identifier (LParen expression? RParen)? testBlock
    ;
    // Identifier = "assert_revert" (contextual).
    // Forms:
    //   assert_revert { ... }            — any revert
    //   assert_revert() { ... }          — any revert (empty parens)
    //   assert_revert("msg") { ... }     — revert message must contain "msg"
    //   assert_revert(ErrorName(...)) { ... } — revert must match custom error

assertAllStatement
    : Identifier testBlock
    ;

assertInstructionsLeStatement
    : Identifier LParen expression RParen testBlock
    ;

// ============================================================
// Expressions
//
// Precedence (highest → lowest):
//   postfix:   . [] () {}  ++ --               (level 14)
//   prefix:    ! ~ + - delete                  (level 13)
//   pow:       **              (right-assoc)   (level 12)
//   mul:       * / %                           (level 11)
//   add:       + -                             (level 10)
//   shift:     << >> >>>                       (level 9)
//   cmp:       < <= > >=                       (level 8)
//   eq:        == !=                           (level 7)
//   bitand:    &                               (level 6)
//   bitxor:    ^                               (level 5)
//   bitor:     |                               (level 4)
//   and:       &&                              (level 3)
//   or:        ||                              (level 2)
//   ternary:   ? :            (right-assoc)   (level 1)
//   assign:    = += -= etc.   (right-assoc)   (level 0)
// ============================================================

expression
    // Postfix / primary-with-suffix (highest precedence)
    : expression LBracket expression? RBracket                         # IndexAccess
    | expression LBracket expression? Colon expression? RBracket       # SliceAccess
    | expression Dot Identifier                                        # MemberAccess
    | expression LBrace namedArgument (Comma namedArgument)* RBrace callArgumentList # FunctionCallOptions
    // Call options followed by argument list: transfer{value: 1 ether}(recipient, amount)
    // The {key: value} block MUST be immediately followed by callArgumentList.
    // Valid keys: 'gas' and 'value' (checked by sema; not enforced by the grammar).
    | expression callArgumentList                                       # FunctionCall
    | expression (PlusPlus | MinusMinus)                               # PostfixOp

    // Prefix / unary
    | (Bang | BitNot | Plus | Minus) expression                        # PrefixOp
    // NOTE: '++' / '--' prefix form is STATEMENT-ONLY in TOL (not a sub-expression).
    // Standalone ++x; / --x; is handled by prefixIncDecStatement in statement context.

    // Object construction / deployment
    | New typeName callArgumentList                                     # NewExpr
    | New Identifier LBracket RBracket callArgumentList                # NewArrayExpr
    // new T[](size) — dynamic memory array allocation: new uint256[](100)
    | Deploy typeName callArgumentList                                  # DeployExpr
    // 'deploy' is a TOL-specific alias for 'new'; both compile identically.

    // payable conversion: payable(addr)
    | Payable callArgumentList                                          # PayableConversion

    // type() meta: type(T).min / type(T).max / type(I).interfaceId
    | Type LParen typeName RParen                                       # MetaType

    // Arithmetic
    | <assoc=right> expression Pow expression                          # PowExpr
    | expression (Star | Slash | Percent) expression                   # MulDivMod
    | expression (Plus | Minus) expression                             # AddSub

    // Bitwise shifts
    | expression (Shl | Sar | Shr) expression                         # Shift

    // Comparison
    | expression (Lt | Le | Gt | Ge) expression                       # ComparisonExpr
    | expression (Eq | Ne) expression                                   # EqualityExpr

    // Bitwise binary
    | expression BitAnd expression                                      # BitAndExpr
    | expression BitXor expression                                      # BitXorExpr
    | expression BitOr expression                                       # BitOrExpr

    // Logical
    | expression AndAnd expression                                      # LogicalAnd
    | expression OrOr expression                                        # LogicalOr

    // Ternary (right-associative)
    | <assoc=right> expression Question expression Colon expression    # TernaryExpr

    // Assignment (right-associative)
    | <assoc=right> expression assignmentOperator expression           # AssignExpr

    // Primary
    | primary                                                           # PrimaryExpr
    ;

expressionList
    : expression (Comma expression)*
    ;

// ============================================================
// Primary expressions
// ============================================================

primary
    : inspectExpression       // MUST come before Identifier: inspect is contextual keyword
    | structLiteralExpression // MUST come before Identifier: StructName { field: expr }
    | Identifier
    | DecimalNumber (SubDenomination)?
    | HexNumber
    | StringLiteral+
    | UnicodeStringLiteral+
    | HexString+
    | BooleanLiteral
    | tupleExpression
    | inlineArrayExpression
    | typeExpression          // type(I).interfaceId — also type(T) for MetaType
    | elementaryTypeName      // type name used as cast: uint256(x), agent(y)
    ;
    // IMPORTANT ordering note:
    //   inspectExpression must precede Identifier because 'inspect' is a contextual
    //   keyword — the production parser checks (literal == "inspect") before treating
    //   the token as a plain identifier.
    //   structLiteralExpression must precede Identifier because disambiguation
    //   requires lookahead: StructName '{' only matches if StructName is a known
    //   struct type (tracked in parser.structNames).

BooleanLiteral
    : True | False
    ;

// Tuple expression: (a, b) / (a,) / (, b)
// Used for multi-assignment and multi-return binding.
tupleExpression
    : LParen (expression? (Comma expression?)*) RParen
    ;

// Inline array: [1, 2, 3]
inlineArrayExpression
    : LBracket (expression (Comma expression)*) RBracket
    ;

// Struct literal: StructName { field: expr, field2: expr }
//
// Only parsed when StructName is a declared struct type — the production parser
// tracks known struct names in parser.structNames to disambiguate from blocks.
// Field separator is Colon (:), same as named arguments.
//
// Examples:
//   Point { x: 1, y: 2 }
//   Order { buyer: msg.sender, amount: 100 ether, deadline: block.timestamp + 1 days }

structLiteralExpression
    : Identifier LBrace (structFieldInit (Comma structFieldInit)* Comma?)? RBrace
    ;
    // Identifier = struct type name (must be a declared struct in scope).

// type(I).interfaceId  or  type(T)  (MetaType primary form)
typeExpression
    : Identifier LParen typeName RParen (Dot Identifier)?
    ;
    // When followed by .Identifier: type(I).interfaceId, type(T).min, type(T).max.
    // When standalone: type(T) used as metatype (in MetaType expression production).

// inspect binding.slotName  — white-box storage read (test blocks only)
inspectExpression
    : Identifier Identifier Dot Identifier
    ;

// ============================================================
// structFieldInit is the shared field initializer rule used by structLiteralExpression.
structFieldInit
    : Identifier Colon expression
    ;
