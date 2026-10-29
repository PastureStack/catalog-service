# Catalog Service v0.20.16 candidate

Status: unpublished candidate. The official Go 1.27.3 build, full tests/race,
reproducible archives, source/binary/image security and required CodeQL checks
must pass before publication. No release assets, producer or runtime success
are claimed here.

This release adds a fixed, private loopback metadata handler for the Engine's
typed Catalog reader. It returns project-visible and global template metadata
and parent-bound version selection metadata without source credentials, files,
questions, default answers, readme or arbitrary links.

Safe `templateKey` values preserve logical folder identity beside the display
name, so two folders with identical names/descriptions remain selectable. They
cannot contain an absolute/local path, URL or separator. The builder also
applies Ubuntu's exact checksum-pinned libbrotli1 1.2.0-3ubuntu0.1 update for
USN-8956-1, without adding a vulnerability exception or changing the frozen
package baseline.

Logical template IDs bind the catalog scope, name, sanitized repository origin,
branch/pin and template path. Version IDs also bind version and nullable
revision. Cache rebuilds preserve identity; a changed source or reused cache
row cannot rebind a prior ID. Cursor anchors are re-read under the same
authorized parent/project predicates before paging.

Existing read, refresh and launch APIs, database tables, catalog content,
operator origin policy and build gates retain their behavior. The new handler
trusts only the actual loopback peer and an effective project supplied by the
Engine, with no forwarded credentials or user JWT. Engine owner/Key decisions
and Web opaque references are separate integration responsibilities. A
project-only Key grant does not become a platform Catalog grant.

Catalog create/edit/launch, compatibility resolution, browser/role/weak-model
acceptance and Server deployment are outside this metadata component result.
The previous immutable release remains v0.20.15; rollback uses that numeric
component release and the existing operator configuration/data.
