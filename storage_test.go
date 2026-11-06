package bysto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageAppendReturnsSectionOffsetAndLength(t *testing.T) {
	s := NewStorage(2, 10)

	sid, off, ln, err := s.Append([]byte("abc"))
	require.NoError(t, err)
	assert.Equal(t, uint32(0), sid)
	assert.Equal(t, uint32(0), off)
	assert.Equal(t, uint32(3), ln)

	sid, off, ln, err = s.Append([]byte("defg"))
	require.NoError(t, err)
	assert.Equal(t, uint32(0), sid)
	assert.Equal(t, uint32(3), off)
	assert.Equal(t, uint32(4), ln)
}

func TestStorageAppendOpensNewSectionWhenFull(t *testing.T) {
	s := NewStorage(2, 10)
	_, _, _, err := s.Append([]byte("aaaaaa"))
	require.NoError(t, err)

	sid, off, ln, err := s.Append([]byte("bbbbbb"))
	require.NoError(t, err)
	assert.Equal(t, uint32(1), sid)
	assert.Equal(t, uint32(0), off)
	assert.Equal(t, uint32(6), ln)
	assert.Len(t, s.sections, 2)
}

func TestStorageAppendRejectsDataLargerThanSection(t *testing.T) {
	s := NewStorage(1, 4)

	_, _, _, err := s.Append([]byte("12345"))
	assert.ErrorIs(t, err, ErrDataTooLarge)
	assert.Empty(t, s.sections[0])
}

func TestStorageGetRefDataOutOfBounds(t *testing.T) {
	s := NewStorage(1, 10)
	_, _, _, err := s.Append([]byte("hello"))
	require.NoError(t, err)

	tests := []struct {
		name      string
		sectionID int
		offset    int
		length    int
	}{
		{"negative section", -1, 0, 1},
		{"section past end", 1, 0, 1},
		{"negative offset", 0, -1, 1},
		{"negative length", 0, 0, -1},
		{"range past section end", 0, 3, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.GetRefData(tt.sectionID, tt.offset, tt.length)
			assert.ErrorIs(t, err, ErrOutOfBounds)
		})
	}
}

func TestStorageGetRefDataCapsCapacity(t *testing.T) {
	s := NewStorage(1, 10)
	_, _, _, err := s.Append([]byte("hello"))
	require.NoError(t, err)

	ref, err := s.GetRefData(0, 1, 3)
	require.NoError(t, err)
	assert.Equal(t, []byte("ell"), ref)
	assert.Equal(t, len(ref), cap(ref))
}

func TestStorageGetCopyIsIndependent(t *testing.T) {
	s := NewStorage(1, 10)
	_, _, _, err := s.Append([]byte("hello"))
	require.NoError(t, err)

	cp, err := s.GetCopy(0, 0, 5)
	require.NoError(t, err)
	cp[0] = 'X'

	ref, err := s.GetRefData(0, 0, 5)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), ref)
}

func TestStorageUsage(t *testing.T) {
	s := NewStorage(2, 10)
	assert.InDelta(t, 0.0, s.Usage(), 0.0001)

	_, _, _, err := s.Append(make([]byte, 5))
	require.NoError(t, err)
	assert.InDelta(t, 25.0, s.Usage(), 0.0001)
}

func TestStorageUsageWithZeroCapacity(t *testing.T) {
	s := NewStorage(0, 0)
	assert.InDelta(t, 0.0, s.Usage(), 0.0001)
}
