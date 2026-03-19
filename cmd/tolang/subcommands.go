package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	lua "github.com/tos-network/tolang"
)

func dispatchSubcommand(args []string) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	switch name := args[0]; name {
	case "compile", "pack", "inspect", "verify", "test", "lsp", "fmt", "format":
		return true, runNamedSubcommand(name, args[1:])
	case "--version", "version":
		fmt.Println(lua.PackageCopyRight)
		return true, 0
	case "--help", "-h":
		printRootSubcommandUsage()
		return true, 0
	case "help":
		if len(args) == 1 {
			printRootSubcommandUsage()
			return true, 0
		}
		return true, runNamedSubcommand(args[1], []string{"--help"})
	default:
		return false, 0
	}
}

func runNamedSubcommand(name string, args []string) int {
	switch name {
	case "compile":
		return cmdCompile(args)
	case "pack":
		return cmdPack(args)
	case "inspect":
		return cmdInspect(args)
	case "verify":
		return cmdVerify(args)
	case "test":
		return cmdTest(args)
	case "lsp":
		return cmdLSP(args)
	case "fmt", "format":
		return cmdFormat(args)
	default:
		fmt.Printf("unknown subcommand %q\n", name)
		return 1
	}
}

func printRootSubcommandUsage() {
	fmt.Print(`Usage:
  tol <subcommand> [flags] <inputs...>
  tol [lua-options] [script [args]]

Subcommands:
  compile   compile .tol source to .toc/.abi/.tor
  pack      package a directory with manifest.json into .tor
  inspect   inspect .toc/.abi/.tor metadata
  verify    verify .toc/.abi/.tor integrity
  test      run *_test.tol test files
  fmt       format .tol source files
  lsp       start Language Server Protocol server (stdin/stdout)

Global:
  --version print version
  --help    print this help
  help      print help for a subcommand (e.g., "tol help compile")
`)
}

