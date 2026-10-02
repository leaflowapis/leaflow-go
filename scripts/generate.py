#!/usr/bin/env python3
"""用 pinned 原生 ogen 在 server 包生成客户端和服务端。"""
import os
import pathlib
import subprocess
import contracts
import ogen

ROOT = pathlib.Path(__file__).resolve().parent.parent


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
        package = "".join(relative.parts[:-1]) + relative.parts[-1]
        ogen.generate_api(spec, ROOT / relative, package, ROOT)
        print(str(relative), "native client + server + referenced models", flush=True)
        module = ROOT / relative.parent / "go.mod"
        if not module.exists():
            module.write_text("module github.com/leaflowapis/leaflow-go/" + str(relative.parent) + "\n\ngo 1.26.0\n\nrequire github.com/ogen-go/ogen v1.24.0\n")


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


if __name__ == "__main__":
    main()
