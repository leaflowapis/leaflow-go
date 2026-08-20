#!/usr/bin/env python3
"""把契约编译成 Go：一个客户端包，一个服务端包。

    openapi/<服务>/<版本>/openapi.yaml
        → go/<服务>/<版本>/client.gen.go          package <服务><版本>
        → go/<服务>/<版本>/server/server.gen.go   package <服务><版本>server

一个服务一个契约文件。它一度按 OpenAPI 的节点类型拆过（paths/ 加 schemas/，一个 schema 一个
文件），compute 拆出 74 个三十来行的文件——形式上模块化，实际是把「改一个接口」变成在十几个
文件之间跳。哪天某个服务真的长到难受了再单独给它拆。

# 客户端和服务端出自同一份契约，也在同一个 Go 模块里

因为它们必须来自同一版契约。分成两个模块两个 tag 的话，服务端可以钉 v1.4、客户端钉 v1.2，
而那正是这套东西要消除的漂移——一个 tag 管住两边，就不存在「钉得不一致」这回事。

分成两个**包**是因为它们的内容真的不同（一个发请求一个收请求），而且 Go 不编译没被 import
的包：外部用户只 import <服务>/<版本>，服务端那套一个字节都不会进他的二进制。

# 身份不在契约里，也不在生成的代码里

这里一度往服务端那份副本里注入 X-Leaflow-User-Id 之类的头，让身份变成 handler 的必填参数。
那套被换掉了，因为**头没有签名**：谁能连上服务的端口，谁就能自己写一个，而挡住伪造只能靠
「流量一定经过 waypoint」——那依赖 Service 上两个标签，少一个就漏一半，且漏了不报错。

现在身份走 kit/auth：请求上那张 IAM 签发的 access token 一路带到服务，服务自己验签，
中间件把它变成 Principal 放进 context。生成的 handler 因此完全不知道身份这回事，而
`auth.FromContext(ctx)` 在漏挂中间件时 panic——那条「handler 拿不到空身份」的保证还在，
只是换了执行者。

顺带解决了另一件事：邮箱和姓名曾经也是头。它们是**用户资料**不是安全主体，会变，而一个会变的
东西被复制进每一次请求就有两处不一致的时刻。要显示名字去 IAM 的 ConnectRPC 拿一次。
"""
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile

import yaml

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import contracts  # noqa: E402

ROOT = pathlib.Path(__file__).resolve().parent.parent

# 契约由 scripts/contracts.py 拉到本地，版本记在 CONTRACTS_REF 里。
#
# 语言仓库是契约的**产物**，产物不该反过来持有源的一个 git 指针——那正是 submodule 干的事，
# 而它换来四类只在 CI 上出现的失败，报出的都不是「submodule 配置有误」。
#
# leaflow/ 是命名空间层，与 googleapis 的 google/ 对应。
CONTRACTS_REMOTE = "https://github.com/leaflowapis/leaflowapis.git"
CONTRACTS_ROOT = ROOT / "leaflowapis"
CONTRACTS = CONTRACTS_ROOT / "leaflow"

# 生成器版本钉死，不用 latest：换一版会改字段名和可选性，而那种改动在服务仓库的 diff 里看起来
# 和「契约改了」一模一样，一次 review 分不出谁改了 API、谁升了工具。
CODEGEN = "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0"

# 配置文件里的键名和命令行 flag 不一样：-generate types 在配置里叫 models。写错的话它不报错，
# 只是那一项没生成——而「少了一半类型」要等到调用方编译不过才发现。
#
# strict-server 给的是 (ctx, request) (response, error)——和 huma 那套形状一致，所以迁移时
# handler 里那层业务代码几乎不用动。std-http-server 是把它接到标准库 ServeMux 上的胶水：
# Go 1.22 之后 http.ServeMux 自己就认 method 和路径参数，不需要第三方路由。
SERVER_GENERATE = {"models": True, "strict-server": True, "std-http-server": True}
CLIENT_GENERATE = {"models": True, "client": True}
# 共用类型那份只出类型：它没有 paths，生成 client 会得到一个空壳。
SHARED_GENERATE = {"models": True}

# skip-prune 只给共用类型那份开。
#
# 生成器默认裁掉「没被任何接口引用」的 component，而这份文件里的 Error 恰恰谁都不引用——它存在
# 就是为了被别的契约 $ref。不开的话生成出来是一个**只有 package 一行的空文件**，而且不报错：
# 报错的是几百公里外那七个服务包，一句 undefined: externalRef0.Error。
#
# 服务那边保持裁剪：那边的未引用 schema 是真的没人要。
SHARED_OUTPUT_OPTIONS = {**{"skip-prune": True}}

# SHARED 是那份共用类型在契约里的相对路径，以及它生成出来的 Go 包。
#
# 生成器对外部 $ref **拒绝内联**，要求你说清它对应哪个 Go 包（--import-mapping）。那比内联好：
# 内联的话十三份契约会得到十三个长得一样的 Go 类型，调用方拿 compute 的 Error 去喂一个收 iam
# Error 的函数会编译不过——而它们本来就是同一个东西。
SHARED_SPEC = "type/v1/error.yaml"
SHARED_PACKAGE = "github.com/leaflowapis/leaflow-go/type/v1"

