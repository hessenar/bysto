package bysto

import (
	"errors"
	"fmt"
	"sync"
)

type Manager struct {
	mu      sync.RWMutex // guards records, refs and storage
	writeMu sync.Mutex   // serializes writes and rebuilds

	sectionId []uint32
	offsets   []uint32
	lens      []uint32
	refs      []int // number of active records referencing each section, indexed by section ID

	storage          *Storage
	rebuildThreshold float64 // storage usage percentage that triggers a rebuild
}

func NewStorageManager(cfg Config) *Manager {
	return &Manager{
		sectionId: make([]uint32, cfg.RecordsCount),
		offsets:   make([]uint32, cfg.RecordsCount),
		lens:      make([]uint32, cfg.RecordsCount),
		refs:      make([]int, 0, cfg.StorageSectionCount),

		storage:          NewStorage(cfg.StorageSectionCount, cfg.StorageSectionSize),
		rebuildThreshold: cfg.RebuildThreshold,
	}
}

func (m *Manager) Get(index int) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if index < 0 || index >= len(m.lens) {
		return nil, fmt.Errorf("record %d: %w", index, ErrOutOfBounds)
	}
	return m.storage.GetCopy(int(m.sectionId[index]), int(m.offsets[index]), int(m.lens[index]))
}

// Set stores data as a new entry in storage and points the record at it.
// When storage is full, it rebuilds to free unreferenced data and retries; if that
// does not free enough room, it rebuilds with 50% more sections.
func (m *Manager) Set(index int, data []byte) error {
	if index < 0 || index >= len(m.lens) {
		return fmt.Errorf("record %d: %w", index, ErrOutOfBounds)
	}

	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	orphaned, err := m.write(index, data)
	if errors.Is(err, ErrStorageFull) {
		if err = m.rebuild(m.storage.maxSections); err != nil {
			return err
		}
		if orphaned, err = m.write(index, data); errors.Is(err, ErrStorageFull) {
			if err = m.rebuild(grownSectionCount(m.storage.maxSections)); err != nil {
				return err
			}
			orphaned, err = m.write(index, data)
		}
	}
	if err != nil {
		return fmt.Errorf("record %d: %w", index, err)
	}

	if orphaned || m.storage.Usage() >= m.rebuildThreshold {
		return m.compact()
	}
	return nil
}

// write appends data and points the record at it. It reports whether the record's
// previous section was left without any referencing record.
func (m *Manager) write(index int, data []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sid, offset, length, err := m.storage.Append(data)
	if err != nil {
		return false, err
	}

	oldSid, oldLen := m.sectionId[index], m.lens[index]
	m.sectionId[index], m.offsets[index], m.lens[index] = sid, offset, length
	if length > 0 {
		m.refs = incRef(m.refs, sid)
	}
	if oldLen == 0 {
		return false, nil
	}
	m.refs[oldSid]--
	return m.refs[oldSid] == 0, nil
}

// compact rebuilds with the current section count. If the live data still reaches
// the threshold, it rebuilds with 50% more sections.
func (m *Manager) compact() error {
	if err := m.rebuild(m.storage.maxSections); err != nil {
		return err
	}
	if m.storage.Usage() >= m.rebuildThreshold {
		return m.rebuild(grownSectionCount(m.storage.maxSections))
	}
	return nil
}

// rebuild copies all active records into a new storage with sectionCount sections,
// packed contiguously from the beginning. The caller must hold writeMu. Readers keep
// using the current storage during the copy and are only blocked for the final swap.
func (m *Manager) rebuild(sectionCount int) error {
	next := NewStorage(sectionCount, m.storage.size)
	n := len(m.lens)
	sectionIDs := make([]uint32, n)
	offsets := make([]uint32, n)
	lens := make([]uint32, n)
	var refs []int

	for i := 0; i < n; i++ {
		if m.lens[i] == 0 {
			continue
		}
		data, err := m.storage.GetRefData(int(m.sectionId[i]), int(m.offsets[i]), int(m.lens[i]))
		if err != nil {
			return fmt.Errorf("rebuild record %d: %w", i, err)
		}
		sid, offset, length, err := next.Append(data)
		if err != nil {
			return fmt.Errorf("rebuild record %d: %w", i, err)
		}
		sectionIDs[i], offsets[i], lens[i] = sid, offset, length
		refs = incRef(refs, sid)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.storage = next
	m.sectionId = sectionIDs
	m.offsets = offsets
	m.lens = lens
	m.refs = refs
	return nil
}

// grownSectionCount returns the section count 50% above n, rounded up.
func grownSectionCount(n int) int {
	return n + (n+1)/2
}

// incRef increments the reference count of section sid, growing refs when a new section appears.
func incRef(refs []int, sid uint32) []int {
	for int(sid) >= len(refs) {
		refs = append(refs, 0)
	}
	refs[sid]++
	return refs
}
