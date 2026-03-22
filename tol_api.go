package lua

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/lower"
	"github.com/tos-network/tolang/tol/parser"
	"github.com/tos-network/tolang/tol/sema"
)

// OSFileResolver is a sema.FileResolver that reads files from the OS filesystem.
// Relative import paths are resolved against BaseDir.
//
// PackageSearchPaths provides explicit directories where package-style imports
// are resolved before falling back to the walk-up heuristic.  Each entry is a
// root directory that may contain package family subdirectories (e.g., if
// PackageSearchPaths includes "/repo/openlib" then "tolang.openlib.account" will
// check "/repo/openlib/account/" first).
type OSFileResolver struct {
	BaseDir            string
	PackageSearchPaths []string
}

const maxGitHubImportBytes = 4 << 20 // 4 MiB

var githubImportHTTPGet = http.Get

// NewOSFileResolver returns a FileResolver that resolves paths relative to baseDir.
func NewOSFileResolver(baseDir string) sema.FileResolver {
	return &OSFileResolver{BaseDir: baseDir}
}

func (r *OSFileResolver) Resolve(importingFile string, importPath string, importName string) ([]byte, string, error) {
	// GitHub import: github.com/{user}/{repo}/{path}@{ref}
	if strings.HasPrefix(importPath, "github.com/") {
		return resolveGitHubImport(importPath, importName)
	}
	// Package-style dotted path (e.g. "tolang.registry" with importName="AgentRegistry"):
	// Convert "tolang.registry" → directory "tolang/registry/" and look for importName.{tol,abi,toc,tor}.
	if importName != "" && isPackageStylePath(importPath) {
		return r.resolvePackagePath(importingFile, importPath, importName)
	}
	// Local path: relative to BaseDir or importingFile's directory.
	base := r.BaseDir
	if base == "" {
		base = filepath.Dir(importingFile)
	}
	abs := filepath.Join(base, importPath)
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read %q: %w", abs, err)
	}
	// Detect and convert .toc and .tor artifacts to synthetic TOL source.
	if IsArtifact(raw) {
		src, err := artifactToTOLSource(raw, importName)
		if err != nil {
			return nil, "", fmt.Errorf("import %q: %w", importPath, err)
		}
		return src, abs, nil
	}
	if IsPackage(raw) {
		src, err := artifactToInterfaceSource(raw, importName)
		if err != nil {
			return nil, "", fmt.Errorf("import %q: %w", importPath, err)
		}
		return src, abs, nil
	}
	return raw, abs, nil
}

// isPackageStylePath reports whether importPath looks like a package namespace path
// (e.g. "tolang.registry") rather than a file path. A package path:
//   - contains at least one dot
//   - has no file separators
//   - has no file extension (doesn't end in .tol, .abi, etc.)
//   - is not a relative path (doesn't start with "." or "/")
func isPackageStylePath(p string) bool {
	if strings.HasPrefix(p, ".") || strings.HasPrefix(p, "/") {
		return false
	}
	if strings.ContainsAny(p, "/\\") {
		return false
	}
	if !strings.Contains(p, ".") {
		return false
	}
	// If it ends with a known file extension, it's a file path.
	lower := strings.ToLower(p)
	for _, ext := range []string{".tol", ".abi", ".toc", ".tor", ".json"} {
		if strings.HasSuffix(lower, ext) {
			return false
		}
	}
	return true
}

