#!/usr/bin/env python3
"""Valida um consumidor contra este checkout sem alterar os seus módulos."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("consumer", type=Path, help="Diretório do serviço consumidor")
    parser.add_argument("--build-only", action="store_true", help="Compila sem executar testes")
    args = parser.parse_args()

    package = Path(__file__).resolve().parents[1]
    consumer = args.consumer.resolve()
    module = "github.com/deelperp/deelp-pkg"
    versions = []
    for directory in (package, consumer):
        manifest = directory / "go.mod"
        if not manifest.is_file():
            parser.error(f"go.mod ausente: {manifest}")
        source = manifest.read_text()
        version = re.search(r"^go ([0-9]+\.[0-9]+(?:\.[0-9]+)?)$", source, re.MULTILINE)
        if version is None:
            parser.error(f"Diretiva go ausente ou inválida: {manifest}")
        versions.append(version.group(1))
        if directory == consumer and not re.search(
            rf"^\s*(?:require\s+)?{re.escape(module)}\s+v", source, re.MULTILINE
        ):
            parser.error(f"{manifest} não declara dependência de {module}")

    version = max(versions, key=lambda value: tuple(map(int, value.split("."))))
    environment = os.environ.copy()
    environment["GOFLAGS"] = "-mod=readonly"
    with tempfile.TemporaryDirectory(prefix="deelp-compatibility-") as temporary:
        workspace = Path(temporary) / "go.work"
        workspace.write_text(
            f"go {version}\n\nuse (\n"
            f"\t{json.dumps(str(package))}\n\t{json.dumps(str(consumer))}\n)\n"
        )
        environment["GOWORK"] = str(workspace)
        resolved = subprocess.check_output(
            ["go", "list", "-m", "-f", "{{.Dir}}", module],
            cwd=consumer, env=environment, text=True,
        ).strip()
        if Path(resolved).resolve() != package:
            raise SystemExit(f"Candidato não selecionado: {resolved}")
        print(f"Consumidor: {consumer}\nCandidato: {package}", flush=True)
        command = ["go", "build", "./..."] if args.build_only else [
            "go", "test", "-count=1", "-timeout=90s", "./..."
        ]
        result = subprocess.run(command, cwd=consumer, env=environment)
        return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
