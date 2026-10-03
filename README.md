# leaflow-go

Leaflow 平台的 Go SDK，由正式 [leaflowapis](https://github.com/leaflowapis/leaflowapis) 契约生成。
每个服务一个模块；tag 使用各模块路径前缀。Billing 统一为
`github.com/leaflowapis/leaflow-go/billing`，客户端与服务端包为 `billing/v1/server`。
旧 `billing/account`、`billing/catalog`、`billing/project` 三个模块和包已移除。

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

## Billing 迁移

账户与公开目录接口现在由同一个 `billingv1server.Client` / `Handler` 提供，共 63 个操作。
URL 统一使用 `/api/v1`；项目关联使用 `/api/v1/assignments/{projectId}`。
`SecuritySource` / `SecurityHandler` 仅包含 `AccessTokenAuth`：55 个金融操作使用 AccessToken
与账户权限，8 个公开目录读取不调用凭据源。16 个重复项目接口及 `CreateEstimate` 已移除。

报价只有 `CreateQuote(ctx, *QuoteRequest) (*Quote, error)`。`QuoteRequest.BillingAccountID` 为必填
`int64`，用于统一报价 items、renewals、order_id、cancellation 或 refund；目标的组合及账户归属
由服务判断。`QuoteItemInput.TerminationPolicy` 为 `OptTerminationPolicy`；
`DurationSeconds` 为 `OptInt64`，计量报价按 quantity 乘以秒数计算。

支出和持续计量记录使用 `ListSpend`、`ListActiveResources`，路径分别为 `/api/v1/spend`、
`/api/v1/active-resources`。`ListSpendParams.BillingAccountID` 必填，汇总使用同一账户币种；
其他金融列表的账户筛选可省略。`ProjectIds` 使用 `[]uuid.UUID`，最多 100 个唯一值，以
`project_ids=id1,id2` 传输，只有筛选作用。`ListUsageChargesParams.MeterID` 和
`ListActiveResourcesParams.MeterID` 保留计量指标筛选。

Credit Note 读取使用 `ListCreditNotes`、`GetCreditNote` 和 `ListCreditNoteItems`，均使用 AccessToken。
`Invoice` 与 `InvoiceSummary` 新增必填 `Money` 字段 `UnpaidCreditNotesAmount` 和
`PaidCreditNotesAmount`；后者记录退回义务，不代表退款已完成。
`CreditNote.VoidedAt` / `RefundID` 为必填可空的 `NilDateTime` / `NilUUID`。

`CancelOrder(ctx, OptOrderCancel, CancelOrderParams)` 的 JSON body 可省略；
`OptOrderCancel{}` 表示不发送 body，`NewOptOrderCancel(OrderCancel{})` 表示发送 `{}`。
`OrderCancel.OrderItemIds` 为可省略的 `[]uuid.UUID`，显式选择需 1–100 个唯一 ID。
`Order` 响应已移除 `CancelReason`。

原生可选数组解码把 `project_ids=` 当作省略。要应用项目筛选，请传非空项目列表；
账户授权范围不会因省略筛选而扩大。

`Quote` 和报价条目不再包含 `priced` / `unpriced_reason`，报价金额是必填的原生 `Money` 字段；
单一单价不适用时，`QuoteItem.UnitAmount` 仍为必填可空的 `NilMoney`。
`QuoteRequest.Refund` 和 `Quote.Refund` 分别使用 `OptOrderRefundQuoteInput` 和
`OptOrderRefundQuote`。列表直接使用源中的公共分页参数引用，默认每页 50、最多 200。
使用方需迁移到新包及生成签名；不提供旧模块别名或兼容转发。

固定源中的响应 `Money` 仅声明为字符串；原生解码检查字符串类型和必填性，但不会额外拒绝
指数或尾部空白。请求金额的 pattern 等约束按各自 schema 生成。

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

各服务 go.mod 仅依赖 ogen 与公共 Go 库，不依赖 public Go type module。
业务服务消费者的编译迁移由使用方处理。

这是本次 breaking SDK 升级的公开类型边界变更。外部旧消费者若引用 public type package，升级到
新候选时应改用准确 API 包的生成类型；不提供兼容 alias/wrapper。旧发布 type tags 和 Git 历史不动，
固定旧版本的消费者不因此被改写。尚未核对仓库外的真实使用者，正式版本由父代理确定。

当前 `CONTRACTS_REF` 固定为已推送的公开源
[`9dc43285d1e9043e779158f7098870f2623705a2`](https://github.com/leaflowapis/leaflowapis/commit/9dc43285d1e9043e779158f7098870f2623705a2)。
产物使用正式脚本、原生 ogen v1.24.0，从该远端提交的干净 checkout 生成。
这份 SDK 提交仍是发布候选；各改变模块的新 tag 由发布方决定。
