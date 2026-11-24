import os
import pathlib
import subprocess
import tempfile

import yaml

OGEN = "github.com/ogen-go/ogen/cmd/ogen@v1.24.0"


def generate_api(spec, output, package, cwd):
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
    if package == "okdatav1":
        wire_query_client(target)
    for path in (output / "client.gen.go", output / "defaults.gen.go", target / "defaults.gen.go"):
        path.unlink(missing_ok=True)


OKDATA_QUERY_CLIENT = """package okdatav1server

import (
\t"errors"
\t"net/http"
\t"strings"

\tht "github.com/ogen-go/ogen/http"
)

type queryClient struct {
\tht.Client
}

func (c queryClient) Do(req *http.Request) (*http.Response, error) {
\tif !isMeteredDataQuery(req.Method, req.URL.Path) {
\t\treturn c.Client.Do(req)
\t}

\tkeys := req.Header.Values("Idempotency-Key")
\tif len(keys) > 1 || (len(keys) == 1 && !validQueryKey(keys[0])) {
\t\treturn nil, errors.New("Idempotency-Key must contain 1 to 255 visible ASCII characters")
\t}

\t// Preserve explicit caller replay without allowing transparent body resubmission.
\tr := req.WithContext(req.Context())
\tr.GetBody = nil
\treturn c.Client.Do(r)
}

func isMeteredDataQuery(method, path string) bool {
\t_, resource, found := strings.Cut(path, "/api/v1/")
\tif !found || (method != http.MethodGet && method != "QUERY" && method != http.MethodPost) {
\t\treturn false
\t}
\tcollection, _, _ := strings.Cut(resource, "/")
\tswitch collection {
\tcase "enablement", "capabilities", "requests":
\t\treturn false
\tdefault:
\t\treturn true
\t}
}

func validQueryKey(key string) bool {
\tif len(key) == 0 || len(key) > 255 {
\t\treturn false
\t}
\tfor i := 0; i < len(key); i++ {
\t\tif key[i] < '!' || key[i] > '~' {
\t\t\treturn false
\t\t}
\t}
\treturn true
}
"""


def wire_query_client(target):
    (target / "query_client.go").write_text(OKDATA_QUERY_CLIENT, encoding="utf-8")
    path = target / "oas_cfg_gen.go"
    source = path.read_bytes()
    marker = b"\tcfg.Client = queryClient{cfg.Client}\n"
    boundary = b"\tcfg.initOTEL()\n\treturn cfg\n}\n\ntype baseClient struct"
    if source.count(boundary) != 1 or marker in source:
        raise ValueError("Cannot wire the OKData query client into this ogen output")
    path.write_bytes(source.replace(boundary, marker + boundary))
