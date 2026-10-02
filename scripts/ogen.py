import contextlib
import json
import os
import pathlib
import re
import shutil
import subprocess
import tempfile

import yaml

OGEN = "github.com/ogen-go/ogen/cmd/ogen@v1.24.0"
SHARED_PACKAGE = "github.com/leaflowapis/leaflow-go/type/v1"


def generation_env():
    """固定外部类型加载器的官方工具链；生成期环境不写入发布模块。"""
    env = {**os.environ, "GOWORK": "off", "GOTOOLCHAIN": "local" if os.environ.get("OGEN_GO") else "go1.26.5"}
    go = os.environ.get("OGEN_GO", "go")
    version = subprocess.check_output([go, "version"], text=True, env=env)
    if "go1.26.5" not in version:
        raise ValueError("run generation with official Go 1.26.5, or set OGEN_GO to its go binary")
    if os.environ.get("OGEN_GO"):
        env["PATH"] = str(pathlib.Path(go).resolve().parent) + os.pathsep + env["PATH"]
    return env


@contextlib.contextmanager
def load_shared(module_root, local_shared=None):
    """从模块正式 pin 加载共同类型；公开仓库生成自身 type 时直接读取 type 模块。"""
    if local_shared is not None:
        yield local_shared
        return
    text = (module_root / "go.mod").read_text()
    match = re.search(r"github.com/leaflowapis/leaflow-go/type\s+(v\S+)", text)
    if not match:
        raise ValueError("go.mod must pin the native shared type module")
    with tempfile.TemporaryDirectory(prefix="ogen-type-loader-") as raw:
        cwd = pathlib.Path(raw)
        (cwd / "go.mod").write_text("module native.ogen.type.loader\n\ngo 1.26.0\n\nrequire github.com/leaflowapis/leaflow-go/type " + match.group(1) + "\n")
        (cwd / "loader.go").write_text('package loader\nimport _ "github.com/leaflowapis/leaflow-go/type/v1"\n')
        subprocess.run(["go", "mod", "tidy"], cwd=cwd, env=generation_env(), check=True)
        yield cwd


def run_ogen(spec, package, output, role, cwd):
    """服务包使用原生 client/server；共同 type 仅生成模型和校验。"""
    features = {"enable": ["client/request/validation"]}
    if role == "models":
        features = {"disable_all": True, "enable": ["client/request/validation"]}
    config = {"parser": {"allow_remote": True}, "generator": {"features": features}}
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="ogen-config-") as directory:
        path = pathlib.Path(directory) / "config.yaml"
        path.write_text(yaml.safe_dump(config), encoding="utf-8")
        binary = os.environ.get("OGEN_BIN")
        command = [binary] if binary else ["go", "run", OGEN]
        if binary:
            version = subprocess.check_output(["go", "version", "-m", binary], text=True)
            if "github.com/ogen-go/ogen\tv1.24.0" not in version:
                raise ValueError("OGEN_BIN must be the pinned v1.24.0 binary")
        env = generation_env()
        subprocess.run(command + ["-config", str(path), "-target", str(output), "-package", package, "-clean", str(spec)], cwd=cwd, env=env, check=True)


def generate_shared(contracts, output):
    """共享文档合并后仅生成原生模型；生成期根引用阻止 ogen 裁掉未引用 schema，不发布操作。"""
    schemas = {}
    documents = sorted((contracts / "leaflow/type/v1").glob("*.yaml"))
    names = {"./" + path.name for path in documents}

    def rewrite(value):
        if isinstance(value, list):
            return [rewrite(item) for item in value]
        if not isinstance(value, dict):
            return value
        result = {key: rewrite(item) for key, item in value.items()}
        reference = result.get("$ref")
        if isinstance(reference, str):
            target, separator, fragment = reference.partition("#")
            if separator and target in names:
                result["$ref"] = "#" + fragment
        return result

    for path in documents:
        document = yaml.safe_load(path.read_text())
        for name, value in document.get("components", {}).get("schemas", {}).items():
            if name in schemas:
                raise ValueError("duplicate shared schema " + name)
            schemas[name] = rewrite(value)
    anchors = {name: {"$ref": "#/components/schemas/" + name} for name in schemas}
    schemas["GenerationRoot"] = {"type": "object", "properties": anchors, "additionalProperties": False}
    document = {"openapi": "3.2.1", "info": {"title": "Shared native models", "version": "v1"}, "paths": {"/__generation_only": {"post": {"operationId": "generation-root", "requestBody": {"required": True, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/GenerationRoot"}}}}, "responses": {"204": {"description": "Generation-only root"}}}}}, "components": {"schemas": schemas}}
    with tempfile.TemporaryDirectory(prefix="ogen-shared-") as directory:
        path = pathlib.Path(directory) / "types.yaml"
        path.write_text(yaml.safe_dump(document, sort_keys=False))
        run_ogen(path, "typev1", output, "models", output.parent)
    (output / "types.gen.go").unlink(missing_ok=True)


def prepare_contracts(source, scratch):
    """原生 x-ogen-type 是生成期映射；正式契约不改，两个面均引用公开共同 package。"""
    shutil.copytree(source, scratch)
    for path in sorted((scratch / "leaflow/type/v1").glob("*.yaml")):
        document = yaml.safe_load(path.read_text())
        for name, schema in document.get("components", {}).get("schemas", {}).items():
            schema["x-ogen-type"] = SHARED_PACKAGE + "." + name
        path.write_text(yaml.safe_dump(document, sort_keys=False))


def generate_api(spec, output, package, generation_cwd, defaults=False):
    client_output = output
    output = output / "server"
    package = package + "server"
    run_ogen(spec, package, output, "both", generation_cwd)
    # 成功后移除旧生成客户端，保留手写测试和其他 WIP；ogen -clean 管理原生文件。
    for name in ("client.gen.go", "defaults.gen.go"):
        (client_output / name).unlink(missing_ok=True)
    if defaults:
        document = yaml.safe_load(spec.read_text())
        servers = document.get("servers") or []
        if servers and servers[0].get("url"):
            url = json.dumps(servers[0]["url"])
            secured = (output / "oas_security_gen.go").exists() and "type SecuritySource" in (output / "oas_security_gen.go").read_text()
            signature = "security SecuritySource, " if secured else ""
            argument = "security, " if secured else ""
            text = f'''// Code generated from the contract's servers[0]. DO NOT EDIT.
package {package}

// New 使用契约地址和原生 SecuritySource；不保留旧 ClientWithResponses 形状。
func New({signature}options ...ClientOption) (*Client, error) {{
    return NewClient({url}, {argument}options...)
}}
'''
            path = output / "defaults.gen.go"
            path.write_text(text)
            subprocess.run(["gofmt", "-w", str(path)], check=True)