// resolvePackagePath resolves a package-style import path (e.g. pkgPath="tolang.registry",
// name="AgentRegistry") by converting the dotted path to a directory and looking for
// name.{tol,toi,toc,tor} under that directory.
func (r *OSFileResolver) resolvePackagePath(importingFile, pkgPath, name string) ([]byte, string, error) {
	for _, dir := range r.packageSearchDirs(importingFile, pkgPath) {
		for _, ext := range []string{".tol", ".abi", ".toc", ".tor"} {
			abs := filepath.Join(dir, name+ext)
			raw, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			if IsArtifact(raw) {
				src, err := artifactToTOLSource(raw, name)
				if err != nil {
					return nil, "", fmt.Errorf("package import %q.%q: %w", pkgPath, name, err)
				}
				return src, abs, nil
			}
			if IsPackage(raw) {
				src, err := artifactToInterfaceSource(raw, name)
				if err != nil {
					return nil, "", fmt.Errorf("package import %q.%q: %w", pkgPath, name, err)
				}
				return src, abs, nil
			}
			return raw, abs, nil
		}
	}
	return nil, "", fmt.Errorf("package import %q.%q: no file found in package search roots (tried .tol/.abi/.toc/.tor)", pkgPath, name)
}

// ListPackage implements sema.PackageLister.
// It returns the names of all TOL contracts/interfaces available in pkgPath
// by scanning the corresponding directory (e.g. "tol.lang" -> <base>/tol/lang/).
// Only files with recognised TOL extensions (.tol, .abi, .toc, .tor) are listed;
// the extension is stripped to produce the bare contract name.
func (r *OSFileResolver) ListPackage(pkgPath string) ([]string, error) {
	var names []string
	seen := make(map[string]bool)
	for _, dir := range r.packageSearchDirs("", pkgPath) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			for _, ext := range []string{".tol", ".abi", ".toc", ".tor"} {
				if strings.HasSuffix(strings.ToLower(name), ext) {
					contractName := name[:len(name)-len(ext)]
					if contractName != "" && !seen[contractName] {
						names = append(names, contractName)
						seen[contractName] = true
					}
					break
				}
			}
		}
	}
	return names, nil
}

