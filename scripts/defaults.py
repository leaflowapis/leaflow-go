"""把契约里的 `default` 兑现成 Go 代码。

# 为什么需要这一步

**oapi-codegen 不实现 default。** 一个 `default: ssd` 的可选字段生成出来是 `Media *string`，
没传就是 nil，而 handler 那边 `Value(request.Body.Media)` 取到的是空字符串——契约上写着的那个
默认值在服务端从来没有生效过。

这件事**不报错**。它的表现取决于那个字段落到哪里：运气好的撞上一条数据库约束（compute 的
`media` 就是这样，一个 500 加一句 ent 的 validator failed）；运气不好的静静地写进去一个零值，
比如 `max_size_gb: 0` —— 一款怎么也扩不了容的硬盘类型，而创建它的那次请求返回 201。

上一代（huma）从 Go 类型导出契约，默认值本来就住在 Go 里，所以不存在这个缺口。方向反过来之后
它出现了，而且是**沉默地**出现的：迁移当时六个服务一共 37 个这样的字段，一个都没有兑现。

# 为什么不在 handler 里写 ValueOr(x, "ssd")

那是把契约里的默认值抄一份到 Go 里。两份会漂：改契约的人不会想到还有第二处，而漂了之后契约
文档说 ssd、服务端给 hdd，两边都不报错。

生成出来则只有一个源。

# 为什么挂在请求对象上，而不是让 handler 调

`ApplyDefaults` 由 kit/oapiserver 的中间件统一调用（见那边的 ApplyDefaults 中间件）。让 handler
自己调的话，加一个新接口忘了调是一次静默的回退——正是这个文件要消除的那类失败。

# 字段名不猜，从生成的代码里读

`max_ipv4_per_port` 变成 `MaxIpv4PerPort` 是 oapi-codegen 的命名规则，重新实现一遍就是第二份
会漂的东西。所以这里解析它已经生成好的那个结构体，按 json tag 反查字段名——对不上时直接报错，
而不是生成一行编译不过的代码。
"""

import pathlib
import re


# 请求体里 default 的 Go 字面量写法。契约的 YAML 值类型有限，逐个列出来比一个通用的转换稳妥：
# 漏掉一种是生成期报错，而一个"聪明"的转换会在某种类型上给出能编译但不对的字面量。
def _literal(value, go_type: str) -> str:
    if isinstance(value, bool):
        return f"{go_type}(true)" if value else f"{go_type}(false)"
    if isinstance(value, int):
        # 必须带类型：`value := 32768` 推出来是 int，而字段是 *int64，赋值编译不过。
        return f"{go_type}({value})"
    if isinstance(value, float):
        return f"{go_type}({value!r})"
    if isinstance(value, str):
        # 枚举字段生成的是一个具名类型（CreateDiskTypeRequestBodyMedia），同样要转。
        quoted = '"' + value.replace('"', '\\"') + '"'
        return f"{go_type}({quoted})"
    raise SystemExit(f"不认识的 default 值：{value!r}（{type(value).__name__}）")


def _parse_structs(source: str) -> dict[str, dict[str, tuple[str, str]]]:
    """从生成的代码里读出每个结构体的 json 名 → (Go 字段名, Go 类型)。"""
    structs: dict[str, dict[str, tuple[str, str]]] = {}
    for match in re.finditer(r"^type (\w+) struct \{\n(.*?)^\}", source, re.S | re.M):
        name, body = match.group(1), match.group(2)
        fields: dict[str, tuple[str, str]] = {}
        for line in body.splitlines():
            field = re.match(r"\s+(\w+)\s+(\S+)\s+`json:\"([^\",]+)", line)
            if field:
                fields[field.group(3)] = (field.group(1), field.group(2))
        structs[name] = fields
    return structs


def _request_objects(source: str) -> dict[str, str]:
    """请求对象 → 它的请求体类型名。

    两跳：`XxxRequestObject.Body *XxxJSONRequestBody`，而 `XxxJSONRequestBody = XxxRequestBody`。
    """
    aliases = dict(re.findall(r"^type (\w+JSONRequestBody) = (\w+)$", source, re.M))
    objects: dict[str, str] = {}
    for match in re.finditer(
        r"^type (\w+RequestObject) struct \{\n(.*?)^\}", source, re.S | re.M
    ):
        name, body = match.group(1), match.group(2)
        field = re.search(r"\s+Body \*(\w+)", body)
        if field:
            objects[name] = aliases.get(field.group(1), field.group(1))
    return objects


