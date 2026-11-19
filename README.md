# leaflow-go

Go SDK for the Leaflow platform, generated from the [leaflowapis](https://github.com/leaflowapis/leaflowapis) contracts.
Each service is a module, and tags use the module path prefix.

```sh
go get github.com/leaflowapis/leaflow-go/compute
```

```go
import computev2 "github.com/leaflowapis/leaflow-go/compute/v2/server"

client, err := computev2.NewClient("https://compute.leaflow.cloud", securitySource)
```

`securitySource` implements the generated `ScopedTokenAuth` interface. Optional and nullable inputs use native Opt/Nil.

## Regenerate

```sh
python3 scripts/generate.py
```

The contract version is recorded in `CONTRACTS_REF`. `CONTRACTS_DIR` may point to a clean checkout at that ref.
