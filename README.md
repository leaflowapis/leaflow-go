# leaflow-go

Leaflow 平台的 Go SDK。**这个仓库里的 `.gen.go` 全是产物**,源在
[LeaflowNET/leaflowapis](https://github.com/LeaflowNET/leaflowapis)。

```go
import (
    computev1 "github.com/LeaflowNET/leaflow-go/compute/v1"
    iamv1     "github.com/LeaflowNET/leaflow-go/iam/v1"
)
```

## 一个服务一个模块

```
compute/go.mod   module github.com/LeaflowNET/leaflow-go/compute   tag compute/v0.1.0
iam/go.mod       module github.com/LeaflowNET/leaflow-go/iam       tag iam/v0.1.0
...
```

不是整个仓库一个模块,因为:装 compute 的人不该被拖进 monitoring 的依赖;而改 compute 的契约也不该
bump iam 的版本号——版本号是外部用户看得见的东西,无关的跳动会让他们以为自己漏了什么。
`google-cloud-go` 是同一个形状(`compute/go.mod` = `cloud.google.com/go/compute`)。

代价是 **tag 必须带服务名前缀**(`compute/v0.1.0`,不是 `v0.1.0`)。忘了前缀的表现是「发了但
`go get` 拿不到」,而没有任何东西会报错。

## 客户端和服务端在同一个模块里

```
<服务>/<版本>/client.gen.go          package <服务><版本>          调用方用
<服务>/<版本>/server/server.gen.go   package <服务><版本>server    服务端骨架
```

因为它们**必须来自同一版契约**。分成两个模块两个 tag 的话,服务端可以钉 v1.4、客户端钉 v1.2,而那
正是这套东西要消除的漂移。一个 tag 管住两边,就不存在「钉得不一致」这回事。

分成两个**包**是因为内容真的不同,而且 Go 不编译没被 import 的包:外部用户只 import
`<服务>/<版本>`,服务端那套一个字节都不会进他的二进制。

## 契约是一个 submodule

```
leaflowapis/    → LeaflowNET/leaflowapis,钉在某一个 commit 上
```

钉着而不是跟 main 走:那个指针同时回答了「这版 SDK 出自哪版契约」——它出现在 git diff 里,review
看得见,出事时答得出。跟 main 走的话,同一个 tag 在两台机器上能生成出不同的代码,而那件事不报错。

## 重新生成

```
git submodule update --remote leaflowapis    # 要跟进新契约时才做
python3 scripts/generate.py
```

**产物由人在本地跑完提交,CI 只验证不写回。** 一个会 commit 回来的流水线,在两次推送挨得近时要处理
「我算完了但 main 又变了」,而处理方式无非是重试或者强推——前者会打架,后者会丢东西。

Go 这边尤其不能靠发布时现生成:`go get` 直接读 tag 上的源码,**签进去的 `.gen.go` 就是消费者拿到的
东西**,没有哪一步会替它兜底。
