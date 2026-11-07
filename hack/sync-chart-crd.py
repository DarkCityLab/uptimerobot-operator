#!/usr/bin/env python3
"""Regenerate the Helm chart CRDs from config/crd/bases.

`make manifests` writes config/crd/bases/*.yaml but the Helm chart ships its own
copy under dist/chart/templates/crd/, which otherwise drifts (a stale schema
makes the apiserver prune new status/spec fields). This rewrites each chart CRD
from the base, re-applying the chart's `crd.enable` guard and the `crd.keep`
resource-policy annotation. Run via `make sync-chart-crd` (included in manifests).
"""
import glob
import os
import pathlib

BASE_DIR = "config/crd/bases"
OUT_DIR = "dist/chart/templates/crd"


def convert(src: str) -> None:
    lines = pathlib.Path(src).read_text().splitlines()
    if lines and lines[0].strip() == "---":
        lines = lines[1:]

    name = None
    out = []
    for ln in lines:
        stripped = ln.strip()
        if stripped.startswith("controller-gen.kubebuilder.io/version:"):
            indent = ln[: len(ln) - len(ln.lstrip())]
            out.append(f"{indent}{{{{- if .Values.crd.keep }}}}")
            out.append(f'{indent}"helm.sh/resource-policy": keep')
            out.append(f"{indent}{{{{- end }}}}")
        if name is None and ln.startswith("  name: "):
            name = stripped.split(":", 1)[1].strip()
        out.append(ln)

    if not name:
        raise SystemExit(f"could not determine CRD name in {src}")
    dst = os.path.join(OUT_DIR, f"{name}.yaml")
    content = "{{- if .Values.crd.enable }}\n" + "\n".join(out) + "\n{{- end }}\n"
    pathlib.Path(dst).write_text(content)
    print(f"synced {dst}")


def main() -> None:
    os.makedirs(OUT_DIR, exist_ok=True)
    for src in sorted(glob.glob(os.path.join(BASE_DIR, "*.yaml"))):
        convert(src)


if __name__ == "__main__":
    main()
