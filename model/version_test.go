package model

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionsSortMalformedLegacyValuesBeforeSemanticVersions(t *testing.T) {
	versions := Versions{
		{Version: "2.0.0"},
		{Version: "not-semver"},
		{Version: "1.0.0"},
	}
	sort.Sort(versions)
	assert.Equal(t, "not-semver", versions[0].Version)
	assert.Equal(t, "1.0.0", versions[1].Version)
	assert.Equal(t, "2.0.0", versions[2].Version)
}
