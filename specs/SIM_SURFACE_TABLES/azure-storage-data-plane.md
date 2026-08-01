# Azure Storage — host/path data planes

Surface: `simulators/azure/blob.go` and `simulators/azure/storage_dataplane.go`.

These are the service-native Storage REST data planes advertised from `Microsoft.Storage/storageAccounts` as `{account}.blob.<host>`, `{account}.file.<host>`, `{account}.queue.<host>`, and `{account}.table.<host>`. The simulator also supports the Azurite-compatible path-style forms used by SDKs configured with localhost endpoints.

## Status legend

- ✓ — implemented + tested
- ✗ — missing
- n/a — not a canonical client surface for this protocol in the repo harness

## Blob

| Operation | Verb + path | sim handler | sdk-test | raw-wire test | paged-shape verified | notes |
|---|---|---|---|---|---|---|
| CreateContainer | `PUT /{container}?restype=container` | ✓ `handleCreateContainer` | ✓ `TestStorageSDK_BlobLifecycleAndPagedLists` | ✓ `TestBlobDataPlane_RoundTrip` | n/a | |
| GetContainerProperties | `GET/HEAD /{container}?restype=container` | ✓ `handleGetContainer` | ✓ `TestStorageSDK_BlobContainerAndBlobProperties` | ✓ `TestBlobDataPlane_ListContainersProperties` | n/a | |
| DeleteContainer | `DELETE /{container}?restype=container` | ✓ `handleDeleteContainer` | ✓ `TestStorageSDK_BlobLifecycleAndPagedLists` | ✓ `TestBlobDataPlane_RoundTrip` | n/a | Cascades blobs. |
| ListContainers | `GET /?comp=list` | ✓ `handleListContainers` | ✓ `TestStorageSDK_BlobLifecycleAndPagedLists` | ✓ `TestBlobDataPlane_ListContainersProperties` | ✓ SDK pager | Emits per-container `Properties` with `Last-Modified` and `Etag`. |
| PutBlob | `PUT /{container}/{blob}` | ✓ `handlePutBlob` | ✓ `TestStorageSDK_BlobLifecycleAndPagedLists` | ✓ `TestBlobDataPlane_RoundTrip` | n/a | |
| CopyBlob | `PUT /{container}/{blob}` + `x-ms-copy-source` | ✓ `handleCopyBlob` | ✓ `TestStorageSDK_BlobStartCopyFromURL` | ✓ `TestBlobCopyCLI` | n/a | Copies stored bytes from host-style or Azurite-style path-style source URLs; returns Azure copy ID/status headers. |
| StageBlock | `PUT /{container}/{blob}?comp=block&blockid=...` | ✓ `handleStageBlock` | ✓ `TestStorageSDK_BlobBlockStaging` | ✓ `TestBlobBlockStagingCLI` | n/a | Persists uncommitted block data by base64 block ID. |
| CommitBlockList | `PUT /{container}/{blob}?comp=blocklist` | ✓ `handleCommitBlockList` | ✓ `TestStorageSDK_BlobBlockStaging` | ✓ `TestBlobBlockStagingCLI` | n/a | Commits the requested block IDs in order and materializes the block blob bytes. |
| GetBlockList | `GET /{container}/{blob}?comp=blocklist&blocklisttype=...` | ✓ `handleGetBlockList` | ✓ `TestStorageSDK_BlobBlockStaging` | ✓ `TestBlobBlockStagingCLI` | n/a | Returns committed and/or uncommitted block lists with SDK-compatible XML shape. |
| GetBlob | `GET /{container}/{blob}` | ✓ `handleGetBlob` | ✓ `TestStorageSDK_BlobLifecycleAndPagedLists` | ✓ `TestBlobDataPlane_RoundTrip` | n/a | |
| GetBlobProperties | `HEAD /{container}/{blob}` | ✓ `handleHeadBlob` | ✓ `TestStorageSDK_BlobContainerAndBlobProperties` | ✓ `TestBlobDataPlane_RoundTrip` | n/a | |
| DeleteBlob | `DELETE /{container}/{blob}` | ✓ `handleDeleteBlob` | ✓ `TestStorageSDK_BlobLifecycleAndPagedLists` | ✓ `TestBlobDataPlane_RoundTrip` | n/a | |
| ListBlobs | `GET /{container}?restype=container&comp=list` | ✓ `handleListBlobs` | ✓ `TestStorageSDK_BlobLifecycleAndPagedLists` | ✓ `TestBlobDataPlane_RoundTrip` | ✓ SDK pager | |

