# Compatibility Contract

The migration preserves `/v1-catalog` routes, generated catalog schemas, database table names, template and version resource fields, legacy Compose metadata, version suffix ordering, and upgrade-link behavior.

Legacy metadata parsing remains compatible without the archived general-purpose Compose parser. Top-level `catalog` and `.catalog` blocks continue to work, Compose v2 continues to read `services[".catalog"]`, and an explicit top-level block keeps precedence when both forms are present. Unrelated service definitions are not interpreted by Catalog Service.

`platformVersion` is the preferred public query parameter. Historical version-query names, `rancher-compose.yml`, `minimumRancherVersion` and `maximumRancherVersion` schema fields, generated `RancherCompose` and `RancherClient` types, legacy repository fixtures, and vendored `github.com/rancher/*` paths remain only as catalog-data, HTTP, or inherited dependency contracts.

Operator lifecycle messages support `en-US` and `zh-TW`. Catalog content, questions, template readmes, API resources, database data, version strings, and remote errors are not translated.

Source authorization is intentionally stricter than the historical service. Existing public GitHub HTTPS catalogs continue to work. Other HTTPS Git or Helm origins must be added by the service operator to `PASTURESTACK_CATALOG_ALLOWED_EXTERNAL_ORIGINS`; catalog API data cannot self-authorize a destination. Non-loopback plain HTTP, SSH, scp-style, Git helper, and `file://` sources are rejected. Absolute local Git repositories remain available only for isolated tests beneath the platform temporary root; arbitrary installation paths cannot be enabled.

The `0.20.9` candidate replaces MD5 cache directory names with SHA-256 names. The database and API identifiers are unchanged. Existing cache directories are disposable and are rebuilt on the first refresh; catalog records and version history are not migrated or deleted.

Release validation covers `platformVersion` precedence and legacy fallback, both legacy metadata layouts, catalog refresh, database migration, empty default configuration, icon and readme routes, version ordering, upgrade links, malformed repositories, empty-index recovery, outbound-origin and path boundaries, Helm archive limits, SQLite and non-SQLite binaries, and rollback.