# prefer-skip-optional-pointer-on-container-types
#   可选的数组和 map 生成成值，不是指针。`*[]string` 除了逼每个调用点写一次解引用之外没有任何
#   信息量——一个 nil 切片和一个「没传的切片」在这套 API 里是同一件事。
#
#   **不开全局那个 prefer-skip-optional-pointer**：标量必须保持指针，PATCH 靠它区分「这个字段
#   不动」和「把它改成空」。开了的话，一次只改名字的请求会把描述一起清掉，而且不报错。
OUTPUT_OPTIONS = {"prefer-skip-optional-pointer-on-container-types": True}


def codegen(spec, package, output, generate, scratch, mapping=None, options=None):
    output.parent.mkdir(parents=True, exist_ok=True)
    config = scratch / f"{package}-{len(generate)}.codegen.yaml"
    document = {
        "package": package,
        "generate": generate,
        "output": str(output),
        "output-options": {**OUTPUT_OPTIONS, **(options or {})},
    }
    if mapping:
        document["import-mapping"] = mapping
    with open(config, "w", encoding="utf-8") as fh:
        yaml.dump(document, fh)
    try:
        subprocess.run(
            ["go", "run", CODEGEN, "-config", str(config), str(spec)],
            check=True, capture_output=True, text=True,
            env={**os.environ, "GOFLAGS": "-mod=mod"})
    except subprocess.CalledProcessError as failure:
        print(failure.stderr, file=sys.stderr)
        sys.exit(f"{output} 生成失败")


# GO_MOD 是每个服务那份 go.mod 的模板。
#
# **一个服务一个模块**，不是整个仓库一个：装 compute 的人不该被拖进 monitoring 的依赖，而改
# compute 的契约也不该 bump iam 的版本号——版本号是外部用户看得见的东西，无关的跳动会让他们
# 以为自己漏了什么。google-cloud-go 是同一个形状（compute/go.mod = cloud.google.com/go/compute）。
#
# 代价是 tag 要带服务名前缀（iam/v0.1.0），忘了打前缀的表现是「发了但 go get 拿不到」。
GO_MOD = """module github.com/leaflowapis/leaflow-go/{service}

go 1.26.0

require github.com/oapi-codegen/runtime v1.7.0

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
)
"""


def write_module(service):
    """没有 go.mod 时给这个服务补一份。

    **只在缺的时候写，不覆盖。** 它一度是每次生成都重写的——理由是「它是产物不是手写文件」，
    而那在依赖固定的时候成立。现在不成立了：这些模块要 require 共用类型那个包，版本由
    `go get` / `go mod tidy` 维护，而覆盖会把那一行抹掉。

    抹掉之后的表现很绕：本地 `go build` 还是好的（go.sum 和 module cache 都在），
    **只有 CI 的「生成物是不是最新的」那一步会红**，报的是「契约改了但生成物没跟着更新」——
    一句完全指错方向的话，因为契约根本没改。

    所以模板只负责让一个新服务第一次生成时有个能编译的起点，之后归 go 的工具链管。
    """
    path = ROOT / service / "go.mod"
    if path.exists():
        return
    path.write_text(GO_MOD.format(service=service), encoding="utf-8")


def main():
    contracts.fetch(CONTRACTS_REMOTE, CONTRACTS_ROOT)
    specs = sorted(CONTRACTS.glob("*/*/openapi.yaml"))
    if not specs:
        sys.exit(f"{CONTRACTS} 下一份契约都没有")

    with tempfile.TemporaryDirectory(prefix="leaflow-go-") as raw:
        scratch = pathlib.Path(raw)

        # 共用类型先生成：各服务的包会 import 它。
        shared_out = ROOT / "type" / "v1"
        shutil.rmtree(shared_out, ignore_errors=True)
        codegen(CONTRACTS / SHARED_SPEC, "typev1", shared_out / "types.gen.go",
                SHARED_GENERATE, scratch, options=SHARED_OUTPUT_OPTIONS)
        write_module("type")
        print(f"{SHARED_SPEC:24} → type/v1")

        for contract in specs:
            version = contract.parent.name
            service = contract.parent.parent.name
            package = f"{service}{version}"
            out = ROOT / service / version

            # 生成前先删干净。覆盖式生成留得下垃圾：改一个 schema 的名字，旧那份代码没人删，
            # 而它照样编译、照样被 go get 拉走。现在这个 SDK 的 clean-generated.mjs 记着
            # v0.1.0 → v0.2.0 那次重命名留下了 584 个没人要的文件。
            shutil.rmtree(out, ignore_errors=True)

            # 相对路径按契约文件自己的位置算：<服务>/<版本>/openapi.yaml 到共用类型是 ../../
            mapping = {f"../../{SHARED_SPEC}": SHARED_PACKAGE}
            codegen(contract, package, out / "client.gen.go", CLIENT_GENERATE, scratch, mapping)
            codegen(contract, f"{package}server", out / "server" / "server.gen.go",
                    SERVER_GENERATE, scratch, mapping)

            write_module(service)
            print(f"{service}/{version:8} → {service}/{version}")


if __name__ == "__main__":
    main()
