# PastureStack Catalog Service

Catalog Service indexes reviewed catalog repositories and serves compatible catalog, template, version, question, icon, and upgrade-link APIs.

PastureStack is an independent community effort to preserve, audit, and modernize the Rancher 1.6 ecosystem. It is not affiliated with or endorsed by Rancher Labs or SUSE.

**Upstream:** [`rancher/catalog-service`](https://github.com/rancher/catalog-service). This GitHub fork preserves upstream history, authorship, dates, tags, licenses, and bundled dependency notices; all PastureStack maintenance follows the preserved upstream boundary.

## Project status

The current published numeric maintenance release is
[`v0.20.15`](https://github.com/PastureStack/catalog-service/releases/tag/v0.20.15),
built with Go 1.27.3. The actual archive and both binary producers are source
commit `6ea547594065f7f95d5877b82f0c1bc553e72640`. Its archive, archive checksum,
license and license checksum are published immutable assets. Official
publication and reproducible packaging checks passed; matching Server
integration and acceptance remain separate and are not established by this
component release.

The next numeric candidate, **v0.20.16**, adds a private loopback-only Catalog
metadata interface for typed Engine reads. Template and version IDs derive
from logical catalog identity rather than disposable cache rows. Bounded
responses exclude repository credentials, launch files, questions, defaults
and readme; existing Catalog APIs keep their contracts. See
[the metadata contract](docs/ai-metadata.md). This candidate is not published;
its official Go 1.27.3/security gates and matching Engine/Web runtime
integration are pending.

Earlier isolated upgrade checks confirmed removal of empty Git-metadata and
README-only index entries while preserving valid catalog content, plus the
remaining upgrade steps and cleanup of an existing catalog. That evidence
covers only those flows; earlier incomplete results are not retroactively
passed. The full resource and permission matrix remains INCOMPLETE.

The disposable AMD64 release builder also pins the official curl
8.18.0-1ubuntu2.8 payloads and certifi 2026.10.19 wheel; its existing
OpenSSL 3.5.5-1ubuntu3.7 pin is retained. Refreshed exact header-only VEX
statements cover only the reviewed linux-libc-dev 7.0.0-38.38 package,
not a host kernel or the Go runtime. Raw-to-VEX equality, no-kernel-package
and independent product gates remain unchanged.

The retained indexing fix skips Git's `.git` metadata, validates a
numeric revision or semantic-version folder before reading a version file or
allocating a template, and rebuilds a same-commit index containing unnamed
templates through the existing catalog transaction. Other catalogs are not
included in that cache check. Native revision numbers, semantic versions,
template metadata and labels retain their meaning. Surrogate and API template
IDs may change during transactional reindexing; identify content by catalog
origin, template key and version, not by assuming those IDs are permanent.
README, icon and version files alone do not emit templates: a successfully
parsed `config.yml` or `template.yml` must establish the folder identity.
Root README files are resolved from their containing directory rather than
mistaken for version folders. This repairs previously empty root README fields
from repository bytes; version README files, icons and valid numeric or semantic
versions retain their existing contents and interpretation.

It retains the Ubuntu 26.04, database, dependency, version-filter, TLS, and build maintenance completed after the preserved upstream boundary, with the compiler updated to Go 1.27.3 for the crypto/x509 fix CVE-2026-111943. Product-owned imports, binaries, default configuration, version query, and operator messages use PastureStack naming. The default `repo.json` is intentionally empty; no unreviewed catalog is cloned. Python build and integration-test dependencies are transitively pinned with package hashes and installed from an offline wheelhouse inside the disposable build image. The historical `--track` flag is accepted only for command-line compatibility; the service does not read or transmit an installation identifier. MySQL DSNs are created from the driver's reviewed defaults so existing `mysql_native_password` installations remain compatible after the driver upgrade.

The archived `docker/libcompose` parser and the unmaintained `go-rancher` client are no longer imported or vendored. A small project-owned compatibility layer now emits only the resource, schema, link, and JSON shapes this service actually uses. Catalog metadata is decoded through YAML v3 with focused compatibility tests for top-level legacy metadata, Compose v2 service metadata, alias fields, precedence, malformed input, and empty metadata. A source gate prevents the removed parser from returning.

Database access uses GORM v2 with current MySQL and SQLite drivers. Dependencies are resolved by Go Modules, locked by `go.mod` and `go.sum`, and rebuilt into `vendor/` so release builds remain offline and reproducible.

The disposable build image is pinned by digest. Its Ubuntu package source is fixed to the `20261002T000000Z` official snapshot, and every directly installed APT package has an exact version in `ubuntu-apt.lock`. The candidate security workflow builds and packages twice, runs the complete test, race, validation, and packaging path, scans source, product binaries, and the exported build-image filesystem, and uploads review evidence without publishing or deploying anything. Both the image-metadata and exported-filesystem raw reports are retained. Findings originating from an embedded third-party SBOM require exact OpenVEX set equality plus executable checks that the affected implementation and call path are absent; installed package databases remain independent evidence and product binaries are gated separately.

The same hash-locked integration dependencies are preinstalled in the isolated
`/opt/tox` environment, which runs flake8 and pytest directly without seeding
another environment. Bootstrap invokes Ubuntu's `/usr/bin/python3` explicitly
so the seedless environment placed first on PATH cannot shadow the installer.
The official pip installer, tox and virtualenv are retired
with their supported uninstall commands after preparation; the system pip
packages are explicitly purged without autoremove. The developer `tox.ini` is
retained. Bootstrap dependency locks remain provenance, not a claim that the
bootstrap phase has no vulnerabilities: pip's embedded urllib3 is not fixed by
installing an unrelated top-level urllib3. The final image must prove that the
retired installer code and packages are absent and that the exact test
dependencies remain installed; no urllib3 OpenVEX exception is permitted.

Catalog sources are denied unless their exact origin is authorized by the service operator. Reviewed public GitHub origins are built in. Add private HTTPS origins as a comma-separated list in `PASTURESTACK_CATALOG_ALLOWED_EXTERNAL_ORIGINS`; each entry must contain only a scheme, hostname, and optional port. Plain HTTP is accepted only for loopback tests. Local Git catalogs are restricted to isolated tests: `PASTURESTACK_CATALOG_ALLOWED_LOCAL_ROOTS` may enable only the platform temporary root. Catalog documents, API callers, redirects, icon links, and chart links cannot expand either policy.

Release packaging is manual and reproducible. The GitHub release workflow runs only when an organization maintainer explicitly dispatches it. It builds and tests the selected main-branch commit twice, requires byte-identical packages, and publishes both binaries in one checksummed archive, plus the license and its checksum, to a GitHub Release. It does not deploy a service or publish a catalog.

## Pinned GitHub catalogs

Git catalogs may specify both a branch and a full 40-character `pinnedCommit`. The service clones the branch, checks out the exact commit in detached mode, verifies `HEAD`, and does not advance a pinned catalog during refresh. This lets a server consume a reviewed public GitHub catalog without requiring operators to host an additional catalog service or mirror.

```json
{
  "catalogs": {
    "pasturestack": {
      "url": "https://github.com/PastureStack/catalog-templates.git",
      "branch": "main",
      "pinnedCommit": "FULL_40_CHARACTER_COMMIT_SHA"
    }
  }
}
```

The placeholder above must be replaced with a reviewed commit. An omitted `pinnedCommit` preserves the historical moving-branch behavior for compatibility and is not suitable for a PastureStack release gate.

## API migration

Use `platformVersion` when filtering templates and upgrade links. The historical `rancherVersion` and `minimumRancherVersion_lte` query parameters remain read-only compatibility fallbacks. Set `PASTURESTACK_LOCALE=en-US` or `zh-TW` for operator lifecycle messages.

## Build and test

From a Docker-capable Linux host:

To rebuild the published source, check out `v0.20.15` and set
`VERSION_OVERRIDE=v0.20.15`. The default Go 1.27.3 archive pins cover
Linux/amd64 and Linux/armv6l; the published artifact qualification covers
Linux/amd64 only, not an ARM binary build.

```sh
make test
make build
make package
```

Catalog repository URLs must be supplied explicitly in a reviewed configuration. See [COMPATIBILITY.md](COMPATIBILITY.md), [SECURITY.md](SECURITY.md), and [ORIGIN.md](ORIGIN.md).

Before a future release is approved, run the **Security release gate** workflow against the exact candidate commit and verify that its source revision, reproducible archive checksum, SBOMs, raw findings, and applicable findings all match that commit. Release `v0.20.15` passed that gate on candidate `4379307d07941ee2a4e8d6fbeb0686d519c4753e`, whose whole source tree exactly matched the published producer above. The release workflow separately built the actual published producer twice; candidate bytes were not substituted. This repository still does not deploy the service by itself.

Maintainers can create an immutable release from the current `main` commit with the manual **Release Catalog Service** GitHub workflow. The workflow accepts a semantic release tag, rejects an existing tag or release, and publishes `catalog-service` and `catalog-service-sqlite` together in one checksummed archive.

## License and attribution

The inherited project remains licensed under [Apache License 2.0](LICENSE). Copyright and attribution for inherited work and vendored dependencies remain with their respective authors and contributors. PastureStack contributors claim authorship only for their own changes.

## October 2026 security maintenance

Maintained Go builds select the patched Go 1.27.3 compiler without raising the existing Go language requirement. Ubuntu build/runtime stages apply the checksum-pinned `ca-certificates 20260816ubuntu0.26.04.1` security package after their frozen APT baseline; the final trust store and installed version are verified. Python tooling uses the patched pip/CA path; CI interpreter pins remain on the Python 3.14 series. Release artifacts must pass the repository's existing checks before publication.
# Build-only pip security refresh

The installer bootstrap is reproducibly rebuilt from official pip 26.2.2 and urllib3 2.8.0 using the pinned upstream vendoring machinery. Its local version is `26.2.2+pasturestack.1`, not an official PyPA release. The expected wheel SHA-256 is `be5f63d537bbe611dc422fabb6740a44b5997888b592888df5091727f023802a`. Streaming and TLS controls run before dependency download; installers and seed wheels are then retired as before. This build-only change does not replace the already verified v0.20.15 runtime archive.

Future release gates retain and evaluate every product severity, including UNKNOWN.
No newly reported or officially fixable product vulnerability is accepted; this
policy-only update does not rebuild or replace the current immutable release.

