package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type MapStringKeysToSliceTestData struct {
	name     string
	input    map[string]bool
	expected []string
}
type MapIntegerKeysToSliceTestData struct {
	name     string
	input    map[int]bool
	expected []int
}

func TestMapKeysToSlice(t *testing.T) {
	t.Run("string keys", func(t *testing.T) {
		tests := []MapStringKeysToSliceTestData{
			{
				name:     "empty map",
				input:    map[string]bool{},
				expected: []string{},
			},
			{
				name:     "single key",
				input:    map[string]bool{"foo": true},
				expected: []string{"foo"},
			},
			{
				name:     "multiple keys",
				input:    map[string]bool{"a": true, "b": false, "c": true},
				expected: []string{"a", "b", "c"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := MapKeysToSlice(tt.input)
				assert.ElementsMatch(t, result, tt.expected)
			})
		}
	})

	t.Run("int keys", func(t *testing.T) {
		tests := []MapIntegerKeysToSliceTestData{
			{
				name:     "empty map",
				input:    map[int]bool{},
				expected: []int{},
			},
			{
				name:     "single key",
				input:    map[int]bool{1: true},
				expected: []int{1},
			},
			{
				name:     "multiple keys",
				input:    map[int]bool{1: true, 2: false, 3: true},
				expected: []int{1, 2, 3},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := MapKeysToSlice(tt.input)
				assert.ElementsMatch(t, result, tt.expected)
			})
		}
	})
}
