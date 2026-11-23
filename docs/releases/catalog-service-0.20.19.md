# Catalog Service v0.20.19

This maintenance candidate makes offline catalog bootstrap and index repair native
to Catalog Service. It retains the checksum-pinned Go 1.27.4 compiler and existing
source authorization, rooted cache, catalog identity, and database transaction
boundaries. Publication and matching Server runtime acceptance are separate.

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

Focused validation covers cold and warm offline starts, different bundle and
upstream ref names, tracked/untracked/ignored cache contamination, hidden tracked
changes, old database pins, missing same-commit revisions, rejected hashes,
malformed bundles, missing pins, partial parse failures, source-policy boundaries,
custom-source independence, concurrent refresh, command timeout, flag/environment
precedence, and failed-readiness API access. The official release gate separately
qualifies reproducible archives, complete tests, scans, and build provenance.
