package proofofwork

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_ProofOfWork(t *testing.T) {
	pow := New(8, []byte("john@example!"), 0)
	assert.Equal(t, 8, pow.Bits)
	assert.Equal(t, uint64(0), pow.Proof)
	assert.False(t, pow.HasDoneWork())
	assert.False(t, pow.IsValid())

	pow.Work()
	assert.True(t, pow.HasDoneWork())
	assert.True(t, pow.IsValid())
	assert.Equal(t, uint64(88), pow.Proof)

	pow = New(8, []byte("jane@example!"), 171)
	assert.Equal(t, 8, pow.Bits)
	assert.Equal(t, uint64(171), pow.Proof)
	assert.True(t, pow.HasDoneWork())
	assert.True(t, pow.IsValid())
}