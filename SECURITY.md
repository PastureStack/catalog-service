# Security Policy

## Supported state

The latest PastureStack release and the current `main` branch receive security fixes. Historical upstream tags are retained for provenance and are not supported by PastureStack.

## Security boundaries

- Catalog repositories are untrusted input and can contain links, templates, questions, images, and large file trees.
- Repository URLs, database credentials, API credentials, installation identifiers, and private catalogs are sensitive.
- No catalog source is fetched by the default configuration.
- Do not commit database credentials, private repository URLs, tokens, production catalog data, or installation identifiers.
- Release artifacts are built only from the current public `main` commit by the manually dispatched GitHub release workflow.

## Reporting

Report suspected vulnerabilities through this repository's private security advisory channel. Do not include credentials, private catalog content, or installation identifiers in a public issue.
