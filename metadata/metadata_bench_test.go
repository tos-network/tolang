package metadata

import (
	"testing"
)

// benchABI is a realistic ABI JSON with multiple functions, events, and manifest.
const benchABI = `{
  "gas_model": {
    "version": "tolang/0.2.0",
    "sload": 2100,
    "sstore": 20000,
    "log_base": 375
  },
  "functions": [
    {
      "name": "transfer",
      "visibility": "external",
      "selector": "0xa9059cbb",
      "params": ["address", "uint256"],
      "returns": ["bool"],
      "doc": {
        "effects": {
          "reads": ["balances"],
          "writes": ["balances", "allowances"],
          "emits": ["Transfer"],
          "calls": [{"cap": "token", "iface": "ITRC20", "selector": "0x12345678", "max_gas": 50000}]
        },
        "gas_upper": 65000
      },
      "requires_capability": "token_send",
      "pay_amount_tomi": "1000000",
      "verifiable": true,
      "delegated": false
    },
    {
      "name": "approve",
      "visibility": "external",
      "selector": "0x095ea7b3",
      "params": ["address", "uint256"],
      "returns": ["bool"],
      "doc": {
        "effects": {
          "reads": ["allowances"],
          "writes": ["allowances"],
          "emits": ["Approval"]
        },
        "gas_upper": 45000
      },
      "requires_capability": "token_send",
      "verifiable": false,
      "delegated": true
    },
    {
      "name": "balanceOf",
      "visibility": "public",
      "selector": "0x70a08231",
      "params": ["address"],
      "returns": ["uint256"],
      "doc": {
        "effects": {
          "reads": ["balances"]
        },
        "gas_upper": 2100
      },
      "verifiable": false,
      "delegated": false
    },
    {
      "name": "totalSupply",
      "visibility": "public",
      "selector": "0x18160ddd",
      "params": [],
      "returns": ["uint256"],
      "doc": {
        "effects": {
          "reads": ["total_supply"]
        },
        "gas_upper": 2100
      },
      "verifiable": false,
      "delegated": false
    }
  ],
  "events": [
    {
      "name": "Transfer",
      "params": ["address", "address", "uint256"]
    },
    {
      "name": "Approval",
      "params": ["address", "address", "uint256"]
    }
  ],
  "manifest": {
    "name": "BenchToken",
    "version": "2.0.0",
    "description": "A benchmark token contract",
    "tags": ["token", "trc-20"],
    "extra": {
      "spec": "TRC-20",
      "sla_uptime": "99.9%"
    }
  },
  "account_contract": false
}`

func benchContractMetadata(b *testing.B) *ContractMetadata {
	b.Helper()
	meta, err := ExtractFromABI([]byte(benchABI))
	if err != nil {
		b.Fatalf("ExtractFromABI setup failed: %v", err)
	}
	meta.Contract.Name = "BenchToken"
	return meta
}

func BenchmarkExtractFromABI(b *testing.B) {
	abiBytes := []byte(benchABI)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ExtractFromABI(abiBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateHumanReadable(b *testing.B) {
	meta := benchContractMetadata(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GenerateHumanReadable(meta)
	}
}

func BenchmarkBuildDiscoveryManifest(b *testing.B) {
	meta := benchContractMetadata(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildDiscoveryManifest(meta, "bench-token")
	}
}

func BenchmarkComputeArtifactRef(b *testing.B) {
	pkgData := []byte("realistic-package-data-that-is-longer-than-trivial-input-bytes")
	bytecode := []byte("realistic-bytecode-data-representing-compiled-contract-output")
	source := []byte("realistic-source-code-data-representing-tol-contract-source")
	abiData := []byte(benchABI)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ComputeArtifactRef(pkgData, bytecode, source, abiData, "2.0.0")
	}
}
