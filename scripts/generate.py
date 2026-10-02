#!/usr/bin/env python3
"""用 pinned 原生 ogen 在 server 包生成客户端和服务端。"""
import os
import pathlib
import subprocess
import tempfile
import contracts
import ogen

ROOT = pathlib.Path(__file__).resolve().parent.parent


def require_source(source, ref):
    head = subprocess.check_output(["git", "-C", str(source), "rev-parse", "HEAD"], text=True).strip()
    if head != ref:
        raise ValueError("source HEAD differs from CONTRACTS_REF")
    return head


def generate_services(source, public, generation_cwd):
    with tempfile.TemporaryDirectory(prefix="native-contracts-") as raw:
        scratch = pathlib.Path(raw) / "contracts"
        ogen.prepare_contracts(source, scratch)
        specs = sorted((scratch / "leaflow").glob("**/openapi.yaml"))
        for spec in specs:
            relative = spec.parent.relative_to(scratch / "leaflow")
            parts = relative.parts
            service = "/".join(parts[:-1])
            package = "".join(parts[:-1]) + parts[-1]
            ogen.generate_api(spec, ROOT / relative, package, generation_cwd, defaults=public)
            print(str(relative), "native client + server", flush=True)
            if public:
                module = ROOT / service / "go.mod"
                if not module.exists():
                    module.write_text("module github.com/leaflowapis/leaflow-go/" + service + "\n\ngo 1.26.0\n\nrequire github.com/ogen-go/ogen v1.24.0\n")


def main():
    ref = (ROOT / "CONTRACTS_REF").read_text().strip()
    source = os.environ.get("CONTRACTS_DIR")
    if source:
        source = pathlib.Path(source).resolve()
        require_source(source, ref)
    else:
        source = ROOT / 'leaflowapis'
        contracts.fetch('https://github.com/leaflowapis/leaflowapis.git', source)
    public = True
    if public:
        ogen.generate_shared(source, ROOT / "type/v1")
    with ogen.load_shared(ROOT, ROOT / "type" if public else None) as cwd:
        generate_services(source, public, cwd)
    if not public:
        if os.environ.get("SKIP_PROTO"):
            print("proto preserved (explicit generation-stage skip)", flush=True)
        else:
            subprocess.run(["buf", "generate", str(source)], check=True, cwd=ROOT)


if __name__ == "__main__":
    main()
