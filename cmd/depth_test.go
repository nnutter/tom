package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDepthValueSet(t *testing.T) {
	var d depthValue
	require.NoError(t, d.Set("2"))
	assert.Equal(t, depthValue(2), d)
	require.NoError(t, d.Set("function"))
	assert.Equal(t, depthValue(2), d)
	require.NoError(t, d.Set("Sub-Function"))
	assert.Equal(t, depthValue(3), d)
	assert.Equal(t, "3", d.String())
	for _, bad := range []string{"", "4", "-1", "bogus", "func"} {
		assert.Error(t, d.Set(bad))
	}
}

func TestDepthFlagAcceptsAlias(t *testing.T) {
	t.Chdir("..")
	var out bytes.Buffer
	c := NewRootCmd()
	c.SetOut(&out)
	c.SetArgs([]string{"--depth", "file"})
	require.NoError(t, c.Execute())
	assert.Contains(t, out.String(), "package mapgen")
}

func TestDepthCompletions(t *testing.T) {
	choices := depthCompletions()
	assert.Len(t, choices, 8)
	assert.Contains(t, choices, "package\tpackages with all import lines")
	assert.Contains(t, choices, "3\tpositions and function metrics")
}
