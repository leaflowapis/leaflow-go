"""把契约里的约束和默认值编译成 Go 代码。

# 为什么生成代码，而不是运行时拿契约对照

**oapi-codegen 只把 schema 翻成 Go 类型，不生成一行校验代码。** 契约上写着 `minLength: 1` 的
字段生成出来是 `Name string`，那条约束整个丢掉；`default: ssd` 同理，没传就是 nil，而契约上
声明的默认值从来没有生效过。

迁移当时全平台请求体上有 292 个带约束的字段和 38 个带默认值的字段，**一个都没有执行**。这件事
不报错：撞上库约束的成为 500（compute 有过一次 create-ipv6-hub 空名字，跑了 3.5 秒才失败——
它先在网络后端建好了路由器，才在写自己那一行时撞上），没撞上的静静写进去一个零值。

这里一度改成运行时拿契约本身对照（kin-openapi + oapi-codegen 的校验中间件）。那条路的好处是
不用自己实现任何关键字，但它把行为绑在了那个库对 spec 版本的支持上——而我们的契约是 3.1.0，
实测下来：违规明细里 `field` 和 `rule` 都是空的（结构化字段只在 3.0 路径上填），default 也不
回写。同一套代码在 3.0 上全对、3.1 上一半不对，而两边都不报错。

生成代码则没有这层依赖：

	错误形状     我们定，就是契约里那个共用的 Violation
	default      顺手就做了，不需要第二套机制
	出错时机     契约改了 → 生成 → **编译期**报，而不是运行时
	每请求代价    一串 if，不走 schema

# 「等于重新实现一个 JSON Schema 校验器」——量过了，不是

请求体上实际用到的关键字只有八个：

	additionalProperties  required  maxLength  minLength
	minimum  maximum  minItems  maxItems      加上 enum 和 default

**没有 pattern**（不用编译正则）、**没有 allOf / oneOf**（不用合并 schema）、没有 multipleOf、
没有 dependentRequired。契约里那 6 处 anyOf 全是 `[$ref, {type: null}]` 这种「可空」写法，而且
全在响应上——校验只管请求，碰不到它们。

这是一个封闭的小集合。遇到不认识的关键字**直接生成期报错**，不静默跳过：跳过的表现是那条约束
在服务端不执行，而契约上写着它。

# 类型上的约束不用生成

`format: uuid` 生成出来是 `openapi_types.UUID`，`enum` 生成出来是一个具名字符串类型——前者解不开
就是 400（解析阶段的事，不到这一层），后者……**oapi-codegen 不校验枚举值**，所以 enum 仍然要生成。
"""

import pathlib
import re


class Unsupported(Exception):
    """契约里有这个生成器不认识的东西。

    不降级成警告：一条被跳过的约束在服务端不执行，而契约上写着它——两边都看不出问题。
    """


# 认得的关键字。多出来的一律报错，见模块头。
KNOWN = {
    "type", "format", "description", "title", "properties", "required", "items",
    "additionalProperties", "nullable", "example", "examples", "deprecated",
    "readOnly", "writeOnly", "$ref", "allOf", "anyOf", "oneOf",
    # 下面这些是真的会生成代码的
    "minLength", "maxLength", "minimum", "maximum", "enum", "default",
    "minItems", "maxItems",
}


def _parse_structs(source: str) -> dict[str, dict[str, tuple[str, str]]]:
    """从生成的代码里读出每个结构体的 json 名 → (Go 字段名, Go 类型)。

    不自己推导命名：`max_ipv4_per_port` 变成 `MaxIpv4PerPort` 是 oapi-codegen 的规则，重新实现
    一遍就是第二份会漂的东西。对不上时报错，而不是生成一行编译不过的代码。
    """
    structs: dict[str, dict[str, tuple[str, str]]] = {}
    for match in re.finditer(r"^type (\w+) struct \{\n(.*?)^\}", source, re.S | re.M):
        fields: dict[str, tuple[str, str]] = {}
        for line in match.group(2).splitlines():
            field = re.match(r"\s+(\w+)\s+(\S+)\s+`json:\"([^\",]+)", line)
            if field:
                fields[field.group(3)] = (field.group(1), field.group(2))
        structs[match.group(1)] = fields
    return structs