## Files

| Operation | Verb + path | sim handler | sdk-test | raw-wire test | paged-shape verified | notes |
|---|---|---|---|---|---|---|
| CreateShare | `PUT /{share}?restype=share` | ✓ `handleFilesCreateShare` | ✓ `TestStorageSDK_FileLifecycleAndPagedLists` | ✓ `TestFilesDataPlane` | n/a | |
| GetShareProperties | `GET/HEAD /{share}?restype=share` | ✓ `handleFilesGetShareProperties` | ✓ `TestStorageSDK_FileShareAndFileProperties` | ✓ `TestFilesDataPlane` | n/a | |
| DeleteShare | `DELETE /{share}?restype=share` | ✓ `handleFilesDeleteShare` | ✓ `TestStorageSDK_FileLifecycleAndPagedLists` | ✓ `TestFilesDataPlane` | n/a | Cascades files. |
| ListShares | `GET /?comp=list` | ✓ `handleFilesListShares` | ✓ `TestStorageSDK_FileLifecycleAndPagedLists` | ✓ `TestFilesShareListCLI` | ✓ SDK pager | Added after the SDK test exposed the missing service-level list. |
| CreateFile / PutRange | `PUT /{share}/{file}` and `PUT /{share}/{file}?comp=range` | ✓ `handleFilesPutFile` | ✓ `TestStorageSDK_FileLifecycleAndPagedLists` | ✓ `TestFilesDataPlane` | n/a | The sim persists the last uploaded range body as file contents for the supported single-range runner flow. |
| GetFile | `GET /{share}/{file}` | ✓ `handleFilesGetFile` | ✓ `TestStorageSDK_FileLifecycleAndPagedLists` | ✓ `TestFilesDataPlane` | n/a | |
| GetFileProperties | `HEAD /{share}/{file}` | ✓ `handleFilesHeadFile` | ✓ `TestStorageSDK_FileShareAndFileProperties` | ✓ `TestFilesDataPlane` | n/a | |
| DeleteFile | `DELETE /{share}/{file}` | ✓ `handleFilesDeleteFile` | ✓ `TestStorageSDK_FileLifecycleAndPagedLists` | ✓ `TestFilesDataPlane` | n/a | |
| ListFilesAndDirectories | `GET /{share}?restype=directory&comp=list` | ✓ `handleFilesListFiles` | ✓ `TestStorageSDK_FileLifecycleAndPagedLists` | ✓ `TestFilesDataPlane` | ✓ SDK pager | |

## Queues

| Operation | Verb + path | sim handler | sdk-test | raw-wire test | paged-shape verified | notes |
|---|---|---|---|---|---|---|
| CreateQueue | `PUT /{queue}` | ✓ `handleQueueCreate` | ✓ `TestStorageSDK_QueueLifecycleAndPagedLists` | ✓ `TestQueuesDataPlane` | n/a | |
| GetQueueMetadata | `GET/HEAD /{queue}` | ✓ `handleQueueGetMetadata` | ✓ `TestStorageSDK_QueueMetadataPeekAndClear` | ✓ `TestQueuesDataPlane` | n/a | |
| DeleteQueue | `DELETE /{queue}` | ✓ `handleQueueDelete` | ✓ `TestStorageSDK_QueueLifecycleAndPagedLists` | ✓ `TestQueuesDataPlane` | n/a | |
| ListQueues | `GET /?comp=list` | ✓ `handleQueuesList` | ✓ `TestStorageSDK_QueueLifecycleAndPagedLists` | ✓ `TestQueueListAndClearMessagesCLI` | ✓ SDK pager | |
| PutMessage | `POST /{queue}/messages` | ✓ `handleQueuePutMessage` | ✓ `TestStorageSDK_QueueLifecycleAndPagedLists` | ✓ `TestQueuesDataPlane` | n/a | |
| PeekMessages | `GET /{queue}/messages?peekonly=true` | ✓ `handleQueuePeekMessages` | ✓ `TestStorageSDK_QueueMetadataPeekAndClear` | ✓ `TestQueuesDataPlane` | n/a | |
| GetMessages | `GET /{queue}/messages` | ✓ `handleQueueGetMessages` | ✓ `TestStorageSDK_QueueLifecycleAndPagedLists` | ✓ `TestQueuesDataPlane` | n/a | |
| DeleteMessage | `DELETE /{queue}/messages/{messageid}?popreceipt=...` | ✓ `handleQueueDeleteMessage` | ✓ `TestStorageSDK_QueueLifecycleAndPagedLists` | ✓ `TestQueuesDataPlane` | n/a | |
| ClearMessages | `DELETE /{queue}/messages` | ✓ `handleQueueClearMessages` | ✓ `TestStorageSDK_QueueMetadataPeekAndClear` | ✓ `TestQueueListAndClearMessagesCLI` | n/a | |

