package bysto

import (
	"bytes"
	"errors"
	"fmt"
)

var (
	ErrOutOfBounds  = errors.New("out of bounds")
	ErrDataTooLarge = errors.New("data exceeds section size")
)

type Storage struct {
	sections    [][]byte
	size        int
	maxSections int
}

func NewStorage(sectionCount, sectionSize int) *Storage {
	sections := make([][]byte, 0, sectionCount)
	sections = append(sections, make([]byte, 0, sectionSize))
	return &Storage{
		sections:    sections,
		size:        sectionSize,
		maxSections: sectionCount,
	}
}

// GetRefData returns a view of the stored bytes without copying.
// The capacity is capped so appends to the result cannot write into storage.
// TODO: wrap the result in an immutable type so element writes are also prevented.
func (s *Storage) GetRefData(sectionID, offset, length int) ([]byte, error) {
	if sectionID < 0 || sectionID >= len(s.sections) {
		return nil, fmt.Errorf("section %d: %w", sectionID, ErrOutOfBounds)
	}
	section := s.sections[sectionID]
	end := offset + length
	if offset < 0 || length < 0 || end > len(section) {
		return nil, fmt.Errorf("range [%d:%d] in section %d: %w", offset, end, sectionID, ErrOutOfBounds)
	}
	return section[offset:end:end], nil
}

func (s *Storage) GetCopy(sectionID, offset, length int) ([]byte, error) {
	ref, err := s.GetRefData(sectionID, offset, length)
	if err != nil {
		return nil, err
	}
	return bytes.Clone(ref), nil
}

// Append writes b to the end of the last section, opening a new section if needed.
// It returns the section ID, offset and length of the written data.
func (s *Storage) Append(b []byte) (uint32, uint32, uint32, error) {
	if len(b) > s.size {
		return 0, 0, 0, fmt.Errorf("%d bytes: %w", len(b), ErrDataTooLarge)
	}
	lastID := len(s.sections) - 1
	if len(s.sections[lastID])+len(b) > s.size {
		s.sections = append(s.sections, make([]byte, 0, s.size))
		lastID++
	}
	offset := len(s.sections[lastID])
	s.sections[lastID] = append(s.sections[lastID], b...)

	return uint32(lastID), uint32(offset), uint32(len(b)), nil
}

// Usage returns the percentage of nominal capacity (maxSections * size) occupied by stored bytes.
func (s *Storage) Usage() float64 {
	capacity := s.maxSections * s.size
	if capacity == 0 {
		return 0
	}
	used := 0
	for _, section := range s.sections {
		used += len(section)
	}
	return float64(used) / float64(capacity) * 100
}