def _request_objects(source: str) -> dict[str, str]:
    """请求对象 → 它的请求体类型名。

    两跳：`XxxRequestObject.Body *XxxJSONRequestBody`，而 `XxxJSONRequestBody = XxxRequestBody`。
    """
    aliases = dict(re.findall(r"^type (\w+JSONRequestBody) = (\w+)$", source, re.M))
    objects: dict[str, str] = {}
    for match in re.finditer(r"^type (\w+RequestObject) struct \{\n(.*?)^\}", source, re.S | re.M):
        # gofmt 会把字段对齐，所以 Body 和它的类型之间可能不止一个空格——带路径参数的请求对象
        # 就是这样（HubId 比 Body 长）。写死一个空格的话，那些接口静静地没有校验。
        field = re.search(r"\bBody\s+\*(\w+)", match.group(2))
        if field:
            objects[match.group(1)] = aliases.get(field.group(1), field.group(1))
    return objects


def _literal(value, go_type: str) -> str:
    """default 的 Go 字面量。

    一律带类型：`value := 32768` 推出来是 int，而字段多半是 *int64，赋值编译不过。
    """
    if isinstance(value, bool):
        return f"{go_type}({str(value).lower()})"
    if isinstance(value, (int, float)):
        return f"{go_type}({value})"
    if isinstance(value, str):
        return f'{go_type}("' + value.replace('"', '\\"') + '")'
    raise Unsupported(f"不认识的 default 值：{value!r}（{type(value).__name__}）")


def _checks(json_name: str, go_name: str, go_type: str, schema: dict, required: bool) -> list[str]:
    """一个字段的校验语句。"""
    unknown = set(schema) - KNOWN
    if unknown:
        raise Unsupported(f"{json_name}：不认识的关键字 {sorted(unknown)}")

    lines: list[str] = []
    pointer = go_type.startswith("*")
    # 可选字段先判 nil：没传就没什么可查的，那是「不过滤」不是「违规」。
    read = f"*request.Body.{go_name}" if pointer else f"request.Body.{go_name}"
    guard = f"\tif request.Body.{go_name} != nil {{\n" if pointer else ""
    indent = "\t\t" if pointer else "\t"

    body: list[str] = []
    if "minLength" in schema:
        body.append(
            f'{indent}if len({read}) < {schema["minLength"]} {{\n'
            f'{indent}\tfailed = append(failed, violation("{json_name}", "minLength"))\n'
            f'{indent}}}')
    if "maxLength" in schema:
        body.append(
            f'{indent}if len({read}) > {schema["maxLength"]} {{\n'
            f'{indent}\tfailed = append(failed, violation("{json_name}", "maxLength"))\n'
            f'{indent}}}')
    if "minimum" in schema:
        body.append(
            f'{indent}if {read} < {schema["minimum"]} {{\n'
            f'{indent}\tfailed = append(failed, violation("{json_name}", "minimum"))\n'
            f'{indent}}}')
    if "maximum" in schema:
        body.append(
            f'{indent}if {read} > {schema["maximum"]} {{\n'
            f'{indent}\tfailed = append(failed, violation("{json_name}", "maximum"))\n'
            f'{indent}}}')
    if "minItems" in schema:
        body.append(
            f'{indent}if len({read}) < {schema["minItems"]} {{\n'
            f'{indent}\tfailed = append(failed, violation("{json_name}", "minItems"))\n'
            f'{indent}}}')
    if "maxItems" in schema:
        body.append(
            f'{indent}if len({read}) > {schema["maxItems"]} {{\n'
            f'{indent}\tfailed = append(failed, violation("{json_name}", "maxItems"))\n'
            f'{indent}}}')
    if "enum" in schema and schema.get("type") == "string":
        # oapi-codegen 生成的是一个具名字符串类型，但**不校验取值**——任何字符串都赋得进去。
        allowed = ", ".join(f'"{one}"' for one in schema["enum"])
        base = go_type.lstrip("*")
        cast = f"string({read})" if base != "string" else read
        body.append(
            f'{indent}if !slices.Contains([]string{{{allowed}}}, {cast}) {{\n'
            f'{indent}\tfailed = append(failed, violation("{json_name}", "enum"))\n'
            f'{indent}}}')

    if not body:
        return []
    if pointer:
        lines.append(guard + "\n".join(body) + "\n\t}")
    else:
        lines.extend(body)
    return lines


