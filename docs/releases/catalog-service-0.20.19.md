# Catalog Service v0.20.19

Published on 2026-11-23, this maintenance release makes offline catalog bootstrap
and index repair native to Catalog Service. It retains the checksum-pinned
Go 1.27.4 compiler and existing source authorization, rooted cache, catalog
identity, and database transaction boundaries. Server assembly, deployment, and
matching runtime acceptance remain separate from this component publication.

## Published identity

- Release: [`v0.20.19`](https://github.com/PastureStack/catalog-service/releases/tag/v0.20.19).
- Immutable producer: [`151d62fff93deaca9b2486b0b071df6772d1a27b`](https://github.com/PastureStack/catalog-service/commit/151d62fff93deaca9b2486b0b071df6772d1a27b).
- Compiler: Go 1.27.4; published product qualification covers Linux/amd64.
- [Security release gate 45100342078](https://github.com/PastureStack/catalog-service/actions/runs/45100342078): success for that producer, including complete tests, race, reproducible packaging, scans, and provenance checks.
- [Release workflow 45101142629](https://github.com/PastureStack/catalog-service/actions/runs/45101142629): success for the same producer.

The downloaded release archive is byte-identical to the qualified gate archive.
Both binaries are contained in that archive; the license is a separate release
asset. The immutable SHA-256 values are:

| Artifact | SHA-256 |
| --- | --- |
| [`catalog-service-0.20.19.tar.xz`](https://github.com/PastureStack/catalog-service/releases/download/v0.20.19/catalog-service-0.20.19.tar.xz) | `dc46d200cfe8f454b234ca36c04043beeaa3cc48eac921f85118ee9c8db5e0c2` |
| `catalog-service` inside the archive | `4dc152e56630c845bfe9638aa06f3f28ae74da89342a5f8354847e9888cea5a9` |
| `catalog-service-sqlite` inside the archive | `7b82205cffde1989d033e9a1d9dd6a05138a5cef1d5107440ce24e118e00bac1` |
| [`catalog-service-0.20.19-LICENSE.txt`](https://github.com/PastureStack/catalog-service/releases/download/v0.20.19/catalog-service-0.20.19-LICENSE.txt) | `0d542e0c8804e39aa7f37eb00da5a762149dc682d7829451287e11b938e94594` |

This documentation update does not change those producer bytes or qualify a
Server deployment or a production-site repair.

## Released behavior

- Operators may provide `--bundled-sources` or
  `CATALOG_SERVICE_BUNDLED_SOURCES`. Version 1 manifests identify an exact catalog
  URL, branch, full commit, local Git bundle, SHA256, and optional required
  template revisions. API callers cannot supply local bundle paths.
- Matching configured catalogs bootstrap from verified bundles before background
  network refresh. Unmatched custom catalogs retain their existing source policy
  and background behavior. There is no remote fallback for a failing matched
  bundle and no automatic pin downgrade.
- Bundle paths remain within the operator's manifest directory. Missing pins,
  incomplete bundles, invalid hashes, path traversal, and symlink escapes fail
  closed. Import uses a staged repository and file-only Git transport.
- Warm caches must match the pin and have clean tracked and untracked content,
  including ignored files and index flags. Polluted caches are repaired from the
  verified bundle. An existing healthy cache is preserved if validation fails.
- Required template revisions are checked against the current database even for
  a nonempty same-commit index. Candidate parsing and revision validation complete
  before transactional replacement; partial parse results cannot replace a good
  bundled index, including when ordinary strict validation is disabled.
- Startup, automatic, and manual refreshes are serialized. Git commands have
  context deadlines, and `GET /healthcheck` verifies only matching bundled sources
  without waiting for custom network refreshes. Failed refreshes retain existing
  Catalog API access and report unavailable bundled readiness until recovery.

Focused validation covered cold and warm offline starts, different bundle and
upstream ref names, tracked/untracked/ignored cache contamination, hidden tracked
changes, old database pins, missing same-commit revisions, rejected hashes,
malformed bundles, missing pins, partial parse failures, source-policy boundaries,
custom-source independence, concurrent refresh, command timeout, flag/environment
precedence, and failed-readiness API access. The successful official release gate
above records reproducible archives, complete tests, scans, and build provenance.
