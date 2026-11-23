# Catalog Service v0.20.20

Published on 2026-11-23, this maintenance release preserves the upgrade path for
historical library Container Crontab identities through a scoped, read-only
compatibility bridge. It retains the checksum-pinned Go 1.27.4 compiler, native
offline bundles, source authorization, rooted caches, and database transactions.

## Published identity

- Release: [`v0.20.20`](https://github.com/PastureStack/catalog-service/releases/tag/v0.20.20).
- Immutable producer: [`bced4b96b0c6a0fe7d3db044f5c30a5b33d01172`](https://github.com/PastureStack/catalog-service/commit/bced4b96b0c6a0fe7d3db044f5c30a5b33d01172), with a verified GitHub signature.
- Compiler: Go 1.27.4; published product qualification covers Linux/amd64.
- [Candidate security gate 45131943562](https://github.com/PastureStack/catalog-service/actions/runs/45131943562): success at `c29053cb8a473099a3594f646ae9d5ae73d803a5`.
- [Required CodeQL 45131944132](https://github.com/PastureStack/catalog-service/actions/runs/45131944132): actions, Go, and Python checks passed before merge.
- [Release workflow 45133228172](https://github.com/PastureStack/catalog-service/actions/runs/45133228172): success for the signed producer above.

The candidate and signed producer have the same Git tree
`90c8f2c2a801f0bfd98f1e3f3b7e10001aedb483`. The release workflow independently
verified the producer revision embedded in both binaries, clean source metadata,
exact version output, and byte-identical archives from two clean builds. The
published archive and license were downloaded and checked against their official
checksum assets; the two binary hashes below were computed from that archive.

| Artifact | SHA-256 |
| --- | --- |
| [`catalog-service-0.20.20.tar.xz`](https://github.com/PastureStack/catalog-service/releases/download/v0.20.20/catalog-service-0.20.20.tar.xz) | `6bce527300205aeb0d589b67f4b38e037be3bdb8e6da5ea04f7357d287eedf6d` |
| `catalog-service` inside the archive | `acf89922925f62ce2ae9333636568f50c7fa6a2dee8b843e68dd8be43d8342b4` |
| `catalog-service-sqlite` inside the archive | `c7e6c53d2382d7955edaf8b77869d6df3aa9e7cab2b7bf7646080791b3169e16` |
| [`catalog-service-0.20.20-LICENSE.txt`](https://github.com/PastureStack/catalog-service/releases/download/v0.20.20/catalog-service-0.20.20-LICENSE.txt) | `0d542e0c8804e39aa7f37eb00da5a762149dc682d7829451287e11b938e94594` |

The archive contains exactly those two binaries; the license is a separate
release asset and matches the preserved root LICENSE. A later documentation
commit does not change these producer bytes.

## Released behavior

- Historical `library:infra*container-crontab` read routes can resolve the reviewed
  first-party compatibility template when the original is absent. Requested IDs,
  exact historical revisions, and ordinary upgrade links are preserved; unknown
  revisions return 404.
- Configured custom library sources keep precedence and their own identity.
  Compatibility-only templates remain hidden from new-install lists.
- The matching template retains its service key and answer names. Existing Stack
  upgrade, completion, and rollback actions own persisted changes. Catalog GET
  requests do not create a catalog, rewrite a Stack identity, or start a scheduler.
- The source version gate now requires a numeric version core before recognizing
  a forbidden product suffix. The legitimate compatibility metadata key remains
  accepted; isolated regression fixtures still reject all eight covered product
  suffix forms without exempting any source file.

## Validation boundary

The candidate gate passed complete source and integration tests, formatting,
reproducible packaging, source policy checks, and the targeted gate regressions.
The product vulnerability and secret scans were empty. Dapper's raw findings
remain recorded as 3 Critical and 51 High; the existing exact header-only VEX
qualification leaves zero applicable Critical or High findings. The release
workflow then tested and packaged the signed producer and verified both binaries'
native version and build metadata.

Server assembly and deployment, scheduler upgrades and rollback, operator
settings, runtime data retention, and the complete UI and permission lifecycle
remain separate acceptance results. This component publication does not qualify
a production-site repair or overall runtime PASS.