func cmdCompile(args []string) int {
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var emit, output, name, packageName, packageVersion, signKey string
	var includeSource, emitABI, dumpAST, emitSourceMap bool
	fs.StringVar(&emit, "emit", "toc", "emit format: toc|abi|tor")
	fs.StringVar(&output, "o", "", "output artifact path")
	fs.StringVar(&output, "output", "", "output artifact path")
	fs.StringVar(&name, "name", "", "name override: interface name for emit=abi, abi interface name for emit=tor")
	fs.StringVar(&packageName, "package-name", "", "package name override (tor)")
	fs.StringVar(&packageVersion, "package-version", "0.0.0", "package version override (tor)")
	fs.BoolVar(&includeSource, "include-source", false, "include source in .tor")
	fs.BoolVar(&emitSourceMap, "sourcemap", false, "embed source map/debug metadata into emitted bytecode artifacts")
	fs.BoolVar(&emitABI, "abi", false, "write .abi.json alongside .toc")
	fs.BoolVar(&dumpAST, "ast", false, "dump parsed TOL module")
	fs.StringVar(&signKey, "sign", "", "ed25519 private key seed file (32-byte hex) to sign the .tor package")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol compile [--emit toc|abi|tor] [-o <output>] [options] <input.tol>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "compile requires exactly one input .tol file")
		fs.Usage()
		return 1
	}

	input := fs.Arg(0)
	source, err := os.ReadFile(input)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}

	if dumpAST {
		mod, err := lua.ParseModule(source, input)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println(mod.String())
		// Keep `tol compile --ast input.tol` as inspect-only unless explicitly asked to emit files.
		if output == "" && !emitABI && strings.EqualFold(strings.TrimSpace(emit), "toc") {
			return 0
		}
	}

	emit = strings.ToLower(strings.TrimSpace(emit))
	if emit == "" {
		emit = "toc"
	}
	switch emit {
	case "toc", "abi", "tor":
	default:
		fmt.Printf("unsupported --emit value %q (expected toc|abi|tor)\n", emit)
		return 1
	}

	if output == "" {
		output = defaultArtifactPath(input, emit)
	}
	if emitABI && emit != "toc" {
		fmt.Println("--abi flag is only valid with --emit toc")
		return 1
	}
	if emit != "tor" && (strings.TrimSpace(packageName) != "" || includeSource || fs.Lookup("package-version").Value.String() != "0.0.0") {
		fmt.Println("--package-name/--package-version/--include-source are only valid with --emit tor")
		return 1
	}

	switch emit {
	case "toc":
		artifactBytes, err := lua.CompileArtifactWithOptions(source, input, &lua.ArtifactOptions{
			IncludeSourceMap: emitSourceMap,
		})
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(output, artifactBytes, 0o644); err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if emitABI {
			decoded, err := lua.DecodeArtifact(artifactBytes)
			if err != nil {
				fmt.Println(err.Error())
				return 1
			}
			abiPath := strings.TrimSuffix(output, filepath.Ext(output)) + ".abi.json"
			abi := decoded.ABIJSON
			if len(abi) == 0 {
				abi = []byte("{}")
			}
			if err := os.WriteFile(abiPath, abi, 0o644); err != nil {
				fmt.Println(err.Error())
				return 1
			}
		}
	case "abi":
		ifaceBytes, err := lua.CompileInterfaceWithOptions(source, input, &lua.InterfaceOptions{
			InterfaceName: strings.TrimSpace(name),
		})
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(output, ifaceBytes, 0o644); err != nil {
			fmt.Println(err.Error())
			return 1
		}
	case "tor":
		if strings.TrimSpace(packageName) == "" {
			packageName = inputStem(input)
		}
		signingKey, err := loadSigningKey(signKey)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		sm := emitSourceMap
		tor, err := lua.CompilePackage(source, input, &lua.PackageOptions{
			PackageName:      strings.TrimSpace(packageName),
			PackageVersion:   strings.TrimSpace(packageVersion),
			InterfaceName: strings.TrimSpace(name),
			IncludeSourceMap: &sm,
			IncludeSource:    includeSource,
			SigningKey:       signingKey,
		})
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if err := os.WriteFile(output, tor, 0o644); err != nil {
			fmt.Println(err.Error())
			return 1
		}
	}
	return 0
}

func cmdPack(args []string) int {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var output, signKey string
	fs.StringVar(&output, "o", "", "output .tor path")
	fs.StringVar(&output, "output", "", "output .tor path")
	fs.StringVar(&signKey, "sign", "", "ed25519 private key seed file (32-byte hex) to sign the .tor package")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol pack -o <output.tor> [--sign <keyfile>] <directory>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "pack requires exactly one input directory")
		fs.Usage()
		return 1
	}
	if strings.TrimSpace(output) == "" {
		fmt.Fprintln(os.Stderr, "pack requires -o/--output")
		return 1
	}
	input := fs.Arg(0)
	info, err := os.Stat(input)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if !info.IsDir() {
		fmt.Println("pack input must be a directory")
		return 1
	}
	manifest, files, err := collectPackageInputs(input)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	signingKey, err := loadSigningKey(signKey)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	tor, err := lua.EncodePackage(manifest, files)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if len(signingKey) > 0 {
		tor, err = lua.SignPackage(tor, signingKey)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
	}
	if err := os.WriteFile(output, tor, 0o644); err != nil {
		fmt.Println(err.Error())
		return 1
	}
	return 0
}

