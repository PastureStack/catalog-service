#!/usr/bin/env python3
"""Rebuild pip using released urllib3 and pip's unmodified vendoring machinery.

Build dependencies never enter the product wheel. All network inputs are HTTPS
and SHA-256 pinned. The resulting local version is not an official pip release.
"""
import argparse
import base64
import csv
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
from urllib.request import HTTPRedirectHandler, HTTPSHandler, build_opener
from urllib.parse import urlsplit
import venv
import zipfile


class HTTPSOnlyRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        if urlsplit(newurl).scheme != "https":
            raise ValueError("Refusing a non-HTTPS build-input redirect")
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(command, cwd, env):
    print("+", " ".join(map(str, command)), flush=True)
    subprocess.run(list(map(str, command)), cwd=cwd, env=env, check=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--work", type=Path, required=True)
    parser.add_argument("--input-cache", type=Path)
    parser.add_argument("--offline", action="store_true")
    args = parser.parse_args()
    output, work = args.output.resolve(), args.work.resolve()
    output.mkdir(parents=True, exist_ok=True)
    work.mkdir(parents=True, exist_ok=True)
    config = json.loads(Path(__file__).with_name("inputs.json").read_text())
    assert sys.version_info[:2] in ((3, 12), (3, 14)) and os.name == "posix"
    scratch = Path(tempfile.mkdtemp(prefix="pip-revendor-", dir=work))
    downloads = scratch / "downloads"
    downloads.mkdir()
    cache = args.input_cache.resolve() if args.input_cache else downloads
    cache.mkdir(parents=True, exist_ok=True)
    paths = {}
    for item in config["artifacts"]:
        assert urlsplit(item["url"]).scheme == "https"
        target = downloads / Path(urlsplit(item["url"]).path).name
        cached = cache / target.name
        if not cached.is_file():
            assert not args.offline, ("Offline input is missing", target.name)
            opener = build_opener(HTTPSHandler(), HTTPSOnlyRedirect())
            with opener.open(item["url"], timeout=90) as response, cached.open("wb") as dest:
                shutil.copyfileobj(response, dest)
        assert sha(cached) == item["sha256"], ("cached input hash mismatch", target.name)
        if cached != target:
            shutil.copy2(cached, target)
        assert sha(target) == item["sha256"], ("input hash mismatch", target.name)
        paths.setdefault(item["name"], []).append(target)

    environment = os.environ.copy()
    environment.update(PIP_NO_INDEX="1", PIP_FIND_LINKS=str(downloads),
                       PIP_DISABLE_PIP_VERSION_CHECK="1", PIP_ONLY_BINARY=":all:",
                       PYTHONHASHSEED="0", SOURCE_DATE_EPOCH=str(config["sourceDateEpoch"]))
    for key in ("PYTHONPATH", "PYTHONHOME", "PIP_INDEX_URL", "PIP_EXTRA_INDEX_URL",
                "PIP_TRUSTED_HOST", "PIP_CERT", "PIP_CLIENT_CERT"):
        environment.pop(key, None)
    environment["PIP_CONFIG_FILE"] = os.devnull
    tool_env = scratch / "tool-env"
    venv.EnvBuilder(with_pip=False).create(tool_env)
    python = tool_env / "bin/python"
    requirements = scratch / "tools.lock"
    groups = {}
    for item in config["artifacts"]:
        if item["kind"] == "tool":
            groups.setdefault((item["name"], item["version"]), []).append(item["sha256"])
    requirements.write_text("\n".join(
        f"{name}=={version} " + " ".join(f"--hash=sha256:{value}" for value in hashes)
        for (name, version), hashes in sorted(groups.items())) + "\n")
    bootstrap_env = dict(environment, PYTHONPATH=str(paths["pip"][0]))
    run([python, "-m", "pip", "install", "--require-hashes", "--no-compile", "-r", requirements], scratch, bootstrap_env)
    environment["PATH"] = str(tool_env / "bin") + os.pathsep + environment["PATH"]

    source_parent = scratch / "source"
    source_parent.mkdir()
    with tarfile.open(paths["pip-source"][0]) as archive:
        # Reject links and traversal before extracting this fixed official archive.
        for member in archive.getmembers():
            assert not member.issym() and not member.islnk()
            resolved = (source_parent / member.name).resolve()
            assert resolved.is_relative_to(source_parent)
        archive.extractall(source_parent, filter="data")
    source = next(source_parent.iterdir())
    vendor = source / "src/pip/_vendor"
    original = {str(p.relative_to(vendor)): sha(p) for p in vendor.rglob("*") if p.is_file()}
    assert (source / "src/pip/__init__.py").read_text().count('__version__ = "26.2.2"') == 1

    # A scoped project invokes the same upstream tool, namespace and urllib3
    # patches. Unrelated vendored libraries are never regenerated or replaced.
    scoped = scratch / "urllib3-vendoring"
    scoped_vendor = scoped / "src/pip/_vendor"
    scoped_vendor.mkdir(parents=True)
    (scoped_vendor / "vendor.txt").write_text("urllib3==2.8.0\n")
    patches = scoped / "tools/vendoring/patches"
    patches.mkdir(parents=True)
    for patch in (source / "tools/vendoring/patches").glob("urllib3*.patch"):
        shutil.copy2(patch, patches / patch.name)
    assert len(list(patches.glob("*.patch"))) == 3
    (scoped / "pyproject.toml").write_text('''[tool.vendoring]
destination = "src/pip/_vendor/"
requirements = "src/pip/_vendor/vendor.txt"
namespace = "pip._vendor"
protected-files = ["vendor.txt"]
patches-dir = "tools/vendoring/patches"
[tool.vendoring.transformations]
drop = ["*.dist-info/", "bin/"]
''')
    run([python, "-m", "vendoring", "sync", "-v"], scoped, environment)
    generated = scoped_vendor / "urllib3"
    assert (generated / "__init__.py").is_file()
    shutil.rmtree(vendor / "urllib3")  # exact disposable source subtree only
    shutil.copytree(generated, vendor / "urllib3")
    vendor_txt = vendor / "vendor.txt"
    text = vendor_txt.read_text()
    assert text.count("urllib3==2.7.0") == 1
    vendor_txt.write_text(text.replace("urllib3==2.7.0", "urllib3==2.8.0"))
    version_file = source / "src/pip/__init__.py"
    version_file.write_text(version_file.read_text().replace('__version__ = "26.2.2"',
                           f'__version__ = "{config["version"]}"'))
    readme = vendor / "README.rst"
    with readme.open("a") as handle:
        handle.write("\n\nPastureStack security rebuild\n============================\n\n"
                     "This downstream local-version wheel uses released urllib3 2.8.0\n"
                     "through upstream vendoring 1.4.0 and pip's unchanged urllib3\n"
                     "patches. Other vendored library sources and licenses are unchanged.\n"
                     "It is not an official PyPA pip release.\n")
    run([python, "-c", "from pathlib import Path; from vendoring.sbom import create_sbom_file; create_sbom_file('pip._vendor', Path('src/pip/_vendor/vendor.txt'), Path('src/pip/_vendor/bom.cdx.json'))"], source, environment)
    current = {str(p.relative_to(vendor)): sha(p) for p in vendor.rglob("*") if p.is_file()}
    allowed = {"vendor.txt", "README.rst", "bom.cdx.json"}
    assert {p: value for p, value in original.items() if not p.startswith("urllib3/") and p not in allowed} == {
        p: value for p, value in current.items() if not p.startswith("urllib3/") and p not in allowed}
    wheels = []
    for suffix in ("first", "second"):
        directory = scratch / suffix
        directory.mkdir()
        run([python, "-c", f"from flit_core.buildapi import build_wheel; build_wheel({str(directory)!r})"], source, environment)
        wheels.append(next(directory.glob("*.whl")))
    assert sha(wheels[0]) == sha(wheels[1]), "wheel rebuild is not byte reproducible"
    assert sha(wheels[0]) == config["expectedWheelSha256"], "unexpected rebuilt product bytes"
    wheel = output / wheels[0].name
    shutil.copy2(wheels[0], wheel)
    licenses = []
    with zipfile.ZipFile(wheel) as archive:
        names = archive.namelist()
        record = next(name for name in names if name.endswith(".dist-info/RECORD"))
        for name, digest, size in csv.reader(io.StringIO(archive.read(record).decode())):
            if not digest:
                assert name == record
                continue
            data = archive.read(name)
            expected = "sha256=" + base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b"=").decode()
            assert digest == expected and int(size) == len(data), name
        assert not any(name.endswith((".so", ".pyd")) for name in names)
        licenses = [name for name in names if "LICENSE" in name or "COPYING" in name]
        assert any("urllib3" in name for name in licenses)
        assert "pip/_vendor/pkg_resources/__init__.py" in names
        assert not any("setuptools/package_index" in name for name in names)
        shutil.copy2(vendor / "bom.cdx.json", output / "pip-vendor.cdx.json")
    report = {"version": config["version"], "upstreamVersion": config["upstreamVersion"],
              "upstreamCommit": config["sourceCommit"], "wheel": wheel.name,
              "sha256": sha(wheel), "twoBuildsByteIdentical": True,
              "allRecordHashesVerified": True, "nativeExtensionsPresent": False,
              "licenses": licenses, "onlyVendoredLibraryChanged": "urllib3", "urllib3": "2.8.0"}
    (output / "build-evidence.json").write_text(json.dumps(report, indent=2) + "\n")
    shutil.copy2(requirements, output / "tools.lock")
    shutil.copy2(Path(__file__).with_name("inputs.json"), output / "inputs.json")
    print(json.dumps(report), flush=True)


if __name__ == "__main__":
    if sys.flags.optimize:
        raise RuntimeError("Security build verification must not run with Python optimization")
    main()
