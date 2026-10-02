# leaflow-go

Leaflow 平台的 Go SDK,由 [leaflowapis](https://github.com/leaflowapis/leaflowapis) 生成。

```
go get github.com/leaflowapis/leaflow-go/compute
```

每个服务一个模块,tag 带服务名前缀(`compute/v0.2.0`)。按职能分出子包的服务,每个子包是一个模块,
tag 带两段前缀:

```
go get github.com/leaflowapis/leaflow-go/billing/catalog    # billing/catalog/v0.1.0
go get github.com/leaflowapis/leaflow-go/billing/account    # billing/account/v0.1.0
go get github.com/leaflowapis/leaflow-go/billing/project    # billing/project/v0.1.0
```

包名由路径各段连成，并加 `server` 后缀，如 `billingcatalogv1server`。
原生 ogen 客户端和服务端共同生成到 `<服务>/v1/server`，类型和方法以生成接口为准。

```go
import (
    "context"

    computev1 "github.com/leaflowapis/leaflow-go/compute/v1/server"
)

type tokenSource struct{ token string }

func (s tokenSource) ScopedTokenAuth(context.Context, computev1.OperationName) (computev1.ScopedTokenAuth, error) {
    return computev1.ScopedTokenAuth{Token: s.token}, nil
}

// New 使用契约 servers[0]；也可直接调用原生 NewClient 指定服务地址。
client, err := computev1.New(tokenSource{token: token})
```

本版本按批准的升级移除旧 `v1` oapi 客户端。使用方改为原生 `Client`、`Invoker`、
`SecuritySource`、`ClientOption` 和每个操作的类型化结果；不存在 `ClientWithResponses` 转发层。
共同类型位于 `github.com/leaflowapis/leaflow-go/type/v1`，包含原生 Encode/Decode、可选值和可空值。
调用 `encoding/json` 编码这些模型时传入模型指针，以调用原生 MarshalJSON 并保留省略字段。

## 重新生成

```
python3 scripts/generate.py
```

默认从正式远程仓库读取 `CONTRACTS_REF` 指定的契约。`CONTRACTS_DIR` 仅用于本地候选演练，
其 HEAD 必须精确匹配 `CONTRACTS_REF`。本地候选来源不表示已发布。

固定官方 ogen v1.24.0。OpenAPI 外部类型加载使用官方 Go 1.26.5；脚本默认通过 Go 标准工具链选择器
解析该版本，离线时可用 `OGEN_GO` 指定已经安装的官方 go 二进制。`OGEN_BIN` 可指定已安装的
ogen v1.24.0；脚本校验其版本。服务模块编译保留各自 go.mod 的工具链要求。

公开 type 模块先生成，其余 13 个服务加载同一公开共同 package。跨 SDK 使用方必须 pin 新原生
共享 type 的版本；旧 oapi type 没有原生 Encode/Decode。生成不会写入 replace 或 go.work。
正式发布需先发布共享 type，再更新并验证 13 个 public 模块和 private SDK 的共同版本 pin。
当前依赖 pin 指向未发布候选提交；本地模块产物验证不能替代正式远程重生成和依赖验证。

## 候选验收边界

ogen v1.24 的外部 `x-ogen-type` 解码会委托共同类型 Decode，但不会调用其 Validate。
当前候选映射下，外部 CheckoutOptions 的科学计数法、空串、超精度和 mode enum 约束没有在生成
请求链执行；直接对共同类型调用 Validate 通过的测试不能代表请求链拒绝。独立真实服务器红回归
已证明该问题，尚未修复。正式采用前必须解决外部嵌套校验；本候选不能作为生产验收通过。
