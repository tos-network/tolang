package lua

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tolast "github.com/tos-network/tolang/tol/ast"
	"github.com/tos-network/tolang/tol/diag"
	"github.com/tos-network/tolang/tol/lower"
	"github.com/tos-network/tolang/tol/sema"
)

var torZipMagic = [4]byte{'P', 'K', 0x03, 0x04}

const torManifestPath = "manifest.json"

var torDeterministicModTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// Package is a decoded .tor archive payload.
type Package struct {
	ManifestJSON []byte
	Files        map[string][]byte // excludes manifest.json
}

// PackageOptions configures one-shot .tol -> .tor compilation.
type PackageOptions struct {
	PackageName      string
	PackageVersion   string
	ArtifactPath     string
	InterfacePath    string
	InterfaceName    string
	// IncludeSourceMap controls whether embedded .toc bytecode contains source map/debug metadata.
	// nil means default-on (backward compatible).
	IncludeSourceMap *bool
	IncludeSource    bool
	SourcePath       string
	// SigningKey is an ed25519 private key seed (32 bytes) used to sign the package.
	// If nil, the package is produced unsigned. Unsigned packages are always accepted.
	SigningKey []byte
}

// IsPackage reports whether input starts with local-file ZIP magic.
func IsPackage(data []byte) bool {
	if len(data) < len(torZipMagic) {
		return false
	}
	for i := range torZipMagic {
		if data[i] != torZipMagic[i] {
			return false
		}
	}
	return true
}

// PackageHash computes keccak256 hash of a .tor archive.
func PackageHash(data []byte) string {
	return keccak256Hex(data)
}

