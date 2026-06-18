# Embedded API Parity Matrix

Scope: parity between Chroma capabilities and this repo's embedded (in-process) API.

Legend:
- `done`: implemented in embedded mode
- `in-progress`: currently being implemented
- `planned`: not yet implemented
- `out-of-scope`: intentionally not exposed in current embedded Go surface

## System and Lifecycle

| Capability | Embedded Status | Notes |
|---|---|---|
| Init library | done | `Init` |
| Start embedded from YAML path/string | done | `StartEmbedded` / `NewEmbedded` |
| Close embedded handle | done | `(*Embedded).Close()` |
| Heartbeat | done | `(*Embedded).Heartbeat()` |
| Max batch size | done | `(*Embedded).MaxBatchSize()` |
| Reset | done | `(*Embedded).Reset()` |

## Databases

| Capability | Embedded Status | Notes |
|---|---|---|
| Create database | done | `(*Embedded).CreateDatabase()` |
| List databases | done | `(*Embedded).ListDatabases()` |
| Get database | done | `(*Embedded).GetDatabase()` |
| Delete database | done | `(*Embedded).DeleteDatabase()` |

## Collections

| Capability | Embedded Status | Notes |
|---|---|---|
| Create collection | done | `(*Embedded).CreateCollection()` |
| List collections | done | `(*Embedded).ListCollections()` |
| Get collection | done | `(*Embedded).GetCollection()` |
| Count collections | done | `(*Embedded).CountCollections()` |
| Update collection | done | `(*Embedded).UpdateCollection()` (name and metadata updates) |
| Delete collection | done | `(*Embedded).DeleteCollection()` |
| Fork collection | done | `(*Embedded).ForkCollection()` (may return unimplemented on local Chroma backend) |

## Records and Query

| Capability | Embedded Status | Notes |
|---|---|---|
| Add records | done | `(*Embedded).Add()` |
| Query records | done | `(*Embedded).Query()` |
| Get records | done | `(*Embedded).GetRecords()` |
| Count records | done | `(*Embedded).CountRecords()` |
| Update records | done | `(*Embedded).UpdateRecords()` |
| Upsert records | done | `(*Embedded).UpsertRecords()` |
| Delete records | done | `(*Embedded).DeleteRecords()` and `(*Embedded).DeleteRecordsWithResponse()` (ids and/or filters, optional `limit` with filters) |
| Query/get/delete filters (`where`, `where_document`) | done | Supported in query/get/delete record calls |

## Tenants and Admin

| Capability | Embedded Status | Notes |
|---|---|---|
| Create tenant | done | `(*Embedded).CreateTenant()` |
| Get tenant | done | `(*Embedded).GetTenant()` |
| Update tenant | done | `(*Embedded).UpdateTenant()` (resource visibility may depend on backend) |
| Healthcheck | done | `(*Embedded).Healthcheck()` |
| Indexing status | done | `(*Embedded).IndexingStatus()` (may return unimplemented on local backend) |

## Advanced and Function Management

| Capability | Embedded Status | Notes |
|---|---|---|
| Search API (`search`) | planned | Distinct from nearest-neighbor `query` |
| Get collection by CRN | out-of-scope | Internal/advanced lookup not in current Go surface |
| Attach function | out-of-scope | Upstream `attach_function` not yet bridged |
| Get attached function | out-of-scope | Upstream `get_attached_function` not yet bridged |
| Detach function | out-of-scope | Upstream `detach_function` not yet bridged |
