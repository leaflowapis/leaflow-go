# leaflow-go

Leaflow 平台的 Go SDK，由正式 [leaflowapis](https://github.com/leaflowapis/leaflowapis) 契约生成。
每个服务一个模块，包括 billing/account、billing/catalog、billing/project；tag 使用各模块路径前缀。

原生 ogen v1.24.0 在 `<服务>/v1/server` 生成 Client、Handler、SecuritySource、引用模型与 Validate。
客户端与服务端使用同一份契约。使用方直接调用官方 NewClient，显式传入服务地址与准确生成的凭据源：

```go
import (
    "context"

    computev1 "github.com/leaflowapis/leaflow-go/compute/v1/server"
)

type tokenSource struct{ token string }

func (s tokenSource) ScopedTokenAuth(context.Context, computev1.OperationName) (computev1.ScopedTokenAuth, error) {
    return computev1.ScopedTokenAuth{Token: s.token}, nil
}

client, err := computev1.NewClient("https://compute.leaflow.cloud", tokenSource{token: token})
```

本次 breaking 升级移除旧 `v1` oapi 客户端、New/defaults 转发模板和 ClientWithResponses。
输入可选/可空状态使用原生 Opt/Nil；操作结果使用生成的类型化返回值，调用签名以生成接口为准。

## 重新生成

```
python3 scripts/generate.py
```

脚本仅编排官方 ogen 与模块路径，启用官方 client/request/validation，直接读取固定源，不改 schema、
不注入 x-ogen-type、不生成 fake operation 或额外锚模型、不手写客户端类型/校验适配器。
各 API 原生生成其实际引用的 schema/Validate，保留既有约束。模型 nominal 类型变化由消费者编译迁移。

默认从正式 remote 读取 CONTRACTS_REF。CONTRACTS_DIR 可用于候选演练，HEAD 必须精确匹配该 ref，且本地契约 checkout 必须干净。
OGEN_BIN 可指定已安装的官方 ogen v1.24.0；脚本核对模块版本。生成与编译使用外层 Go 工具链，
取消外部类型后不需要前一候选的 Go 1.26 loader 或本地共享模块代理。

## 移除 public Go type 模块

本候选移除 Go type 目录及各服务的共包依赖，不构造 GenerationRoot 或其他生成锚。
源共享 type 契约继续存在，各 API 通过原始引用生成自己的模型/Validate，TS 生成边界不在此处修改。

本 workspace 没有直接业务 Go import 或 Proto/RPC consumer 引用 public Go type module；Billing
仅有旧 SDK 带入的 indirect go.mod 项，此轮未修改业务服务依赖。该结论限于已核对的本地 workspace。

这是本次 breaking SDK 升级的公开类型边界变更。外部旧消费者若引用 public type package，升级到
新候选时应改用准确 API 包的生成类型；不提供兼容 alias/wrapper。旧发布 type tags 和 Git 历史不动，
固定旧版本的消费者不因此被改写。尚未核对仓库外的真实使用者，正式版本由父代理确定。

当前 CONTRACTS_REF 是本地候选 source commit，不表示已发布。正式采用前仍需从已发布 remote 来源
重生成并验证依赖，后续 Money 源约束修订由契约所有者推进；本轮不改契约或业务 wire。