// CompilePackage compiles source into a deterministic .tor package.
// For single-contract files the behaviour is identical to before.
// For multi-contract files every contract is compiled to its own .toc/.abi entry.
// PackageOptions path overrides (ArtifactPath, InterfacePath, InterfaceName) are applied only
// when there is exactly one contract; they are ignored for multi-contract packages.
func CompilePackage(source []byte, name string, opts *PackageOptions) ([]byte, error) {
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	if mod == nil || len(mod.Contracts) == 0 {
		return nil, fmt.Errorf("tor compile requires at least one contract declaration")
	}

	pkgVersion := "0.1.0"
	includeSource := false
	sourcePath := ""
	pkgName := strings.ToLower(mod.Contracts[0].Name)
	if len(mod.Contracts) > 1 {
		// For multi-contract: derive package name from file basename.
		base := filepath.Base(name)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		if base != "" && base != "." {
			pkgName = base
		}
	}

	var artifactPathOverride, interfacePathOverride, interfaceNameOverride string
	if opts != nil {
		if strings.TrimSpace(opts.PackageName) != "" {
			pkgName = strings.TrimSpace(opts.PackageName)
		}
		if strings.TrimSpace(opts.PackageVersion) != "" {
			pkgVersion = strings.TrimSpace(opts.PackageVersion)
		}
		if len(mod.Contracts) == 1 {
			// Path overrides only apply to single-contract packages.
			artifactPathOverride = strings.TrimSpace(opts.ArtifactPath)
			interfacePathOverride = strings.TrimSpace(opts.InterfacePath)
			interfaceNameOverride = strings.TrimSpace(opts.InterfaceName)
		}
		includeSource = opts.IncludeSource
		sourcePath = strings.TrimSpace(opts.SourcePath)
	}
	// Source-declared package path takes highest priority (overrides opts.PackageName).
	if strings.TrimSpace(mod.Package) != "" {
		pkgName = strings.TrimSpace(mod.Package)
	}
	// Security: tol.lang is the reserved platform namespace.
	// External packages must not claim this name; reject before any artifact is produced.
	if pkgName == sema.TolLangPackage || strings.HasPrefix(pkgName, sema.TolLangPackage+".") {
		return nil, fmt.Errorf("[%s] package name %q is reserved for the TOS platform and cannot be used in external packages", diag.CodeSemaTolLangReserved, pkgName)
	}
	if includeSource && sourcePath == "" {
		base := filepath.Base(name)
		if base == "." || base == "/" || base == string(filepath.Separator) || base == "" {
			base = pkgName + ".tol"
		}
		sourcePath = "sources/" + base
	}

	type manifestContract struct {
		Name string `json:"name"`
		Artifact  string `json:"toc"`
		Interface string `json:"abi"`
	}
	files := map[string][]byte{}
	var manifestContracts []manifestContract

	// main_contract and init_code are set when a single contract with a constructor
	// is compiled (init/runtime split for constructor-at-deploy-time execution).
	var mainContract string
	var initCodePath string

	// Compile each concrete contract.
	for i := range mod.Contracts {
		c := &mod.Contracts[i]
		cname := strings.TrimSpace(c.Name)

		// Build runtime bytecode for this specific contract.
		// Runtime artifact: tos.oninvoke dispatch, no constructor entry point.
		// We reuse CompileArtifactWithOptions only for single-contract files.
		// For multi-contract, compile the program directly.
		var artifactBytes []byte
		if len(mod.Contracts) == 1 {
			artifactBytes, err = CompileArtifactWithOptions(source, name, &ArtifactOptions{
				IncludeSourceMap: torIncludeSourceMap(opts),
			})
		} else {
			artifactBytes, err = compileContractToArtifact(source, name, c, torIncludeSourceMap(opts), false)
		}
		if err != nil {
			return nil, fmt.Errorf("contract %s: %w", cname, err)
		}

		artifactPath := fmt.Sprintf("bytecode/%s.toc", cname)
		interfacePath := fmt.Sprintf("interfaces/I%s.abi", cname)
		if len(mod.Contracts) == 1 {
			if artifactPathOverride != "" {
				artifactPath = artifactPathOverride
			}
			if interfacePathOverride != "" {
				interfacePath = interfacePathOverride
			}
		}

		// Generate init artifact when the contract has a constructor or storage slot
		// initializers.  The init artifact is executed once at deploy time (analogous to
		// EVM initcode) and is NOT stored on-chain; only the runtime artifact is.
		if contractNeedsInitCode(c) {
			initBytes, initErr := compileContractToArtifact(source, name, c, torIncludeSourceMap(opts), true)
			if initErr != nil {
				return nil, fmt.Errorf("contract %s: init artifact: %w", cname, initErr)
			}
			itPath := fmt.Sprintf("init/%s_init.toc", cname)
			files[itPath] = initBytes
			// Record main_contract + init_code only for the first concrete contract
			// (single main contract per package is the expected use-case).
			if mainContract == "" {
				mainContract = cname
				initCodePath = itPath
			}
		}

		// Generate .abi.
		iface, err := BuildInterfaceWithOptions(mod, &InterfaceOptions{
			InterfaceName: func() string {
				if len(mod.Contracts) == 1 && interfaceNameOverride != "" {
					return interfaceNameOverride
				}
				return "I" + cname
			}(),
			ContractName: cname,
		})
		if err != nil {
			return nil, fmt.Errorf("contract %s: build interface: %w", cname, err)
		}

		files[artifactPath] = artifactBytes
		files[interfacePath] = iface
		manifestContracts = append(manifestContracts, manifestContract{
			Name: cname, Artifact: artifactPath, Interface: interfacePath,
		})
	}

	// Add top-level interface declarations as .abi-only entries (no .toc).
	for _, iface := range mod.Interfaces {
		iname := strings.TrimSpace(iface.Name)
		if iname == "" {
			continue
		}
		ifacePath := fmt.Sprintf("interfaces/%s.abi", iname)
		if _, exists := files[ifacePath]; !exists {
			ifaceSrc, err := BuildInterfaceForDecl(mod, &iface)
			if err == nil && len(ifaceSrc) > 0 {
				files[ifacePath] = ifaceSrc
				manifestContracts = append(manifestContracts, manifestContract{
					Name: iname, Interface: ifacePath,
				})
			}
		}
	}

	manifest := struct {
		Name         string             `json:"name"`
		Package      string             `json:"package,omitempty"`
		Version      string             `json:"version"`
		MainContract string             `json:"main_contract,omitempty"`
		InitCode     string             `json:"init_code,omitempty"`
		Contracts    []manifestContract `json:"contracts"`
	}{
		Name:         pkgName,
		Package:      mod.Package,
		Version:      pkgVersion,
		MainContract: mainContract,
		InitCode:     initCodePath,
		Contracts:    manifestContracts,
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if includeSource {
		files[sourcePath] = source
	}
	pkgBytes, err := EncodePackage(manifestJSON, files)
	if err != nil {
		return nil, err
	}
	if opts != nil && len(opts.SigningKey) > 0 {
		return SignPackage(pkgBytes, opts.SigningKey)
	}
	return pkgBytes, nil
}

// compileContractToArtifact compiles a single ContractDecl from a pre-parsed module to .toc bytes.
// Used by CompilePackage for multi-contract files where each contract needs independent compilation.
// If initArtifact is true, the artifact is compiled in init mode (constructor-only, no dispatch).
func compileContractToArtifact(source []byte, name string, c *tolast.ContractDecl, includeSourceMap bool, initArtifact bool) ([]byte, error) {
	mod, err := ParseModule(source, name)
	if err != nil {
		return nil, err
	}
	// Point mod.Contract to the contract we want to compile (sema uses m.Contract as primary).
	for i := range mod.Contracts {
		if mod.Contracts[i].Name == c.Name {
			mod.Contract = &mod.Contracts[i]
			break
		}
	}
	resolver := NewOSFileResolver(filepath.Dir(name))
	typed, diags := sema.CheckWithResolver(name, mod, resolver)
	if diags.HasErrors() {
		return nil, diags
	}
	prog, err := lower.FromTypedContract(typed, mod.Contract)
	if err != nil {
		return nil, err
	}
	var irp *IRProgram
	if initArtifact {
		irp, err = BuildIRFromLoweredInit(prog, name)
	} else {
		irp, err = BuildIRFromLowered(prog, name)
	}
	if err != nil {
		return nil, err
	}
	proto, err := CompileIR(irp)
	if err != nil {
		return nil, err
	}
	if !includeSourceMap {
		stripFunctionProtoDebug(proto)
	}
	bytecode, err := EncodeFunctionProto(proto)
	if err != nil {
		return nil, err
	}
	contractName, abiJSON, storageJSON, err := buildArtifactMetadataForContract(mod.Contract)
	if err != nil {
		return nil, err
	}
	maxSlots, bcLen, unbounded, err := analyzeBytecodeMetadata(bytecode)
	if err != nil {
		return nil, fmt.Errorf("artifact gas metadata: %w", err)
	}
	return EncodeArtifact(&Artifact{
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
	})
}

// contractNeedsInitCode returns true when the contract has a constructor or
// storage slot initializers, meaning an init_code artifact must be generated.
func contractNeedsInitCode(c *tolast.ContractDecl) bool {
	if c.Constructor != nil {
		return true
	}
	if c.Storage != nil {
		for _, slot := range c.Storage.Slots {
			if slot.InitExpr != nil {
				return true
			}
		}
	}
	return false
}

func optsOrDefault(opts *PackageOptions) PackageOptions {
	if opts == nil {
		return PackageOptions{}
	}
	return *opts
}

func torIncludeSourceMap(opts *PackageOptions) bool {
	if opts == nil || opts.IncludeSourceMap == nil {
		return true
	}
	return *opts.IncludeSourceMap
}

// torSigningPayload returns the bytes that are signed/verified for a .tor package.
// It is keccak256(manifestJSON_without_signature || sorted file contents).
// manifestJSON must not contain the "signature" field.
func torSigningPayload(manifestJSON []byte, files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf []byte
	buf = append(buf, manifestJSON...)
	for _, n := range names {
		buf = append(buf, files[n]...)
	}
	return keccak256Bytes(buf)
}

// torManifestStripSignature returns a copy of the manifest JSON with the
// "signature" key removed, used as canonical input to the signing hash.
func torManifestStripSignature(manifestJSON []byte) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return nil, err
	}
	delete(m, "signature")
	return json.Marshal(m)
}

