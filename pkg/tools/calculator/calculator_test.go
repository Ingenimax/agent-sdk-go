package calculator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetadata(t *testing.T) {
	c := New()

	assert.Equal(t, "calculator", c.Name())
	assert.Equal(t, "Calculator", c.DisplayName())
	assert.NotEmpty(t, c.Description())
	assert.False(t, c.Internal())

	params := c.Parameters()
	require.Contains(t, params, "expression")
	assert.Equal(t, "string", params["expression"].Type)
	assert.True(t, params["expression"].Required)
}

func TestRun(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "addition", input: "2 + 3", want: "5"},
		{name: "addition with negative second operand", input: "2+-3", want: "-1"},
		{name: "subtraction", input: "10 - 4", want: "6"},
		{name: "subtraction with negative first operand", input: "-5 - 3", want: "-8"},
		{name: "multiplication", input: "4 * 2.5", want: "10"},
		{name: "division", input: "7 / 2", want: "3.5"},
		{name: "exponent", input: "2 ^ 10", want: "1024"},
		{name: "single number", input: "  42  ", want: "42"},
		{name: "decimal number", input: "3.25", want: "3.25"},

		{name: "unsupported expression", input: "abc", wantErr: "unsupported expression: abc"},
		{name: "division by zero", input: "1 / 0", wantErr: "division by zero"},

		{name: "too many additions", input: "1+2+3", wantErr: "invalid addition format"},
		{name: "bad addition first operand", input: "a+1", wantErr: "invalid first operand: a"},
		{name: "bad addition second operand", input: "1+a", wantErr: "invalid second operand: a"},

		{name: "lone negative number", input: "-5", wantErr: "invalid subtraction format with negative"},
		{name: "bad subtraction first operand", input: "a-1", wantErr: "invalid first operand: a"},
		{name: "bad subtraction second operand", input: "1-a", wantErr: "invalid second operand: a"},
		{name: "bad negative first operand", input: "-a-1", wantErr: "invalid first operand: -a"},
		{name: "bad second operand after negative", input: "-1-a", wantErr: "invalid second operand: a"},

		{name: "too many multiplications", input: "1*2*3", wantErr: "invalid multiplication format"},
		{name: "bad multiplication first operand", input: "a*1", wantErr: "invalid first operand: a"},
		{name: "bad multiplication second operand", input: "1*a", wantErr: "invalid second operand: a"},

		{name: "too many divisions", input: "1/2/3", wantErr: "invalid division format"},
		{name: "bad division first operand", input: "a/1", wantErr: "invalid first operand: a"},
		{name: "bad division second operand", input: "1/a", wantErr: "invalid second operand: a"},

		{name: "too many exponents", input: "2^3^2", wantErr: "invalid exponent format"},
		{name: "bad base", input: "a^2", wantErr: "invalid base: a"},
		{name: "bad exponent", input: "2^a", wantErr: "invalid exponent: a"},
	}

	c := New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Run(context.Background(), tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExecute(t *testing.T) {
	c := New()

	t.Run("valid JSON", func(t *testing.T) {
		got, err := c.Execute(context.Background(), `{"expression": "6 * 7"}`)
		require.NoError(t, err)
		assert.Equal(t, "42", got)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := c.Execute(context.Background(), `not json`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse input")
	})

	t.Run("expression error is returned", func(t *testing.T) {
		_, err := c.Execute(context.Background(), `{"expression": "1/0"}`)
		require.EqualError(t, err, "division by zero")
	})
}