func cmdInspect(args []string) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "output JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol inspect [--json] <artifact>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "inspect requires exactly one artifact path")
		fs.Usage()
		return 1
	}

	path := fs.Arg(0)
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	switch detectArtifactKind(path, body) {
	case kindArtifact:
		return inspectArtifact(body, asJSON)
	case kindInterface:
		return inspectInterface(body, asJSON)
	case kindPackage:
		return inspectPackage(body, asJSON)
	default:
		fmt.Println("unknown artifact type (expected .toc, .abi, or .tor)")
		return 1
	}
}

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var sourcePath string
	fs.StringVar(&sourcePath, "source", "", "source file for artifact source_hash verification")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: tol verify [--source <file>] <artifact>")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "verify requires exactly one artifact path")
		fs.Usage()
		return 1
	}

	path := fs.Arg(0)
	body, err := os.ReadFile(path)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}

	switch detectArtifactKind(path, body) {
	case kindArtifact:
		art, err := lua.DecodeArtifact(body)
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		if strings.TrimSpace(sourcePath) != "" {
			source, err := os.ReadFile(sourcePath)
			if err != nil {
				fmt.Println(err.Error())
				return 1
			}
			if err := lua.VerifySourceHash(art, source); err != nil {
				fmt.Println(err.Error())
				return 2
			}
		}
		fmt.Println("artifact: ok")
		return 0
	case kindInterface:
		if strings.TrimSpace(sourcePath) != "" {
			fmt.Println("--source is only valid for .toc artifacts")
			return 1
		}
		if err := lua.ValidateInterface(body); err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println("interface: ok")
		return 0
	case kindPackage:
		if strings.TrimSpace(sourcePath) != "" {
			fmt.Println("--source is only valid for .toc artifacts")
			return 1
		}
		if _, err := lua.DecodePackage(body); err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println("package: ok")
		return 0
	default:
		fmt.Println("unknown artifact type (expected .toc, .abi, or .tor)")
		return 1
	}
}

func defaultArtifactPath(input, emit string) string {
	base := strings.TrimSuffix(input, filepath.Ext(input))
	return base + "." + emit
}

