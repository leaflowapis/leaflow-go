import os
import pathlib
import subprocess
import tempfile

import yaml

OGEN = "github.com/ogen-go/ogen/cmd/ogen@v1.24.0"


def generate_api(spec, output, package, cwd):
    """只编排官方 ogen；直接生成各 API 的引用模型、校验、客户端和服务端。"""
    target = output / "server"
    target.mkdir(parents=True, exist_ok=True)
    config = {
        "parser": {"allow_remote": True},
        "generator": {"features": {"enable": ["client/request/validation"]}},
    }
    with tempfile.TemporaryDirectory(prefix="ogen-config-") as directory:
        path = pathlib.Path(directory) / "config.yaml"
        path.write_text(yaml.safe_dump(config), encoding="utf-8")
        binary = os.environ.get("OGEN_BIN")
        command = [binary] if binary else ["go", "run", OGEN]
        if binary:
            version = subprocess.check_output(["go", "version", "-m", binary], text=True)
            if "github.com/ogen-go/ogen\tv1.24.0" not in version:
                raise ValueError("OGEN_BIN must be the pinned v1.24.0 binary")
        subprocess.run(
            command + ["-config", str(path), "-target", str(target), "-package", package + "server", "-clean", str(spec)],
            cwd=cwd, env={**os.environ, "GOWORK": "off", "GOTOOLCHAIN": "local"}, check=True,
        )
    # 成功后仅移除已废弃的生成客户端和手产 defaults，保留手写测试和其他 WIP。
    for path in (output / "client.gen.go", output / "defaults.gen.go", target / "defaults.gen.go"):
        path.unlink(missing_ok=True)
