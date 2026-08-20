"""用 ogen 把契约编译成服务端骨架。

# 为什么换掉 oapi-codegen

**oapi-codegen 只把 schema 翻成 Go 类型，不生成一行校验代码。** 契约上写着 `minLength: 1` 的
字段生成出来是 `Name string`，那条约束整个丢掉；`default: ssd` 同理。请求体上 292 个带约束的
字段和 38 个带默认值的字段，一个都不执行——而不执行**不报错**：空名字一路走到数据库层，报出来
的是一句 ent 的 validator failed 包成的 500，而那次调用早已在后端建好了资源。

补这一层试过两条路，都不成立：

	自己生成校验代码    写了一版，一次 review 就找出四个缺口——只管顶层请求体（query/path/header
	                   不查）、不递归进嵌套对象、required 和 additionalProperties 识别了却没生成
	                   判断、长度用 len() 算字节而 OpenAPI 按字符算。四个都是「漏了不报错」，
	                   而它们出现在一个刚写完的生成器上：自维护校验器是持续的负债。
	运行时对照契约      kin-openapi + oapi-codegen 的官方中间件。它在 3.0 上是完整的，但我们的
	                   契约是 3.1.0——实测违规明细里 field 和 rule 全是空的（结构化字段只在 3.0
	                   那条路径上填），default 也不回写。同一套代码在两个版本上行为不同，而两边
	                   都不报错。

ogen 把校验规则**编译进代码**，所以和 spec 版本的运行时支持无关。实测拿真实的 compute 契约：

	body 空名字     400  name (string: len 0 less than minimum 1)
	query 超长      400  query "address": string: len 100 greater than maximum 64
	query 越界      400  query "limit": int: value 999 greater than maximum 200
	query 默认值    200  limit 被填成 50
	多余字段        400  decode 阶段就拒

四个缺口全覆盖，长度按 Unicode 码点算（validate.String 里是 `len([]rune(v))`）。

# 它还把认证变成了编译期的事

契约里的 `security` 生成出一个 `SecurityHandler` 接口，不实现就编译不过。而上一版是「记得挂
中间件」——漏挂的表现是那个面对全世界开着，不报错、不 500、不出现在任何日志里。

# 代价：共用的 Error 类型被内联了

ogen 没有 import-mapping 这类机制，外部 `$ref` 一律内联。所以每份契约各得一个 Error 类型，而
不是全平台共用一个 Go 类型。

这件事**在换之前就已经是现状**：kit/oapiserver 写错误响应时手写的是一个 map，注释里写着「那个
类型每个生成包各有一份，kit 引哪一个都是错的」。没有任何一行手写代码 import 过它。

共用 Error 真正的价值在**契约层**——一份定义、CI 盯着公开和私有两份逐字一致——那部分不受影响。

# 服务器仍然是标准库的

ogen 生成的 Server 就是一个 http.Handler：路由、解码、校验、编码、认证入口都在里面，而监听、
TLS、超时、优雅退出仍归 http.Server。所以 kit/oapiserver 那一套原样能用，换的只是挂上去的那个
handler。
"""

import pathlib
import subprocess
import sys
import tempfile

import yaml


# 钉死版本，不用 latest：换一版会改字段名和可选性，而那种改动在服务仓库的 diff 里看起来和
# 「契约改了」一模一样，一次 review 分不出谁改了 API、谁升了工具。
OGEN = "github.com/ogen-go/ogen/cmd/ogen@v1.24.0"

# allow_remote 要开：共用类型是一个外部 $ref（../../type/v1/error.yaml）。不开的话生成期报
# 「external references are disabled」。
CONFIG = {"parser": {"allow_remote": True}}


def generate(spec: pathlib.Path, package: str, output: pathlib.Path) -> None:
    """把一份契约生成到一个 Go 包里。"""
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile("w", suffix=".yml", delete=False) as config:
        yaml.safe_dump(CONFIG, config)
        config_path = config.name

    try:
        subprocess.run(
            ["go", "run", OGEN,
             "-config", config_path,
             "-target", str(output),
             "-package", package,
             # clean：覆盖式生成留得下垃圾——改一个 schema 的名字之后旧那份代码没人删，而它照样
             # 编译、照样被 go get 拉走。
             "-clean",
             str(spec)],
            check=True, capture_output=True, text=True,
            env={**__import__("os").environ, "GOFLAGS": "-mod=mod"})
    except subprocess.CalledProcessError as failure:
        print(failure.stderr, file=sys.stderr)
        sys.exit(f"{output} 生成失败")
    finally:
        pathlib.Path(config_path).unlink(missing_ok=True)