## Tables

| Operation | Verb + path | sim handler | sdk-test | raw-wire test | paged-shape verified | notes |
|---|---|---|---|---|---|---|
| CreateTable | `POST /Tables` | ✓ `handleTableCreate` | ✓ `TestStorageSDK_TableLifecycleAndPagedLists` | ✓ `TestTablesDataPlane` | n/a | |
| GetTable | `GET /Tables('{table}')` | ✓ `handleTableGet` | n/a | ✓ `TestTablesDataPlane` | n/a | The official aztables SDK exposes no Get Table operation (`az storage table exists` also queries via ListTables), so the raw-wire test is the canonical client surface for this path. |
| DeleteTable | `DELETE /Tables('{table}')` | ✓ `handleTableDelete` | ✓ `TestStorageSDK_TableLifecycleAndPagedLists` | ✓ `TestTablesDataPlane` | n/a | Cascades entities. |
| ListTables | `GET /Tables` | ✓ `handleTablesList` | ✓ `TestStorageSDK_TableLifecycleAndPagedLists` | ✓ `TestTableListAndUpsertEntityCLI` | ✓ SDK pager | |
| AddEntity | `POST /{table}` | ✓ `handleEntityInsert` | ✓ `TestStorageSDK_TableLifecycleAndPagedLists` | ✓ `TestTablesDataPlane` | n/a | |
| GetEntity | `GET /{table}(PartitionKey='...',RowKey='...')` | ✓ `handleEntityGet` | ✓ `TestStorageSDK_TableLifecycleAndPagedLists` | ✓ `TestTablesDataPlane` | n/a | |
| UpsertEntity | `PUT/PATCH/MERGE /{table}(PartitionKey='...',RowKey='...')` | ✓ `handleEntityUpsert` | ✓ `TestStorageSDK_TableUpsertEntity` | ✓ `TestTableListAndUpsertEntityCLI` | n/a | Replace mode swaps the entity wholesale; merge mode folds new properties in. |
| DeleteEntity | `DELETE /{table}(PartitionKey='...',RowKey='...')` | ✓ `handleEntityDelete` | ✓ `TestStorageSDK_TableLifecycleAndPagedLists` | ✓ `TestTablesDataPlane` | n/a | |
| QueryEntities | `GET /{table}` / `GET /{table}()` | ✓ `handleEntityQuery` | ✓ `TestStorageSDK_TableLifecycleAndPagedLists` | ✓ `TestTablesDataPlane` | ✓ SDK pager | SDK uses the `/{table}()` form. |

## Follow-up audit note

This table closes the BUG-1183 coverage gap for the current Storage data-plane slice. Every implemented operation is exercised through a canonical client: the official azure-sdk-for-go storage clients in `simulators/azure/sdk-tests/` and either a raw-wire probe or an `az storage` / `az rest` invocation in `simulators/azure/cli-tests/`. Full continuation-token pagination remains a broader Storage fidelity audit item; canonical SDK pager call paths are verified for every supported List operation.