def _defaults(json_name: str, go_name: str, go_type: str, schema: dict) -> str | None:
    """一个字段的默认值兑现。只对可选字段——required 的带 default 是契约自己写拧了。"""
    if "default" not in schema or not go_type.startswith("*"):
        return None
    literal = _literal(schema["default"], go_type[1:])
    return (f"\tif request.Body.{go_name} == nil {{\n"
            f"\t\tvalue := {literal}\n"
            f"\t\trequest.Body.{go_name} = &value\n"
            f"\t}}")


HEADER = '''// Code generated by scripts/validate.py. DO NOT EDIT.
//
// oapi-codegen 只把 schema 翻成 Go 类型，不生成校验代码：契约上写着 minLength: 1 的字段生成
// 出来是 `Name string`，约束整个丢掉。这些方法补上那一层，由 kit/oapiserver 统一调用。

package {package}

import ({imports}
	typev1 "{shared}"
)

// violation 是一条「请求和契约对不上」的明细。
//
// 类型来自**契约里的共用类型**（type/v1），不是这个包自己定义的——它要被 kit 读、被两个前端读，
// 而一个各服务各定义一遍的形状没人能可靠地处理。
func violation(field, rule string) typev1.Violation {{
	return typev1.Violation{{Field: field, Rule: rule}}
}}
'''


def render(package: str, source: str, spec: dict, shared: str) -> str | None:
    """生成 validate.gen.go。这份契约的请求体上一条约束都没有时返回 None。"""
    schemas = spec.get("components", {}).get("schemas") or {}
    structs = _parse_structs(source)
    objects = _request_objects(source)

    blocks = []
    for object_name in sorted(objects):
        body_type = objects[object_name]
        schema = schemas.get(body_type)
        if schema is None:
            continue
        fields = structs.get(body_type)
        if fields is None:
            raise Unsupported(f"{body_type} 在生成的代码里找不到")

        required = set(schema.get("required") or [])
        filled: list[str] = []
        checks: list[str] = []
        for json_name in sorted(schema.get("properties") or {}):
            property_schema = schema["properties"][json_name]
            if not isinstance(property_schema, dict):
                continue
            if json_name not in fields:
                raise Unsupported(
                    f"{body_type}.{json_name} 在生成的代码里找不到对应字段——"
                    f"契约和生成物对不上，先看 oapi-codegen 那一步")
            go_name, go_type = fields[json_name]
            default = _defaults(json_name, go_name, go_type, property_schema)
            if default:
                filled.append(default)
            checks.extend(_checks(json_name, go_name, go_type,
                                  property_schema, json_name in required))

        if not filled and not checks:
            continue

        # default 先补再查：补上来的值同样要满足约束，否则一个写错的默认值会静静通过。
        statements = "\n".join(filled + checks)
        blocks.append(
            f"// Validate 照契约查这个请求，并补上契约里声明的默认值。\n"
            f"//\n"
            f"// 值接收者够用：Body 是指针，改的是它指向的那个结构体。\n"
            f"func (request {object_name}) Validate() []typev1.Violation {{\n"
            f"\tif request.Body == nil {{\n"
            f"\t\treturn nil\n"
            f"\t}}\n"
            f"\tvar failed []typev1.Violation\n"
            f"{statements}\n"
            f"\treturn failed\n"
            f"}}")

    if not blocks:
        return None
    body = "\n\n".join(blocks)
    # slices 只有 enum 那一支用得到。无条件 import 的话，一份没有枚举的契约生成出来编译不过，
    # 报的是一句「imported and not used」——看不出和契约有关。
    imports = '\n\t"slices"\n' if "slices.Contains" in body else ""
    return HEADER.format(package=package, shared=shared, imports=imports) + "\n" + body + "\n"


def write(output: pathlib.Path, package: str, spec: dict, shared: str) -> bool:
    """给一份已经生成好的服务端代码补上 validate.gen.go。写了返回 True。"""
    target = output / "validate.gen.go"
    target.unlink(missing_ok=True)
    rendered = render(package, (output / "server.gen.go").read_text(encoding="utf-8"), spec, shared)
    if rendered is None:
        return False
    target.write_text(rendered, encoding="utf-8")
    return True
