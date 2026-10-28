#!/usr/bin/env python3
"""用 pinned 原生 ogen 在 server 包生成客户端和服务端。"""
import os
import pathlib
import subprocess
import contracts
import ogen

ROOT = pathlib.Path(__file__).resolve().parent.parent
BUF = "github.com/bufbuild/buf/cmd/buf@v1.75.0"


def require_source(source, ref):
    head = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
    if head != ref:
        raise ValueError("source HEAD differs from CONTRACTS_REF")
    # 拒绝同 HEAD 下的未提交契约漂移，避免把他人的 WIP 当成固定 source 生成。
    dirty = subprocess.check_output(["git", "-C", str(source), "status", "--porcelain", "--untracked-files=all"], text=True)
    if dirty:
        raise ValueError("CONTRACTS_DIR must be a clean checkout of CONTRACTS_REF")
    return head


def generate_services(source):
    # 直接读取固定契约，不改 scratch/schema，不注入外部类型或生成额外业务操作。
    for spec in sorted((source / "leaflow").glob("**/openapi.yaml")):
        relative = spec.parent.relative_to(source / "leaflow")
        # A Go package name has no hyphen while a contract directory may (swas-legacy): ogen refuses the name rather
        # than fixing it, and the whole generation stops on that one service.
        package = ("".join(relative.parts[:-1]) + relative.parts[-1]).replace("-", "")
        ogen.generate_api(spec, ROOT / relative, package, ROOT)
        print(str(relative), "native client + server + referenced models", flush=True)
        module = ROOT / relative.parent / "go.mod"
        if not module.exists():
            module.write_text("module github.com/leaflowapis/leaflow-go/" + str(relative.parent) + "\n\ngo 1.27.3\n\nrequire github.com/ogen-go/ogen v1.24.0\n")


def generate_protos(source):
    # 契约仓库只有一部分是 proto（带 buf.yaml 的那个 buf 模块）。没有它就没有可生成的东西。
    if not (source / "buf.yaml").exists():
        return
    # 先删旧产物再生成：契约里删掉的 proto 不会被 buf 覆盖，留下的 .pb.go 照样能编译，而它描述的
    # 消息已经不在契约里了。拉下来的契约副本不在本仓库的产物范围内，跳过。
    for stale in ROOT.glob("*/**/*.pb.go"):
        if stale.relative_to(ROOT).parts[0] != "leaflowapis":
            stale.unlink()
    subprocess.run(
        ["go", "run", BUF, "generate", str(source), "--template", str(ROOT / "buf.gen.yaml")],
        cwd=ROOT, env={**os.environ, "GOWORK": "off", "GOTOOLCHAIN": "local"}, check=True,
    )
    print("proto messages", flush=True)


def main():
    ref = (ROOT / "CONTRACTS_REF").read_text().strip()
    source = os.environ.get("CONTRACTS_DIR")
    if source:
        source = pathlib.Path(source).resolve()
        require_source(source, ref)
    else:
        source = ROOT / 'leaflowapis'
        contracts.fetch('https://github.com/leaflowapis/leaflowapis.git', source)
    generate_services(source)
    generate_protos(source)


if __name__ == "__main__":
    main()
