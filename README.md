# PastureStack Catalog Service

Catalog Service indexes reviewed catalog repositories and serves compatible catalog, template, version, question, icon, and upgrade-link APIs.

PastureStack is an independent community effort to preserve, audit, and modernize the Rancher 1.6 ecosystem. It is not affiliated with or endorsed by Rancher Labs or SUSE.

**Upstream:** [`rancher/catalog-service`](https://github.com/rancher/catalog-service). This GitHub fork preserves upstream history, authorship, dates, tags, licenses, and bundled dependency notices; all PastureStack maintenance follows the preserved upstream boundary.

## Project status

The migration-reviewed compatibility release retains the Ubuntu 26.04, Go 1.26.5, database, dependency, version-filter, TLS, and build maintenance completed after the preserved upstream boundary. Product-owned imports, binaries, tracking identifiers, default configuration, version query, and operator messages use PastureStack naming. The default `repo.json` is intentionally empty; no unreviewed catalog is cloned. Python integration-test dependencies are pinned and installed from an offline wheelhouse inside the disposable build image.

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

Maintainers can create an immutable release from the current `main` commit with the manual **Release Catalog Service** GitHub workflow. The workflow accepts a semantic release tag, rejects an existing tag or release, and publishes `catalog-service` and `catalog-service-sqlite` together in one checksummed archive.

## License and attribution

The inherited project remains licensed under [Apache License 2.0](LICENSE). Copyright and attribution for inherited work and vendored dependencies remain with their respective authors and contributors. PastureStack contributors claim authorship only for their own changes.