// SignPackage adds an ed25519 publisher signature to an existing .tor archive.
// privKeySeed must be a 32-byte ed25519 seed (hex-encoded or raw bytes).
// Returns the new .tor bytes with publisher_key and signature embedded in manifest.json.
func SignPackage(pkgBytes []byte, privKeySeed []byte) ([]byte, error) {
	if len(privKeySeed) != ed25519.SeedSize {
		return nil, fmt.Errorf("package sign: private key must be %d bytes, got %d", ed25519.SeedSize, len(privKeySeed))
	}
	pkg, err := DecodePackage(pkgBytes)
	if err != nil {
		return nil, fmt.Errorf("package sign: decode: %w", err)
	}
	return signPackageFiles(pkg.ManifestJSON, pkg.Files, privKeySeed)
}

// signPackageFiles is the internal helper used by both SignPackage and CompilePackage.
func signPackageFiles(manifestJSON []byte, files map[string][]byte, privKeySeed []byte) ([]byte, error) {
	privKey := ed25519.NewKeyFromSeed(privKeySeed)
	pubKey := privKey.Public().(ed25519.PublicKey)

	manifestNoSig, err := torManifestStripSignature(manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("tor sign: strip signature: %w", err)
	}

	payload := torSigningPayload(manifestNoSig, files)
	sig := ed25519.Sign(privKey, payload)

	// Inject publisher_key and signature into manifest.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(manifestNoSig, &m); err != nil {
		return nil, err
	}
	pubKeyJSON, _ := json.Marshal(hex.EncodeToString(pubKey))
	sigJSON, _ := json.Marshal(hex.EncodeToString(sig))
	m["publisher_key"] = json.RawMessage(pubKeyJSON)
	m["signature"] = json.RawMessage(sigJSON)
	signedManifest, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return encodePackageZip(signedManifest, files)
}

