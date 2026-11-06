package bysto

import (
	"bytes"
	"sync"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager(records, sectionCount, sectionSize int, threshold float64) *Manager {
	return NewStorageManager(Config{
		RecordsCount:        records,
		StorageSectionCount: sectionCount,
		StorageSectionSize:  sectionSize,
		RebuildThreshold:    threshold,
	})
}

func TestManagerSetAndGet(t *testing.T) {
	m := newTestManager(3, 2, 64, 100)
	payloads := [][]byte{[]byte("first"), []byte("second"), []byte("third")}

	for i, p := range payloads {
		require.NoError(t, m.Set(i, p))
	}
	for i, p := range payloads {
		got, err := m.Get(i)
		require.NoError(t, err)
		assert.Equal(t, p, got)
	}
}

func TestManagerRoundTripRandomPayloads(t *testing.T) {
	const records = 10
	m := newTestManager(records, 2, 32, 80)
	expected := make([][]byte, records)

	for j := 0; j < 100; j++ {
		index := gofakeit.Number(0, records-1)
		payload := []byte(gofakeit.LetterN(uint(gofakeit.Number(1, 30))))
		require.NoError(t, m.Set(index, payload))
		expected[index] = payload
	}
	for i, want := range expected {
		if want == nil {
			continue
		}
		got, err := m.Get(i)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
}

func TestManagerGetUnsetRecordIsEmpty(t *testing.T) {
	m := newTestManager(1, 1, 16, 100)

	got, err := m.Get(0)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestManagerOutOfBounds(t *testing.T) {
	m := newTestManager(2, 1, 16, 100)

	_, err := m.Get(-1)
	assert.ErrorIs(t, err, ErrOutOfBounds)
	_, err = m.Get(2)
	assert.ErrorIs(t, err, ErrOutOfBounds)
	assert.ErrorIs(t, m.Set(-1, []byte("x")), ErrOutOfBounds)
	assert.ErrorIs(t, m.Set(2, []byte("x")), ErrOutOfBounds)
}

func TestManagerSetTooLargeKeepsPreviousValue(t *testing.T) {
	m := newTestManager(1, 1, 4, 100)
	require.NoError(t, m.Set(0, []byte("ok")))

	err := m.Set(0, []byte("too large"))
	assert.ErrorIs(t, err, ErrDataTooLarge)

	got, err := m.Get(0)
	require.NoError(t, err)
	assert.Equal(t, []byte("ok"), got)
}

func TestManagerGetReturnsCopy(t *testing.T) {
	m := newTestManager(1, 1, 16, 100)
	require.NoError(t, m.Set(0, []byte("abc")))

	got, err := m.Get(0)
	require.NoError(t, err)
	got[0] = 'X'

	again, err := m.Get(0)
	require.NoError(t, err)
	assert.Equal(t, []byte("abc"), again)
}

func TestManagerNoRebuildWhileSectionReferencedBelowThreshold(t *testing.T) {
	m := newTestManager(1, 2, 10, 1000)
	require.NoError(t, m.Set(0, []byte("aaaa")))
	require.NoError(t, m.Set(0, []byte("bbbb")))

	assert.Len(t, m.storage.sections, 1)
	assert.Len(t, m.storage.sections[0], 8)
	assert.Equal(t, []int{1}, m.refs)
}

func TestManagerRebuildOnThreshold(t *testing.T) {
	m := newTestManager(1, 1, 100, 50)
	first := bytes.Repeat([]byte("a"), 40)
	second := bytes.Repeat([]byte("b"), 40)

	require.NoError(t, m.Set(0, first))
	assert.Len(t, m.storage.sections[0], 40)

	require.NoError(t, m.Set(0, second))
	assert.Len(t, m.storage.sections, 1)
	assert.Len(t, m.storage.sections[0], 40)
	assert.Equal(t, []int{1}, m.refs)

	got, err := m.Get(0)
	require.NoError(t, err)
	assert.Equal(t, second, got)
}

func TestManagerRebuildWhenSectionUnreferenced(t *testing.T) {
	m := newTestManager(2, 2, 10, 1000)
	require.NoError(t, m.Set(0, bytes.Repeat([]byte("a"), 8)))
	require.NoError(t, m.Set(1, bytes.Repeat([]byte("b"), 8)))
	require.Len(t, m.storage.sections, 2)

	// Record 0 moves to a new section, leaving section 0 unreferenced.
	updated := bytes.Repeat([]byte("c"), 8)
	require.NoError(t, m.Set(0, updated))

	assert.Len(t, m.storage.sections, 2)
	assert.Equal(t, []int{1, 1}, m.refs)

	got0, err := m.Get(0)
	require.NoError(t, err)
	assert.Equal(t, updated, got0)

	got1, err := m.Get(1)
	require.NoError(t, err)
	assert.Equal(t, bytes.Repeat([]byte("b"), 8), got1)
}

func TestManagerConcurrentReadsAndWrites(t *testing.T) {
	const records = 4
	const payloadLen = 16
	const iterations = 200

	m := newTestManager(records, 2, 64, 60)
	for i := 0; i < records; i++ {
		require.NoError(t, m.Set(i, bytes.Repeat([]byte{byte('a' + i)}, payloadLen)))
	}

	var wg sync.WaitGroup
	for i := 0; i < records; i++ {
		wg.Add(2)
		go func(index int) {
			defer wg.Done()
			payload := bytes.Repeat([]byte{byte('a' + index)}, payloadLen)
			for j := 0; j < iterations; j++ {
				assert.NoError(t, m.Set(index, payload))
			}
		}(i)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				got, err := m.Get(index)
				assert.NoError(t, err)
				assert.Equal(t, bytes.Repeat([]byte{byte('a' + index)}, payloadLen), got)
			}
		}(i)
	}
	wg.Wait()
}

func TestIncRefGrowsSlice(t *testing.T) {
	refs := incRef(nil, 2)
	assert.Equal(t, []int{0, 0, 1}, refs)

	refs = incRef(refs, 0)
	refs = incRef(refs, 2)
	assert.Equal(t, []int{1, 0, 2}, refs)
}
