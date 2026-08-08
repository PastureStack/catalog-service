# Security Policy

## Supported state

The latest PastureStack release and the current `main` branch receive security fixes. Historical upstream tags are retained for provenance and are not supported by PastureStack.

## Security boundaries

- Catalog repositories are untrusted input and can contain links, templates, questions, images, and large file trees.
- Outbound HTTP and Git sources require an operator-controlled exact-origin allowlist. Redirects are rechecked, arbitrary Git protocols and credential helpers are disabled, and Git refreshes fetch only the authorized source and branch.
- Reviewed GitHub HTTPS origins are built in. Additional HTTPS origins use `PASTURESTACK_CATALOG_ALLOWED_EXTERNAL_ORIGINS`; plain HTTP is restricted to loopback tests. Local Git sources are test-only, root-confined, and can be enabled only beneath the platform temporary root.
- Repository and Helm cache access is rooted, path-local, and symlink-safe. Cache keys use SHA-256. Helm indexes, icons, archives, expanded content, individual files, and file counts are bounded.
- Catalog readmes are returned as `text/plain` with `nosniff`; untrusted markup is never served as executable HTML.
- Repository URLs, database credentials, API credentials, installation identifiers, and private catalogs are sensitive.
- The service does not read or transmit a platform installation identifier. The deprecated `--track` flag is a no-op retained only for startup compatibility.
- No catalog source is fetched by the default configuration.
- Do not commit database credentials, private repository URLs, tokens, production catalog data, or installation identifiers.
- The archived Compose parser dependency is forbidden by a source gate; legacy metadata is handled by focused local decoding and compatibility tests.
- The disposable build image uses a digest-pinned base, an official Ubuntu snapshot, exact direct APT package versions, and hash-pinned transitive Python build and test dependencies. Raw image-metadata and exported-filesystem reports remain evidence. Every Critical or High OpenVEX statement must exactly match a raw finding; embedded Python-component statements additionally require executable checks that the vulnerable implementation and call path are absent, while build-only kernel-header statements require the existing no-kernel-package assertion.
- Candidate validation builds twice, compares byte-identical archives, records both Go binaries and their linkage, and scans source, product, and build-image scopes before release approval. Build-environment gating uses the exported container filesystem, installed package databases, exact raw-to-VEX matching, and code-presence assertions; product binaries are scanned independently.
- Release artifacts are built only from the current public `main` commit by the manually dispatched GitHub release workflow.

## Reporting

Report suspected vulnerabilities through this repository's private security advisory channel. Do not include credentials, private catalog content, or installation identifiers in a public issue.