func inputStem(input string) string {
	base := filepath.Base(input)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

type artifactKind int

const (
	artifactUnknown artifactKind = iota
	kindArtifact
	kindInterface
	kindPackage
)

func detectArtifactKind(path string, body []byte) artifactKind {
	switch strings.ToLower(strings.TrimSpace(filepath.Ext(path))) {
	case ".toc":
		return kindArtifact
	case ".abi":
		return kindInterface
	case ".tor":
		return kindPackage
	}
	if lua.IsArtifact(body) {
		return kindArtifact
	}
	if lua.IsPackage(body) {
		return kindPackage
	}
	if err := lua.ValidateInterface(body); err == nil {
		return kindInterface
	}
	return artifactUnknown
}

func inspectArtifact(body []byte, asJSON bool) int {
	art, err := lua.DecodeArtifact(body)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if _, err := lua.DecodeFunctionProto(art.Bytecode); err != nil {
		fmt.Printf("invalid embedded bytecode: %v\n", err)
		return 1
	}
	if asJSON {
		out := struct {
			Version               uint16          `json:"version"`
			Compiler              string          `json:"compiler"`
			ContractName          string          `json:"contract_name"`
			BytecodeLen           uint32          `json:"bytecode_len"`
			MaxStackSlots         uint32          `json:"max_stack_slots"`
			ContainsUnboundedLoop bool            `json:"contains_unbounded_loop"`
			SourceHash            string          `json:"source_hash"`
			BytecodeHash          string          `json:"bytecode_hash"`
			ABIJSON               json.RawMessage `json:"abi_json,omitempty"`
			StorageJSON           json.RawMessage `json:"storage_json,omitempty"`
		}{
			Version:               art.Version,
			Compiler:              art.Compiler,
			ContractName:          art.ContractName,
			BytecodeLen:           art.BytecodeLen,
			MaxStackSlots:         art.MaxStackSlots,
			ContainsUnboundedLoop: art.ContainsUnboundedLoop,
			SourceHash:            art.SourceHash,
			BytecodeHash:          art.BytecodeHash,
		}
		if len(art.ABIJSON) > 0 {
			out.ABIJSON = json.RawMessage(art.ABIJSON)
		}
		if len(art.StorageLayoutJSON) > 0 {
			out.StorageJSON = json.RawMessage(art.StorageLayoutJSON)
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	fmt.Printf("Artifact version: %d\n", art.Version)
	fmt.Printf("Compiler: %s\n", art.Compiler)
	fmt.Printf("Contract: %s\n", art.ContractName)
	fmt.Printf("Bytecode bytes: %d\n", len(art.Bytecode))
	fmt.Printf("Bytecode decode: ok\n")
	fmt.Printf("Max stack slots: %d\n", art.MaxStackSlots)
	fmt.Printf("Contains unbounded loop: %v\n", art.ContainsUnboundedLoop)
	fmt.Printf("Source hash: %s\n", art.SourceHash)
	fmt.Printf("Bytecode hash: %s\n", art.BytecodeHash)
	if len(art.ABIJSON) > 0 {
		fmt.Printf("ABI JSON: %s\n", string(art.ABIJSON))
	}
	if len(art.StorageLayoutJSON) > 0 {
		fmt.Printf("Storage JSON: %s\n", string(art.StorageLayoutJSON))
	}
	return 0
}

func inspectInterface(body []byte, asJSON bool) int {
	info, err := lua.InspectInterface(body)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if asJSON {
		out := struct {
			Version       string `json:"version"`
			InterfaceName string `json:"interface_name"`
			FunctionCount int    `json:"function_count"`
			EventCount    int    `json:"event_count"`
		}{
			Version:       info.Version,
			InterfaceName: info.InterfaceName,
			FunctionCount: info.FunctionCount,
			EventCount:    info.EventCount,
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println(string(b))
		return 0
	}
	fmt.Printf("Interface version: %s\n", info.Version)
	fmt.Printf("Interface: %s\n", info.InterfaceName)
	fmt.Printf("Functions: %d\n", info.FunctionCount)
	fmt.Printf("Events: %d\n", info.EventCount)
	return 0
}

func inspectPackage(body []byte, asJSON bool) int {
	tor, err := lua.DecodePackage(body)
	if err != nil {
		fmt.Println(err.Error())
		return 1
	}
	if asJSON {
		type torFileInfo struct {
			Path  string `json:"path"`
			Bytes int    `json:"bytes"`
		}
		names := make([]string, 0, len(tor.Files))
		for name := range tor.Files {
			names = append(names, name)
		}
		sort.Strings(names)
		infos := make([]torFileInfo, 0, len(names))
		for _, name := range names {
			infos = append(infos, torFileInfo{
				Path:  name,
				Bytes: len(tor.Files[name]),
			})
		}
		out := struct {
			ManifestJSON json.RawMessage `json:"manifest_json"`
			FileCount    int             `json:"file_count"`
			Files        []torFileInfo   `json:"files"`
			PackageHash  string          `json:"package_hash"`
		}{
			ManifestJSON: json.RawMessage(tor.ManifestJSON),
			FileCount:    len(tor.Files),
			Files:        infos,
			PackageHash:  lua.PackageHash(body),
		}
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Println(err.Error())
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	fmt.Printf("Manifest JSON: %s\n", string(tor.ManifestJSON))
	fmt.Printf("Files: %d\n", len(tor.Files))
	names := make([]string, 0, len(tor.Files))
	for name := range tor.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf(" - %s (%d bytes)\n", name, len(tor.Files[name]))
	}
	fmt.Printf("Package hash: %s\n", lua.PackageHash(body))
	return 0
}

// loadSigningKey reads an ed25519 private key seed from a file.
// The file must contain a 64-character hex string (32-byte seed).
// Returns nil if path is empty (no signing requested).
func loadSigningKey(path string) ([]byte, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sign: cannot read key file %q: %w", path, err)
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, fmt.Errorf("sign: key file must contain a 64-character hex string (32-byte ed25519 seed): %w", err)
	}
	if len(seed) != 32 {
		return nil, fmt.Errorf("sign: key file must be exactly 32 bytes (64 hex chars), got %d bytes", len(seed))
	}
	return seed, nil
}
