# Catalog Service v0.20.17 candidate

Status: unpublished verification candidate. The current immutable release remains
[v0.20.16](https://github.com/PastureStack/catalog-service/releases/tag/v0.20.16).
Official Go 1.27.3 build/test/race, reproducible packaging, security and CodeQL
gates are required; matching Engine/Web/Server runtime acceptance is pending.

The Catalog owner supplies bounded safe questions, private material and explicit
answer validation for a selected logical template/version. It shares the normal
question parser, retains primitive/enum/length/numeric/character/date constraints,
and checks the private source digest on every validation. Sensitive/reference
questions need explicit inputs; optional empty input is distinct from omission.
Private defaults never fill missing typed inputs or enter public tool output.

The Engine must block the private `/v1-catalog/ai` namespace at public proxies,
then call the fixed loopback owner only after current actor/project/Key checks.
Loopback alone is not authorization. The Engine retains private encryption,
source/plan CAS, human confirmation and exact graph-bound delegation; Compose
and Stack keep rendering and launch ownership. This component never issues an
account or Key grant and never returns launch payloads to the model.

Ordinary Catalog UI/API, existing data and build/security gates are preserved.
The existing exact Go/dependency/Ubuntu pins are unchanged. CI now retains the
original reproducible archive beside its binary/hash evidence; no binary or
archive hash is guessed, and no old release is rebuilt or overwritten.

Full Catalog forms, unsupported typed inputs, browser/role/weak-model completion
and Server deployment are not established by this source candidate. Rollback
uses the existing numeric component release and operator configuration/data.
