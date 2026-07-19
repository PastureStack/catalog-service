# Compatibility Contract

The migration preserves `/v1-catalog` routes, generated catalog schemas, database table names, template and version resource fields, legacy Compose metadata, version suffix ordering, and upgrade-link behavior.

`platformVersion` is the preferred public query parameter. Historical version-query names, `rancher-compose.yml`, `minimumRancherVersion` and `maximumRancherVersion` schema fields, generated `RancherCompose` and `RancherClient` types, legacy repository fixtures, and vendored `github.com/rancher/*` paths remain only as catalog-data, HTTP, or inherited dependency contracts.

Operator lifecycle messages support `en-US` and `zh-TW`. Catalog content, questions, template readmes, API resources, database data, version strings, and remote errors are not translated.

Before release, validate `platformVersion` precedence and legacy fallback, catalog refresh, database migration, empty default configuration, icon and readme routes, version ordering, upgrade links, malformed repositories, and rollback.
