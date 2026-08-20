"""把契约仓库拉到本地，供生成器使用。

# 为什么是拉取而不是 submodule

语言仓库是契约的**产物**，产物不该反过来持有源的一个 git 指针。submodule 换来的是同一件事，
代价是四类只在 CI 上出现的失败：runner 镜像内无 ssh、job token 默认跨项目不可用、嵌套
submodule 不随 checkout 初始化、以及生成器把 submodule 目录当作一个服务。四者报出的都不是
「submodule 配置有误」。

# 版本记录在 CONTRACTS_REF

它是一个 commit sha 或 tag，与 submodule 指针作用相同，但它是一个普通文件：diff 可读，
review 时看得见「这版 SDK 出自哪版契约」。
"""
import os, pathlib, shutil, subprocess, sys, tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
REF_FILE = ROOT / "CONTRACTS_REF"


def fetch(remote: str, destination: pathlib.Path) -> str:
    """把契约取到 destination，返回取到的 commit。"""
    ref = REF_FILE.read_text(encoding="utf-8").strip() if REF_FILE.exists() else "main"
    shutil.rmtree(destination, ignore_errors=True)
    subprocess.run(["git", "clone", "--quiet", "--no-tags", remote, str(destination)],
                   check=True)
    subprocess.run(["git", "-C", str(destination), "checkout", "--quiet", ref], check=True)
    got = subprocess.run(["git", "-C", str(destination), "rev-parse", "HEAD"],
                         check=True, capture_output=True, text=True).stdout.strip()
    print(f"契约 {ref} → {got[:12]}")
    return got
