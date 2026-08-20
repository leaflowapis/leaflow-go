# leaflow-go

Leaflow 平台的 Go SDK,由 [leaflowapis](https://github.com/leaflowapis/leaflowapis) 生成。

```
go get github.com/leaflowapis/leaflow-go/compute
```

每个服务一个模块,tag 带服务名前缀(`compute/v0.2.0`)。

```go
import (
    "net/http"

    computev1 "github.com/leaflowapis/leaflow-go/compute/v1"
)

client, err := computev1.NewClientWithResponses(
    "https://compute.leaflow.cloud",
    computev1.WithRequestEditorFn(func(_ context.Context, request *http.Request) error {
        request.Header.Set("Authorization", "Bearer "+token)
        return nil
    }),
)
```

`<服务>/<版本>/server` 下是服务端骨架,仅服务实现方需要。

## 重新生成

```
python3 scripts/generate.py
```

契约版本记在 `CONTRACTS_REF`。
