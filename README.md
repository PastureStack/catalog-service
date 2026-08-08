# PastureStack Catalog Service

Catalog Service indexes reviewed catalog repositories and serves compatible catalog, template, version, question, icon, and upgrade-link APIs.

PastureStack is an independent community effort to preserve, audit, and modernize the Rancher 1.6 ecosystem. It is not affiliated with or endorsed by Rancher Labs or SUSE.

**Upstream:** [`rancher/catalog-service`](https://github.com/rancher/catalog-service). This GitHub fork preserves upstream history, authorship, dates, tags, licenses, and bundled dependency notices; all PastureStack maintenance follows the preserved upstream boundary.

## Project status

The current numeric maintenance candidate is `0.20.9`. It retains the Ubuntu 26.04, Go 1.26.5, database, dependency, version-filter, TLS, and build maintenance completed after the preserved upstream boundary. Product-owned imports, binaries, default configuration, version query, and operator messages use PastureStack naming. The default `repo.json` is intentionally empty; no unreviewed catalog is cloned. Python build and integration-test dependencies are transitively pinned with package hashes and installed from an offline wheelhouse inside the disposable build image. The historical `--track` flag is accepted only for command-line compatibility; the service does not read or transmit an installation identifier.

The archived `docker/libcompose` parser is no longer imported or vendored. Catalog metadata is decoded through the project's existing YAML dependency with focused compatibility tests for top-level legacy metadata, Compose v2 service metadata, alias fields, precedence, malformed input, and empty metadata. A source gate prevents the removed parser from returning.

The disposable build image is pinned by digest. Its Ubuntu package source is fixed to the `20260808T000000Z` official snapshot, and every directly installed APT package has an exact version in `ubuntu-apt.lock`. The candidate security workflow builds and packages twice, runs the complete test, race, validation, and packaging path, scans source, product binaries, and the exported build-image filesystem, and uploads review evidence without publishing or deploying anything. Both the image-metadata and exported-filesystem raw reports are retained. Findings originating from an embedded third-party SBOM require exact OpenVEX set equality plus executable checks that the affected implementation and call path are absent; installed package databases remain independent evidence and product binaries are gated separately.

Catalog sources are denied unless their exact origin is authorized by the service operator. Reviewed public GitHub origins are built in. Add private HTTPS origins as a comma-separated list in `PASTURESTACK_CATALOG_ALLOWED_EXTERNAL_ORIGINS`; each entry must contain only a scheme, hostname, and optional port. Plain HTTP is accepted only for loopback tests. Local Git catalogs are restricted to isolated tests: `PASTURESTACK_CATALOG_ALLOWED_LOCAL_ROOTS` may enable only the platform temporary root. Catalog documents, API callers, redirects, icon links, and chart links cannot expand either policy.

Release packaging is manual and reproducible. The GitHub release workflow runs only when an organization maintainer explicitly dispatches it. It builds and tests the selected main-branch commit twice, requires byte-identical packages, and publishes the binaries and checksum to a GitHub Release. It does not deploy a service or publish a catalog.

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

```sh
make test
make build
make package
```

Catalog repository URLs must be supplied explicitly in a reviewed configuration. See [COMPATIBILITY.md](COMPATIBILITY.md), [SECURITY.md](SECURITY.md), and [ORIGIN.md](ORIGIN.md).

Before a release is approved, run the **Security release gate** workflow against the exact candidate commit and verify that its source revision, reproducible archive checksum, SBOMs, raw findings, and applicable findings all match that commit. `0.20.9` is a source candidate until that gate succeeds and an immutable `v0.20.9` release is created; it is not deployed by this repository.

Maintainers can create an immutable release from the current `main` commit with the manual **Release Catalog Service** GitHub workflow. The workflow accepts a semantic release tag, rejects an existing tag or release, and publishes `catalog-service` and `catalog-service-sqlite` together in one checksummed archive.

## License and attribution

The inherited project remains licensed under [Apache License 2.0](LICENSE). Copyright and attribution for inherited work and vendored dependencies remain with their respective authors and contributors. PastureStack contributors claim authorship only for their own changes.
