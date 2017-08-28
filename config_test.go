package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveSet(t *testing.T) {
	set := deriveStringSet("")
	assert.Equal(t, 0, len(set), "Count")

	set = deriveStringSet("ns-0, ns-1")
	assert.Equal(t, 2, len(set), "Count")
	_, exists := set["ns-0"]
	assert.True(t, exists, "Set item 1")
	_, exists = set["ns-1"]
	assert.True(t, exists, "Set item 2")
}
