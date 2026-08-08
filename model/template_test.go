package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainsString(t *testing.T) {
	assert.True(t, containsString([]string{"Storage", "Networking"}, "Storage"))
	assert.False(t, containsString([]string{"Storage", "Networking"}, "storage"))
	assert.False(t, containsString(nil, "Storage"))
}