// VerifyPackageSignature verifies the ed25519 publisher signature embedded in a .tor archive.
// Returns nil if the package is unsigned (no signature field) — unsigned packages are accepted.
// Returns an error if a signature is present but invalid or the public key is malformed.
func VerifyPackageSignature(pkgBytes []byte) error {
	pkg, err := DecodePackage(pkgBytes)
	if err != nil {
		return fmt.Errorf("package verify: decode: %w", err)
	}
	return verifyManifestSignature(pkg.ManifestJSON, pkg.Files)
}

// verifyManifestSignature checks the signature embedded in manifestJSON, if any.
func verifyManifestSignature(manifestJSON []byte, files map[string][]byte) error {
	var m struct {
		PublisherKey string `json:"publisher_key"`
		Signature    string `json:"signature"`
	}
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return nil // no fields → unsigned
	}
	if m.Signature == "" {
		return nil // unsigned — accepted
	}
	if m.PublisherKey == "" {
		return fmt.Errorf("tor verify: signature present but publisher_key missing")
	}
	pubKeyBytes, err := hex.DecodeString(m.PublisherKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("tor verify: invalid publisher_key")
	}
	sigBytes, err := hex.DecodeString(m.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("tor verify: invalid signature encoding")
	}
	manifestNoSig, err := torManifestStripSignature(manifestJSON)
	if err != nil {
		return fmt.Errorf("tor verify: strip signature: %w", err)
	}
	payload := torSigningPayload(manifestNoSig, files)
	if !ed25519.Verify(ed25519.PublicKey(pubKeyBytes), payload, sigBytes) {
		return fmt.Errorf("tor verify: signature verification failed")
	}
	return nil
}

