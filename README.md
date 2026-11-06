# bysto

Go package that stores a fixed number of byte records in append-only storage sections. Updates never modify existing bytes: a new value is appended and the record is re-pointed at it. Old bytes are reclaimed by a rebuild.

## How it works

The package has two components:

- **`Storage`** keeps data in a list of fixed-size sections (`[][]byte`), up to a limit of `StorageSectionCount` sections. `Append` writes new bytes at the end of the last section and opens a new section when the current one cannot fit the data. It returns the `sectionId`, `offset` and `len` of the written data. Data larger than a section is rejected with `ErrDataTooLarge`. When a new section is needed but the limit is reached, it returns `ErrStorageFull`.
- **`Manager`** owns the records. Each record `i` is described by three arrays: `sectionId[i]`, `offsets[i]` and `lens[i]`. A record with `len == 0` is empty (inactive). The manager also counts, per section, how many records reference it (`refs`).

### Write path (`Set`)

1. The data is appended to `Storage`.
2. The record's `sectionId`, `offset` and `len` are updated to point at the new data.
3. The reference counts are updated: the new section gains a reference, and the previous section loses one.
4. If the write failed because storage is full, the manager rebuilds with the current section count to free unreferenced data, then retries the write. If storage is still full, it rebuilds with 50% more sections and retries again.
5. A compaction runs if either condition holds:
   - a section's reference count dropped to zero (no record uses that section any more), or
   - storage usage reached `RebuildThreshold` percent of its nominal capacity (`StorageSectionCount * StorageSectionSize`).
6. If usage is still at or above `RebuildThreshold` after a compaction, the live data cannot be reduced enough, so the manager rebuilds with 50% more sections.

### Rebuild

A rebuild creates a new `Storage` with the chosen number of sections and copies every active record into it, packed contiguously from the beginning. Each record's `sectionId`, `offset` and `len` are updated to the new location, and the reference counts are recomputed. Old sections are then discarded.

The section count grows by 50%, rounded up (1 → 2, 2 → 3, 8 → 12).

Locking:

- `mu` (`sync.RWMutex`) guards the record arrays, the reference counts and the storage pointer. `Get` takes a read lock; the in-place updates in `Set` and the final swap in rebuild take the write lock.
- `writeMu` serializes writers. Readers are not blocked while a rebuild copies data; they are only blocked for the short swap at the end.

### Read path (`Get`)

`Get` returns a copy of the record's bytes. Reading an index that was never set returns an empty slice. Indexes outside `[0, Records)` return `ErrOutOfBounds`.

## Config

Configuration is read from environment variables with the `APP_` prefix, parsed with `github.com/caarlos0/env/v11` and validated with `github.com/go-playground/validator/v10`.

```go
cfg, err := bysto.LoadConfig()
if err != nil {
    log.Fatal(err)
}
m := bysto.NewStorageManager(cfg)
```

| Variable | Field | Default | Rule | Description |
|---|---|---|---|---|
| `APP_RECORDS_COUNT` | `Records` | required | `> 0` | Number of records the manager holds. |
| `APP_STORAGE_SECTION_COUNT` | `StorageSectionCount` | `8` | `> 0` | Initial section limit. Grows by 50% when live data cannot fit. |
| `APP_STORAGE_SECTION_SIZE` | `StorageSectionSize` | `65536` | `> 0` | Size in bytes of each section. A record cannot be larger than this. |
| `APP_REBUILD_THRESHOLD` | `RebuildThreshold` | `80` | `> 0` and `<= 100` | Storage usage percentage that triggers a rebuild. |

`LoadConfig` returns an error if a required variable is missing or a value breaks its rule. `NewStorageManager` does not validate, so a `Config` built directly in code is not checked.