def collect(spec: dict) -> dict[str, dict]:
    """契约里每个带 default 的可选字段：请求体类型名 → {json 名: 默认值}。

    **只看可选字段。** 一个 required 的字段带 default 是契约自己写拧了（客户端必须传，那个默认
    值永远轮不到），补它只会掩盖那处矛盾。
    """
    wanted: dict[str, dict] = {}
    for name, schema in (spec.get("components", {}).get("schemas") or {}).items():
        required = set(schema.get("required") or [])
        defaults = {
            field: value["default"]
            for field, value in (schema.get("properties") or {}).items()
            if isinstance(value, dict) and "default" in value and field not in required
        }
        if defaults:
            wanted[name] = defaults
    return wanted


def render(package: str, source: str, defaults: dict[str, dict]) -> str | None:
    """生成 defaults.gen.go。这份契约一个默认值都没有时返回 None——不写一个空文件。"""
    if not defaults:
        return None

    structs = _parse_structs(source)
    objects = _request_objects(source)

    blocks = []
    for object_name in sorted(objects):
        body_type = objects[object_name]
        wanted = defaults.get(body_type)
        if not wanted:
            continue
        fields = structs.get(body_type)
        if fields is None:
            raise SystemExit(f"{body_type} 在生成的代码里找不到，无法兑现它的 default")

        assignments = []
        for json_name in sorted(wanted):
            if json_name not in fields:
                raise SystemExit(
                    f"{body_type}.{json_name} 在生成的代码里找不到对应字段——"
                    f"契约和生成物对不上，先看 oapi-codegen 那一步"
                )
            go_name, go_type = fields[json_name]
            if not go_type.startswith("*"):
                # 非指针意味着它在契约里是 required，而 collect 已经把那些排除了。走到这里说明
                # 两边对同一个字段的可选性判断不一致，那是要人看的，不是能兜住的。
                raise SystemExit(
                    f"{body_type}.{json_name} 生成的是 {go_type} 而不是指针，"
                    f"补默认值会覆盖调用方显式传的值"
                )
            literal = _literal(wanted[json_name], go_type[1:])
            assignments.append(
                f"\tif request.Body.{go_name} == nil {{\n"
                f"\t\tvalue := {literal}\n"
                f"\t\trequest.Body.{go_name} = &value\n"
                f"\t}}"
            )

        listed = "、".join(sorted(wanted))
        blocks.append(
            f"// ApplyDefaults 补上契约里为 {listed} 声明的默认值。\n"
            f"//\n"
            f"// 值接收者够用：Body 是指针，改的是它指向的那个结构体。\n"
            f"func (request {object_name}) ApplyDefaults() {{\n"
            f"\tif request.Body == nil {{\n"
            f"\t\treturn\n"
            f"\t}}\n" + "\n".join(assignments) + "\n}"
        )

    if not blocks:
        return None

    header = (
        "// Code generated by scripts/defaults.py. DO NOT EDIT.\n"
        "//\n"
        "// oapi-codegen 不实现契约里的 default，而那件事不报错——没传的字段在服务端拿到的是零值，\n"
        "// 契约上写着的默认值从来没有生效过。这些方法由 kit/oapiserver 的中间件统一调用。\n"
        "\n"
        f"package {package}\n"
    )
    return header + "\n" + "\n\n".join(blocks) + "\n"


def write(output: pathlib.Path, package: str, spec: dict) -> bool:
    """给一份已经生成好的服务端代码补上 defaults.gen.go。写了返回 True。"""
    server = output / "server.gen.go"
    rendered = render(package, server.read_text(encoding="utf-8"), collect(spec))
    target = output / "defaults.gen.go"
    target.unlink(missing_ok=True)
    if rendered is None:
        return False
    target.write_text(rendered, encoding="utf-8")
    return True
