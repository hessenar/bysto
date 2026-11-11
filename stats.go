package bysto

type Stats struct {
	Records       int // number of records the manager holds (len(lens))
	Entries       int // records with len > 0
	MinLen        int // shortest entry in bytes, 0 when there are no entries
	MaxLen        int // longest entry in bytes, 0 when there are no entries
	LiveBytes     int // sum of lens of all entries
	UsedBytes     int // bytes occupied in storage sections, including unreferenced bytes
	CapacityBytes int // maxSections * sectionSize, the nominal capacity
	Sections      int // sections currently allocated
}

func (m *Manager) Stats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	st := Stats{
		Records:       len(m.lens),
		CapacityBytes: m.storage.maxSections * m.storage.size,
		Sections:      len(m.storage.sections),
	}
	for _, sec := range m.storage.sections {
		st.UsedBytes += len(sec)
	}
	for _, l := range m.lens {
		if l == 0 {
			continue
		}
		st.Entries++
		st.LiveBytes += int(l)
		if st.MinLen == 0 || int(l) < st.MinLen {
			st.MinLen = int(l)
		}
		if int(l) > st.MaxLen {
			st.MaxLen = int(l)
		}
	}
	return st
}
