# Bounded Catalog metadata

The new `GET /v1-catalog/ai/templates` route is a private same-container
interface for the Engine's typed Catalog metadata resource. It does not change
the existing Catalog endpoints or their public response contracts.

The handler accepts only a real loopback `RemoteAddr` and one effective
`X-API-Project-Id` header. It does not trust forwarded IP or role headers. The
Engine must authenticate the original Cookie or Basic caller, verify current
project access, and evaluate the original Key's **platform Catalog metadata
read** permission before calling this route. A project-only grant is not a
platform grant. The gateway must use its fixed loopback destination without
redirects, forwarded credentials, arbitrary URLs or new user JWTs. This route
must not be published directly or exposed by a general AI proxy.

Inputs are optional `query` (name substring, at most 256 UTF-16 units), `marker`
(`cmc-<opaque cursor>`), or exclusive logical `templateId` for detail. Unknown or
duplicate selectors are rejected. Project/global visibility and exact catalog
parent ownership are applied in SQL before the fixed 51-row lookahead and
50-row response. Public identities are `ctm-<SHA256 logical identity>` of the
Catalog's global or verified wire-project namespace, Catalog name, canonical
repository URL without userinfo/query/fragment, configured branch/pin, and
template folder/base. They exclude cache row IDs, description, versions and
local filesystem paths. Same-name resources in different scopes/repositories
remain distinct. Credential rotation and cache rebuild do not change identity;
source, branch/pin or template location changes do. No row-ID fallback exists.

The short cursor contains a private cache locator, its logical identity and
query/project binding. It is not a grant or persistent resource ID. The source
revalidates the anchor under the same project/global SQL predicate before the
next page; a changed/reused anchor returns `CatalogMetadataCursorStale`.
Detail recomputes identities from at most 10000 authorized rows and matches the
logical hash, without accepting a caller's DB ID. A larger scan fails closed.

The response is `{resourceType: "catalogTemplate", data: [...], nextMarker}`.
Only template name, description, catalog name, base, scope, configured default
version, maintainer, license and `templateKey` are returned. The key is the
indexer's logical single-component `FolderName`, distinct from display name;
local cache `Path`, absolute paths, URLs, separators and control characters are
rejected. This makes same-name/same-description templates distinguishable
without exposing a local path. Descriptions preserve 1024 UTF-16
units. Truncation is explicit and never splits a Unicode scalar. Catalog URLs,
credentials, compose files, labels, questions/default answers, readme, icons
and version links are excluded. Configured default version is metadata; this
does not verify launch compatibility or implement the Catalog create/launch
form.

Version selection uses the same route with `templateId` and `versions=true`;
an optional marker pages the list and an exclusive `versionId` requests detail.
Name queries are not accepted in this mode. The response resource type is
`catalogTemplateVersion`. Each `cvm-<SHA256>` identity binds the logical template,
version string and nullable revision, never a database row. The parent is
re-authorized on every request, and the version SQL joins and rechecks the
current source identity to reject a source replacement between the two reads.
The cursor is bound to the effective project and logical parent template.

Only version, nullable numeric revision, minimum/maximum platform versions and
upgrade-from metadata are projected. Revision values must fit the consumer's
safe integer range. This is selection metadata, not a compatibility verdict,
launch preview or permission to load files, questions, defaults or readme.

Focused checks:

```sh
go test -count=1 ./internal/aimetadata
go test -count=1 ./service -run '^TestAIMetadataRoute'
```

Engine/Web wiring is developed in separate B3 worktrees. Actual role/Key
integration, release gates with the existing Go 1.27.3 toolchain and
browser/weak-model acceptance remain separate work.