func (r *OSFileResolver) packageSearchDirs(importingFile, pkgPath string) []string {
	segments := strings.Split(pkgPath, ".")
	if len(segments) == 0 {
		return nil
	}

	var dirs []string
	seen := make(map[string]bool)
	addDir := func(dir string) {
		dir = filepath.Clean(dir)
		if dir == "." || dir == string(filepath.Separator) || seen[dir] {
			return
		}
		seen[dir] = true
		dirs = append(dirs, dir)
	}

	// Priority 1: explicit PackageSearchPaths.
	// For "tolang.openlib.account", check <searchPath>/account/ and
	// <searchPath>/tolang/openlib/account/ and <searchPath>/openlib/account/.
	family := segments[len(segments)-1]
	relAll := filepath.Join(segments...)
	relWithinRoot := ""
	if len(segments) > 1 {
		relWithinRoot = filepath.Join(segments[1:]...)
	}
	for _, sp := range r.PackageSearchPaths {
		addDir(filepath.Join(sp, family))
		addDir(filepath.Join(sp, relAll))
		if relWithinRoot != "" {
			addDir(filepath.Join(sp, relWithinRoot))
		}
	}

	// Priority 2: walk up from BaseDir (legacy heuristic).
	base := r.BaseDir
	if base == "" && importingFile != "" {
		base = filepath.Dir(importingFile)
	}
	if base == "" {
		return dirs
	}

	for cur := filepath.Clean(base); ; cur = filepath.Dir(cur) {
		addDir(filepath.Join(cur, relAll))
		if filepath.Base(cur) == segments[0] && relWithinRoot != "" {
			addDir(filepath.Join(cur, relWithinRoot))
		}
		if len(segments) >= 2 && segments[0] == "tolang" && segments[1] == "openlib" {
			if filepath.Base(cur) == "tolang" {
				addDir(filepath.Join(cur, "openlib", "releases", family))
			}
			addDir(filepath.Join(cur, "tolang", "openlib", "releases", family))
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
	}
	return dirs
}

// resolveGitHubImport fetches a file from GitHub using the raw content API.
// The path must have the form: github.com/{user}/{repo}/{file-path}@{commit}
// where {commit} is a full 40-hex commit SHA. Mutable refs (branches/tags)
// are rejected to preserve reproducible builds.
func resolveGitHubImport(importPath string, importName string) ([]byte, string, error) {
	// Strip "github.com/" prefix.
	rest := importPath[len("github.com/"):]
	// Require @ref specifier.
	atIdx := strings.LastIndex(rest, "@")
	if atIdx < 0 {
		return nil, "", fmt.Errorf(
			"github.com import %q missing @rev specifier (e.g. github.com/user/repo/file.abi@abc1234)",
			importPath,
		)
	}
	ref := rest[atIdx+1:]
	pathPart := rest[:atIdx]
	if ref == "" {
		return nil, "", fmt.Errorf("github.com import %q: @rev is empty", importPath)
	}
	if !isFullGitCommitSHA(ref) {
		return nil, "", fmt.Errorf("github.com import %q: @rev must be a full 40-hex commit SHA", importPath)
	}
	// Split into {user}/{repo}/{file-path}.
	parts := strings.SplitN(pathPart, "/", 3)
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, "", fmt.Errorf(
			"github.com import %q: expected github.com/{user}/{repo}/{path}@{ref}",
			importPath,
		)
	}
	user, repo, filePath := parts[0], parts[1], parts[2]
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s", user, repo, ref, filePath)
	resp, err := githubImportHTTPGet(rawURL) //nolint:noctx
	if err != nil {
		return nil, "", fmt.Errorf("github.com import %q: HTTP request failed: %w", importPath, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("github.com import %q: HTTP %d from %s", importPath, resp.StatusCode, rawURL)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGitHubImportBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("github.com import %q: reading response: %w", importPath, err)
	}
	if len(data) > maxGitHubImportBytes {
		return nil, "", fmt.Errorf("github.com import %q: response exceeds size limit (%d bytes)", importPath, maxGitHubImportBytes)
	}
	// GitHub imports may also be .toc or .tor archives delivered over HTTP.
	if IsArtifact(data) {
		src, err := artifactToTOLSource(data, importName)
		if err != nil {
			return nil, "", fmt.Errorf("github.com import %q: %w", importPath, err)
		}
		return src, rawURL, nil
	}
	if IsPackage(data) {
		src, err := artifactToInterfaceSource(data, importName)
		if err != nil {
			return nil, "", fmt.Errorf("github.com import %q: %w", importPath, err)
		}
		return src, rawURL, nil
	}
	return data, rawURL, nil
}

func isFullGitCommitSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil
}

