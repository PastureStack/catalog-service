"""Offline controls for the final Dapper test environment, using stdlib only."""
import ast
from contextlib import redirect_stdout
from hashlib import sha256
import io
import json
from pathlib import Path
import re
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]
VERIFIER = (ROOT / "scripts/verify-dapper-vex").read_text(encoding="utf-8")
PYTHON = VERIFIER.split("python3 - <<'PY'\n", 1)[1].rsplit("\nPY", 1)[0]
TREE = ast.parse(PYTHON)
TREE.body.pop()  # Keep the real pure checker; do not run container-only paths locally.
MODULE = {}
exec(compile(TREE, "verify-dapper-vex", "exec"), MODULE)


class InstallerRetirement(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        root = Path(self.temporary.name)
        self.site = root / "site"
        self.system = root / "system"
        self.seeds = root / "seeds"
        self.wheels = root / "wheelhouse"
        for directory in (self.site, self.system, self.seeds, self.wheels):
            directory.mkdir()
        lines = []
        for line in (ROOT / "integration/requirements.lock").read_text(encoding="utf-8").splitlines():
            name, version = line.split(" --hash=", 1)[0].split("==")
            canonical = MODULE["canonical"](name)
            content = (canonical + "==" + version).encode("ascii")
            (self.wheels / (canonical + ".whl")).write_bytes(content)
            lines.append(name + "==" + version + " --hash=sha256:" + sha256(content).hexdigest())
            self.distribution(canonical, version)
        self.requirements = root / "requirements.lock"
        self.requirements.write_text("\n".join(lines) + "\n", encoding="utf-8")
        self.lock_sha = sha256(self.requirements.read_bytes()).hexdigest()

    def distribution(self, name, version, root=None):
        metadata = (root or self.site) / (name.replace("-", "_") + "-" + version + ".dist-info")
        metadata.mkdir()
        (metadata / "METADATA").write_text("Name: " + name + "\nVersion: " + version + "\n", encoding="utf-8")

    def check(self):
        with redirect_stdout(io.StringIO()):
            MODULE["verify"](self.site, self.requirements, self.system,
                             self.seeds, self.wheels, self.lock_sha)

    def test_exact_test_environment_passes(self):
        self.check()

    def test_extra_or_changed_distribution_rejected(self):
        self.distribution("pip", "26.2.1")
        with self.assertRaises(SystemExit):
            self.check()
        extra = self.site / "pip-26.2.1.dist-info/METADATA"
        extra.unlink()
        extra.parent.rmdir()
        (self.site / "urllib3-2.8.0.dist-info/METADATA").write_text("Name: urllib3\nVersion: 2.7.0\n", encoding="utf-8")
        with self.assertRaises(SystemExit):
            self.check()
        (self.site / "urllib3-2.8.0.dist-info/METADATA").unlink()
        (self.site / "urllib3-2.8.0.dist-info").rmdir()
        with self.assertRaises(SystemExit):
            self.check()

    def test_code_without_metadata_rejected(self):
        for root in (self.site, self.system):
            for name in MODULE["RETIRED"]:
                code = root / name
                code.mkdir()
                with self.assertRaises(SystemExit):
                    self.check()
                code.rmdir()

    def test_system_distribution_and_seed_rejected(self):
        self.distribution("pip", "25.1.1", self.system)
        with self.assertRaises(SystemExit):
            self.check()
        metadata = self.system / "pip-25.1.1.dist-info/METADATA"
        metadata.unlink()
        metadata.parent.rmdir()
        (self.seeds / "pip-26.2.1-py3-none-any.whl").write_bytes(b"seed")
        with self.assertRaises(SystemExit):
            self.check()

    def test_lock_and_wheel_mutation_rejected(self):
        original = self.requirements.read_bytes()
        self.requirements.write_bytes(original + b"unlocked\n")
        with self.assertRaises(SystemExit):
            self.check()
        self.requirements.write_bytes(original)
        next(self.wheels.iterdir()).write_bytes(b"changed")
        with self.assertRaises(SystemExit):
            self.check()

    def test_source_contract_and_same_test_entry_points(self):
        self.assertEqual(MODULE["TEST_LOCK_SHA256"], sha256((ROOT / "integration/requirements.lock").read_bytes()).hexdigest())
        docker = (ROOT / "Dockerfile.dapper").read_text(encoding="utf-8")
        self.assertIn("venv --without-pip /opt/tox", docker)
        self.assertIn("pip --python /opt/tox install --no-cache-dir --require-hashes", docker)
        self.assertNotIn("autoremove", docker)
        self.assertIn("for package in python3-pip python3-pip-whl; do", docker)
        self.assertIn('apt-get purge -y "${package}" || exit 1;', docker)
        build = {line.split("==")[0].lower() for line in (ROOT / "integration/build-requirements.lock").read_text().splitlines()}
        test = {line.split("==")[0].lower() for line in (ROOT / "integration/requirements.lock").read_text().splitlines()}
        uninstall = re.search(r"pip uninstall -y (.*?) &&", docker, re.S).group(1).replace("\\", "").split()
        self.assertEqual(set(uninstall), build - test - {"pip"})
        self.assertIn("/opt/tox/bin/pip uninstall -y pip &&", docker)
        runner = (ROOT / "scripts/test").read_text(encoding="utf-8")
        self.assertIn("go test -mod=vendor ${RACE} -cover -tags=test ./...", runner)
        self.assertTrue(runner.endswith("cd integration\n/opt/tox/bin/python -I -m flake8 core\ncd core\n/opt/tox/bin/python -I -m pytest --durations=20\n"))

    def test_vex_only_exact_reviewed_header_package(self):
        vex = json.loads((ROOT / "security/dapper.openvex.json").read_text(encoding="utf-8"))
        pairs = [(s["vulnerability"]["name"], s["products"][0]["@id"]) for s in vex["statements"]]
        self.assertEqual(len(pairs), 82)
        self.assertEqual(len(set(pairs)), 82)
        self.assertEqual({p for _, p in pairs}, {"pkg:deb/ubuntu/linux-libc-dev@7.0.0-38.38?arch=amd64&distro=ubuntu-26.04"})
        self.assertTrue(all(s["status"] == "not_affected" and s["justification"] == "vulnerable_code_not_present" for s in vex["statements"]))


if __name__ == "__main__":
    unittest.main()