// encodePackageZip builds the ZIP without running validatePackageManifest (used internally
// after signing when the manifest is already known-valid).
func encodePackageZip(manifestJSON []byte, files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writePackageEntry(zw, torManifestPath, manifestJSON); err != nil {
		return nil, err
	}
	for _, name := range names {
		if err := writePackageEntry(zw, name, files[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodePackage serializes manifest + files into deterministic .tor bytes.
func EncodePackage(manifestJSON []byte, files map[string][]byte) ([]byte, error) {
	cleanFiles := map[string][]byte{}
	for name, body := range files {
		clean, err := normalizePackagePath(name)
		if err != nil {
			return nil, err
		}
		if clean == torManifestPath {
			return nil, fmt.Errorf("tor files must not override %q", torManifestPath)
		}
		b := make([]byte, len(body))
		copy(b, body)
		cleanFiles[clean] = b
	}
	if err := validatePackageManifest(manifestJSON, cleanFiles, true); err != nil {
		return nil, err
	}

	var names []string
	for name := range cleanFiles {
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if err := writePackageEntry(zw, torManifestPath, manifestJSON); err != nil {
		return nil, err
	}
	for _, name := range names {
		if err := writePackageEntry(zw, name, cleanFiles[name]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// maxTorEntryBytes is the decompressed size limit for a single .tor ZIP entry.
// Prevents ZIP-bomb attacks where a tiny compressed file expands to gigabytes.
const maxTorEntryBytes = 4 << 20 // 4 MiB per entry

// maxTorTotalBytes is the total decompressed size limit for all entries in a .tor archive.
const maxTorTotalBytes = 16 << 20 // 16 MiB total

// DecodePackage deserializes .tor bytes and validates manifest/files.
func DecodePackage(data []byte) (*Package, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid tor zip: %w", err)
	}

	seen := map[string]struct{}{}
	var manifest []byte
	files := map[string][]byte{}
	var totalBytes int64 // SEC-3: track total decompressed size

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name, err := normalizePackagePath(f.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate tor entry: %s", name)
		}
		seen[name] = struct{}{}

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		// SEC-3: limit decompressed size per entry to prevent ZIP-bomb DoS.
		limited := io.LimitReader(rc, maxTorEntryBytes+1)
		body, err := io.ReadAll(limited)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if int64(len(body)) > maxTorEntryBytes {
			return nil, fmt.Errorf("tor entry %q exceeds decompressed size limit (%d bytes)", name, maxTorEntryBytes)
		}
		totalBytes += int64(len(body))
		if totalBytes > maxTorTotalBytes {
			return nil, fmt.Errorf("tor package total decompressed size exceeds limit (%d bytes)", maxTorTotalBytes)
		}
		if name == torManifestPath {
			manifest = body
			continue
		}
		files[name] = body
	}

	if len(manifest) == 0 {
		return nil, fmt.Errorf("tor manifest.json not found")
	}
	if err := validatePackageManifest(manifest, files, true); err != nil {
		return nil, err
	}
	for name, body := range files {
		if strings.HasSuffix(strings.ToLower(name), ".toc") {
			if _, err := DecodeArtifact(body); err != nil {
				return nil, fmt.Errorf("invalid .toc entry %q: %w", name, err)
			}
		}
		if strings.HasSuffix(strings.ToLower(name), ".abi") {
			if err := ValidateInterface(body); err != nil {
				return nil, fmt.Errorf("invalid .abi entry %q: %w", name, err)
			}
		}
	}
	return &Package{
		ManifestJSON: manifest,
		Files:        files,
	}, nil
}

// computeDispatchTag returns the 4-byte dispatch tag for a contract name.
// tag = keccak256("pkg:" + name)[0:4]
func computeDispatchTag(name string) [4]byte {
	h := keccak256("pkg:" + name)
	var tag [4]byte
	copy(tag[:], h[:4])
	return tag
}

func validatePackageManifest(manifestJSON []byte, files map[string][]byte, verifyRefs bool) error {
	if !json.Valid(manifestJSON) {
		return fmt.Errorf("tor manifest is not valid json")
	}
	var m struct {
		Name      string `json:"name"`
		Version   string `json:"version"`
		Contracts []struct {
			Name string `json:"name"`
			Artifact  string `json:"toc"`
			Interface string `json:"abi"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(manifestJSON, &m); err != nil {
		return fmt.Errorf("tor manifest decode error: %w", err)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("tor manifest requires non-empty 'name'")
	}
	// Security: reject .tor archives that claim the reserved platform namespace.
	pkgField := strings.TrimSpace(m.Name)
	if pkgField == sema.TolLangPackage || strings.HasPrefix(pkgField, sema.TolLangPackage+".") {
		return fmt.Errorf("[%s] package name %q is reserved for the TOS platform; external .tor archives may not use it", diag.CodeSemaTolLangReserved, pkgField)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("tor manifest requires non-empty 'version'")
	}
	// Verify ed25519 signature if present (unsigned packages are accepted).
	if err := verifyManifestSignature(manifestJSON, files); err != nil {
		return err
	}

	if verifyRefs {
		seenContracts := map[string]struct{}{}
		tagSeen := map[[4]byte]string{}
		for _, c := range m.Contracts {
			cname := strings.TrimSpace(c.Name)
			if cname == "" {
				return fmt.Errorf("tor manifest contracts entry requires non-empty 'name'")
			}
			if _, exists := seenContracts[cname]; exists {
				return fmt.Errorf("tor manifest has duplicate contract name %q", cname)
			}
			seenContracts[cname] = struct{}{}
			// Check for dispatch tag (keccak256("pkg:Name")[:4]) collisions.
			tag := computeDispatchTag(cname)
			if prev, exists := tagSeen[tag]; exists {
				return fmt.Errorf("dispatch tag collision between contracts %q and %q (both hash to %x); rename one contract", prev, cname, tag)
			}
			tagSeen[tag] = cname
			hasArtifact := strings.TrimSpace(c.Artifact) != ""
			hasInterface := strings.TrimSpace(c.Interface) != ""
			if !hasArtifact && !hasInterface {
				return fmt.Errorf("tor manifest contract %q must declare at least one of 'toc' or 'abi'", cname)
			}
			if p := strings.TrimSpace(c.Artifact); p != "" {
				np, err := normalizePackagePath(p)
				if err != nil {
					return fmt.Errorf("tor manifest contract %q has invalid toc path %q: %w", cname, p, err)
				}
				if _, ok := files[np]; !ok {
					return fmt.Errorf("tor manifest contract %q references missing toc file %q", cname, np)
				}
			}
			if p := strings.TrimSpace(c.Interface); p != "" {
				np, err := normalizePackagePath(p)
				if err != nil {
					return fmt.Errorf("tor manifest contract %q has invalid abi path %q: %w", cname, p, err)
				}
				if _, ok := files[np]; !ok {
					return fmt.Errorf("tor manifest contract %q references missing abi file %q", cname, np)
				}
			}
		}
	}
	return nil
}

func writePackageEntry(zw *zip.Writer, name string, body []byte) error {
	hdr := &zip.FileHeader{
		Name:   name,
		Method: zip.Store,
	}
	hdr.SetModTime(torDeterministicModTime)
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return nil
	}
	_, err = w.Write(body)
	return err
}

func normalizePackagePath(p string) (string, error) {
	name := strings.TrimSpace(p)
	if name == "" {
		return "", fmt.Errorf("tor entry path is empty")
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return "", fmt.Errorf("tor entry path must be relative: %q", p)
	}
	name = strings.ReplaceAll(name, "\\", "/")
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("tor entry path escapes archive root: %q", p)
	}
	if strings.Contains(clean, "/../") {
		return "", fmt.Errorf("tor entry path escapes archive root: %q", p)
	}
	return clean, nil
}
