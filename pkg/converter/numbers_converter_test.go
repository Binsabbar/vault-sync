package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type ConvertToInt64TestData struct {
	name        string
	input       interface{}
	expected    int64
	expectError bool
	errorMsg    string
}

func TestConvertInterfaceToInt64(t *testing.T) {
	tests := []ConvertToInt64TestData{
		{
			name:        "float32 input",
			input:       float32(42.0),
			expected:    int64(42),
			expectError: false,
		},
		{
			name:        "float64 input",
			input:       float64(42.0),
			expected:    int64(42),
			expectError: false,
		},
		{
			name:        "int32 input",
			input:       int32(1234567),
			expected:    int64(1234567),
			expectError: false,
		},
		{
			name:        "int64 input",
			input:       int64(123456789),
			expected:    int64(123456789),
			expectError: false,
		},
		{
			name:        "int input",
			input:       int(99),
			expected:    int64(99),
			expectError: false,
		},
		{
			name:        "unsupported string input",
			input:       "not a number",
			expected:    int64(0),
			expectError: true,
			errorMsg:    "unsupported type string for conversion to int64",
		},
		{
			name:        "unsupported bool input",
			input:       true,
			expected:    int64(0),
			expectError: true,
			errorMsg:    "unsupported type bool for conversion to int64",
		},
		{
			name:        "unsupported nil input",
			input:       nil,
			expected:    int64(0),
			expectError: true,
			errorMsg:    "unsupported type <nil> for conversion to int64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertInterfaceToInt64(tt.input)
			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, tt.expected, result)
				assert.EqualError(t, err, tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