// artifactToTOLSource decodes a .toc artifact and generates a synthetic TOL source
// declaring an interface named importName with the contract's public functions.
func artifactToTOLSource(data []byte, importName string) ([]byte, error) {
	art, err := DecodeArtifact(data)
	if err != nil {
		return nil, fmt.Errorf("decode .toc: %w", err)
	}
	var abi struct {
		Functions []struct {
			Name       string   `json:"name"`
			Visibility string   `json:"visibility"`
			Selector   string   `json:"selector"`
			Params     []string `json:"params"`
			Returns    []string `json:"returns"`
		} `json:"functions"`
		Events []struct {
			Name   string   `json:"name"`
			Params []string `json:"params"`
		} `json:"events"`
	}
	if err := json.Unmarshal(art.ABIJSON, &abi); err != nil {
		return nil, fmt.Errorf("decode artifact ABI: %w", err)
	}
	var b strings.Builder
	b.WriteString("pragma tolang 0.2.0;\n\ninterface ")
	b.WriteString(importName)
	b.WriteString(" {\n")
	for _, fn := range abi.Functions {
		if fn.Selector != "" {
			b.WriteString("  @selector(\"")
			b.WriteString(fn.Selector)
			b.WriteString("\")\n")
		}
		b.WriteString("  function ")
		b.WriteString(fn.Name)
		b.WriteString("(")
		for i, p := range fn.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s arg%d", p, i)
		}
		b.WriteString(")")
		if len(fn.Returns) > 0 {
			b.WriteString(" returns (")
			for i, r := range fn.Returns {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%s ret%d", r, i)
			}
			b.WriteString(")")
		}
		vis := fn.Visibility
		if vis == "" {
			vis = "external"
		}
		b.WriteString(" ")
		b.WriteString(vis)
		b.WriteString(";\n")
	}
	for _, ev := range abi.Events {
		b.WriteString("  event ")
		b.WriteString(ev.Name)
		b.WriteString("(")
		for i, p := range ev.Params {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%s arg%d", p, i)
		}
		b.WriteString(");\n")
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

// artifactToInterfaceSource decodes a .tor archive and returns the .abi source for importName.
// It uses manifest.json to locate the .abi file for the contract whose name matches
// importName (or "I" + importName for typical IContractName interface naming).
func artifactToInterfaceSource(data []byte, importName string) ([]byte, error) {
	tor, err := DecodePackage(data)
	if err != nil {
		return nil, fmt.Errorf("decode .tor: %w", err)
	}
	var manifest struct {
		Contracts []struct {
			Name      string `json:"name"`
			Interface string `json:"abi"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(tor.ManifestJSON, &manifest); err != nil {
		return nil, fmt.Errorf("decode .tor manifest: %w", err)
	}
	// Find the contract whose .abi file declares the named interface.
	// The manifest contract name is the concrete contract name (e.g. "Token"),
	// while the .abi typically declares interface "IToken". We match by scanning
	// .abi files for the requested importName.
	for _, c := range manifest.Contracts {
		ifacePath := strings.TrimSpace(c.Interface)
		if ifacePath == "" {
			continue
		}
		abiSrc, ok := tor.Files[ifacePath]
		if !ok {
			continue
		}
		// Check whether this .abi declares importName.
		if abiDeclaresName(abiSrc, importName) {
			return abiSrc, nil
		}
	}
	// Fallback: deterministically scan all .abi files in the archive.
	// Reject ambiguous matches instead of depending on Go map iteration order.
	var abiPaths []string
	for path := range tor.Files {
		if !strings.HasSuffix(path, ".abi") {
			continue
		}
		abiPaths = append(abiPaths, path)
	}
	sort.Strings(abiPaths)
	var matchedPaths []string
	for _, path := range abiPaths {
		content := tor.Files[path]
		if abiDeclaresName(content, importName) {
			matchedPaths = append(matchedPaths, path)
		}
	}
	switch len(matchedPaths) {
	case 0:
		return nil, fmt.Errorf("no interface or library named %q found in .tor archive", importName)
	case 1:
		return tor.Files[matchedPaths[0]], nil
	default:
		return nil, fmt.Errorf("ambiguous interface or library %q in .tor archive: %s", importName, strings.Join(matchedPaths, ", "))
	}
}

// abiDeclaresName reports whether a .abi source declares an interface or library
// with the given name. Uses a lightweight string scan to avoid full parse.
func abiDeclaresName(src []byte, name string) bool {
	s := string(src)
	return strings.Contains(s, "interface "+name+" {") ||
		strings.Contains(s, "interface "+name+"{") ||
		strings.Contains(s, "library "+name+" {") ||
		strings.Contains(s, "library "+name+"{")
}

// CompileOptions controls bytecode debug metadata emission.
type CompileOptions struct {
	// IncludeSourceMap controls whether debug metadata (source map / line table /
	// local/call/upvalue debug sections) is embedded in output bytecode.
	// Default is false for reproducible builds.
	IncludeSourceMap bool

	// PackageSearchPaths provides explicit directories where package-style
	// imports are resolved before falling back to the directory-walk heuristic.
	// This eliminates the need for synthetic compile paths in tests.
	PackageSearchPaths []string
}

const defaultIncludeSourceMap = false

// ParseModule parses TOL source into a syntax tree.
func ParseModule(source []byte, name string) (*ast.Module, error) {
	mod, diags := parser.ParseFile(name, source)
	if diags.HasErrors() {
		return nil, diags
	}
	return mod, nil
}

// BuildIR parses and type-checks TOL source and prepares lowering to VM IR.
// Import declarations are resolved relative to the directory of name using the OS filesystem.
func BuildIR(source []byte, name string) (*IRProgram, error) {
	return BuildIRWithResolver(source, name, NewOSFileResolver(filepath.Dir(name)))
}

// BuildIRWithResolver is like BuildIR but uses the provided resolver
// for import resolution. Pass nil to disable import resolution.
func BuildIRWithResolver(source []byte, name string, resolver sema.FileResolver) (*IRProgram, error) {
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	typed, diags := sema.CheckWithResolver(name, mod, resolver)
	if diags.HasErrors() {
		return nil, diags
	}
	prog, err := lower.FromTyped(typed)
	if err != nil {
		return nil, err
	}
	return BuildIRFromLowered(prog, name)
}

// CompileBytecode compiles TOL source into deterministic bytecode.
func CompileBytecode(source []byte, name string) ([]byte, error) {
	return CompileBytecodeWithOptions(source, name, nil)
}

// CompileBytecodeWithOptions compiles TOL source into deterministic
// bytecode with configurable debug metadata emission.
func CompileBytecodeWithOptions(source []byte, name string, opts *CompileOptions) ([]byte, error) {
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	resolver := &OSFileResolver{BaseDir: filepath.Dir(name)}
	if opts != nil && len(opts.PackageSearchPaths) > 0 {
		resolver.PackageSearchPaths = opts.PackageSearchPaths
	}
	typed, diags := sema.CheckWithResolver(name, mod, resolver)
	if diags.HasErrors() {
		return nil, diags
	}
	prog, lerr := lower.FromTyped(typed)
	if lerr != nil {
		return nil, lerr
	}
	irp, err := BuildIRFromLowered(prog, name)
	if err != nil {
		return nil, err
	}
	proto, err := CompileIR(irp)
	if err != nil {
		return nil, err
	}
	includeSourceMap := defaultIncludeSourceMap
	if opts != nil {
		includeSourceMap = opts.IncludeSourceMap
	}
	if !includeSourceMap {
		stripFunctionProtoDebug(proto)
	}
	return EncodeFunctionProto(proto)
}

// BuildIRFromLowered lowers a typed/lowered TOL program directly into VM IR (runtime mode).
// The emitted artifact contains tos.oninvoke dispatch but no constructor entry point.
func BuildIRFromLowered(prog *lower.Program, name string) (*IRProgram, error) {
	return buildDirectIRFromLowered(prog, name, bootstrapModeRuntime)
}

// BuildIRFromLoweredInit lowers a typed/lowered TOL program into VM IR for the init artifact.
// The emitted artifact contains only the constructor, called directly at module top-level
// (no tos.oninvoke, no tos.oncreate wrapper). Used for init_code in the init/runtime split.
func BuildIRFromLoweredInit(prog *lower.Program, name string) (*IRProgram, error) {
	return buildDirectIRFromLowered(prog, name, bootstrapModeInit)
}

// CompileInitBytecode compiles TOL source into init-mode bytecode (constructor only,
// no tos.oninvoke dispatch). The init artifact is used for constructor-at-deploy-time
// execution and exposes tos.oncreate for test-path constructor calls.
func CompileInitBytecode(source []byte, name string) ([]byte, error) {
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	typed, diags := sema.CheckWithResolver(name, mod, NewOSFileResolver(filepath.Dir(name)))
	if diags.HasErrors() {
		return nil, diags
	}
	prog, err := lower.FromTyped(typed)
	if err != nil {
		return nil, err
	}
	irp, err := BuildIRFromLoweredInit(prog, name)
	if err != nil {
		return nil, err
	}
	proto, err := CompileIR(irp)
	if err != nil {
		return nil, err
	}
	stripFunctionProtoDebug(proto)
	return EncodeFunctionProto(proto)
}

// CompileBytecodeFromLowered compiles a lowered TOL program into deterministic bytecode.
func CompileBytecodeFromLowered(prog *lower.Program, name string) ([]byte, error) {
	irp, err := BuildIRFromLowered(prog, name)
	if err != nil {
		return nil, err
	}
	proto, err := CompileIR(irp)
	if err != nil {
		return nil, err
	}
	stripFunctionProtoDebug(proto)
	return EncodeFunctionProto(proto)
}

func stripFunctionProtoDebug(p *FunctionProto) {
	if p == nil {
		return
	}
	p.SourceName = ""
	p.DbgSourcePositions = nil
	p.DbgLocals = nil
	p.DbgCalls = nil
	p.DbgUpvalues = nil
	for _, child := range p.FunctionPrototypes {
		stripFunctionProtoDebug(child)
	}
}

// BuildLowered builds typed and lowered TOL program for diagnostics/testing.
// Import declarations are resolved relative to the directory of name using the OS filesystem.
func BuildLowered(source []byte, name string) (*lower.Program, error) {
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	typed, diags := sema.CheckWithResolver(name, mod, NewOSFileResolver(filepath.Dir(name)))
	if diags.HasErrors() {
		return nil, diags
	}
	return lower.FromTyped(typed)
}

// CompileAllArtifacts compiles all contracts in a multi-contract TOL source file,
// returning one Artifact per contract.
func CompileAllArtifacts(source []byte, name string, opts *CompileOptions) ([]*Artifact, error) {
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	resolver := NewOSFileResolver(filepath.Dir(name))
	typed, diags := sema.CheckWithResolver(name, mod, resolver)
	if diags.HasErrors() {
		return nil, diags
	}
	progs, err := lower.FromTypedAll(typed)
	if err != nil {
		return nil, err
	}

	includeSourceMap := defaultIncludeSourceMap
	if opts != nil {
		includeSourceMap = opts.IncludeSourceMap
	}

	artifacts := make([]*Artifact, 0, len(progs))
	for i, prog := range progs {
		c := &mod.Contracts[i]

		irp, err := BuildIRFromLowered(prog, name)
		if err != nil {
			return nil, fmt.Errorf("contract %s: build IR: %w", c.Name, err)
		}
		proto, err := CompileIR(irp)
		if err != nil {
			return nil, fmt.Errorf("contract %s: compile IR: %w", c.Name, err)
		}
		if !includeSourceMap {
			stripFunctionProtoDebug(proto)
		}
		bytecode, err := EncodeFunctionProto(proto)
		if err != nil {
			return nil, fmt.Errorf("contract %s: encode bytecode: %w", c.Name, err)
		}

		contractName, abiJSON, storageJSON, err := buildArtifactMetadataForContract(mod, c)
		if err != nil {
			return nil, fmt.Errorf("contract %s: build metadata: %w", c.Name, err)
		}
		maxSlots, bcLen, unbounded, err := analyzeBytecodeMetadata(bytecode)
		if err != nil {
			return nil, fmt.Errorf("contract %s: analyze bytecode: %w", c.Name, err)
		}
		art := &Artifact{
			Version:               ArtifactFormatVersion,
			Compiler:              "tolang/" + PackageVersion,
			ContractName:          contractName,
			Bytecode:              bytecode,
			ABIJSON:               abiJSON,
			StorageLayoutJSON:     storageJSON,
			SourceHash:            keccak256Hex(source),
			BytecodeHash:          keccak256Hex(bytecode),
			MaxStackSlots:         maxSlots,
			BytecodeLen:           bcLen,
			ContainsUnboundedLoop: unbounded,
		}
		artifacts = append(artifacts, art)
	}
	return artifacts, nil
}
