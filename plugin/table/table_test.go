package table

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"

	"github.com/kolide/osquery-go/gen/osquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ExampleRow struct {
	Text    string   `column:"text"`
	Integer int      `column:"integer"`
	BigInt  *big.Int `column:"big_int"`
	Double  float64  `column:"double"`
}

func TestTablePlugin(t *testing.T) {
	var StatusOK = osquery.ExtensionStatus{Code: 0, Message: "OK"}
	var calledQueryCtx QueryContext
	plugin, err := NewPlugin(
		"mock",
		ExampleRow{},
		GenerateRows(func(ctx context.Context, queryCtx QueryContext) ([]RowDefinition, error) {
			calledQueryCtx = queryCtx
			return []RowDefinition{
				ExampleRow{
					Text:    "hello world",
					Integer: 123,
					BigInt:  big.NewInt(-1234567890),
					Double:  3.14159,
				},
			}, nil
		}))
	require.NoError(t, err)

	// Basic methods
	assert.Equal(t, "table", plugin.RegistryName())
	assert.Equal(t, "mock", plugin.Name())
	assert.Equal(t, StatusOK, plugin.Ping(context.Background()))
	assert.Equal(t, osquery.ExtensionPluginResponse{
		{"id": "column", "name": "text", "type": "TEXT", "op": "0"},
		{"id": "column", "name": "integer", "type": "INTEGER", "op": "0"},
		{"id": "column", "name": "big_int", "type": "BIGINT", "op": "0"},
		{"id": "column", "name": "double", "type": "DOUBLE", "op": "0"},
	}, plugin.Routes())

	// Call explicit columns action
	resp, err := plugin.Call(context.Background(), osquery.ExtensionPluginRequest{"action": "columns"})
	assert.NoError(t, err)
	assert.Equal(t, osquery.ExtensionPluginResponse{
		{"id": "column", "name": "text", "type": "TEXT", "op": "0"},
		{"id": "column", "name": "integer", "type": "INTEGER", "op": "0"},
		{"id": "column", "name": "big_int", "type": "BIGINT", "op": "0"},
		{"id": "column", "name": "double", "type": "DOUBLE", "op": "0"},
	}, resp)

	// Call with good action and context
	resp, err = plugin.Call(context.Background(), osquery.ExtensionPluginRequest{"action": "generate", "context": "{}"})
	assert.Equal(t, QueryContext{map[string]ConstraintList{}}, calledQueryCtx)
	assert.NoError(t, err)
	assert.Equal(t, osquery.ExtensionPluginResponse{
		{
			"text":    "hello world",
			"integer": "123",
			"big_int": "-1234567890",
			"double":  "3.14159",
		},
	}, resp)
}

func TestTablePluginErrors(t *testing.T) {
	var called bool
	plugin, err := NewPlugin(
		"mock",
		ExampleRow{},
		GenerateRows(func(ctx context.Context, queryCtx QueryContext) ([]RowDefinition, error) {
			called = true
			return nil, errors.New("foobar")
		},
		))
	require.NoError(t, err)

	// Call with bad actions
	_, err = plugin.Call(context.Background(), osquery.ExtensionPluginRequest{})
	assert.Error(t, err)
	assert.False(t, called)
	_, err = plugin.Call(context.Background(), osquery.ExtensionPluginRequest{"action": "bad"})
	assert.Error(t, err)
	assert.False(t, called)

	// Call with good action but generate fails
	_, err = plugin.Call(context.Background(), osquery.ExtensionPluginRequest{"action": "generate", "context": "{[]}"})
	assert.Error(t, err)
	assert.False(t, called)

	// Call with good action but generate fails
	_, err = plugin.Call(context.Background(), osquery.ExtensionPluginRequest{"action": "generate", "context": "{}"})
	assert.True(t, called)
	assert.Error(t, err)
	assert.Equal(t, "error generating table: foobar", err.Error())

}

func TestParseConstraintList(t *testing.T) {
	var testCases = []struct {
		json        string
		constraints []Constraint
		shouldErr   bool
	}{
		{
			json:      "bad",
			shouldErr: true,
		},
		{
			json:      `{"foo": "bar"}`,
			shouldErr: true,
		},
		{
			json:        `""`,
			constraints: []Constraint{},
		},
		{
			json: `[{"op":"2","expr":"foo"}]`,
			constraints: []Constraint{
				Constraint{OperatorEquals, "foo"},
			},
		},
		{
			json: `[{"op":"4","expr":"3"},{"op":"16","expr":"4"}]`,
			constraints: []Constraint{
				Constraint{OperatorGreaterThan, "3"},
				Constraint{OperatorLessThan, "4"},
			},
		},
	}

	for _, tt := range testCases {
		t.Run("", func(t *testing.T) {
			constraints, err := parseConstraintList(json.RawMessage(tt.json))
			if tt.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, tt.constraints, constraints)
			}
		})
	}
}

func TestParseQueryContext(t *testing.T) {
	var testCases = []struct {
		json      string
		context   QueryContext
		shouldErr bool
	}{
		{
			json:      "",
			shouldErr: true,
		},
		{
			json: `
{
  "constraints":[
    {
      "name":"big_int",
      "list":"",
      "affinity":"BIGINT"
    },
    {
      "name":"double",
      "list":"",
      "affinity":"DOUBLE"
    },
    {
      "name":"integer",
      "list":"",
      "affinity":"INTEGER"
    },
    {
      "name":"text",
      "list":[
        {
          "op":"2",
          "expr":"foo"
        }
      ],
      "affinity":"TEXT"
    }
  ]
}`,
			context: QueryContext{map[string]ConstraintList{
				"big_int": ConstraintList{ColumnTypeBigInt, []Constraint{}},
				"double":  ConstraintList{ColumnTypeDouble, []Constraint{}},
				"integer": ConstraintList{ColumnTypeInteger, []Constraint{}},
				"text":    ConstraintList{ColumnTypeText, []Constraint{{OperatorEquals, "foo"}}},
			}},
		},
		{
			json: `
{
  "constraints":[
    {
      "name":"big_int",
      "list":"",
      "affinity":"BIGINT"
    },
    {
      "name":"double",
      "list":[
        {
          "op":"32",
          "expr":"3.1"
        }
      ],
      "affinity":"DOUBLE"
    },
    {
      "name":"integer",
      "list":"",
      "affinity":"INTEGER"
    },
    {
      "name":"text",
      "list":[
        {
          "op":"2",
          "expr":"foobar"
        }
      ],
      "affinity":"TEXT"
    }
  ]
}
`,
			context: QueryContext{map[string]ConstraintList{
				"big_int": ConstraintList{ColumnTypeBigInt, []Constraint{}},
				"double":  ConstraintList{ColumnTypeDouble, []Constraint{{OperatorGreaterThanOrEquals, "3.1"}}},
				"integer": ConstraintList{ColumnTypeInteger, []Constraint{}},
				"text":    ConstraintList{ColumnTypeText, []Constraint{{OperatorEquals, "foobar"}}},
			}},
		},
	}
	for _, tt := range testCases {
		t.Run("", func(t *testing.T) {
			context, err := parseQueryContext(tt.json)
			if tt.shouldErr {
				assert.NotNil(t, err)
			} else {
				assert.Equal(t, &tt.context, context)
			}
		})
	}
}

func TestParseVaryingQueryContexts(t *testing.T) {
	var testCases = []struct {
		json            string
		expectedContext *QueryContext
		shouldErr       bool
	}{
		{ // Stringy JSON from osquery version < 3
			`{"constraints":[{"name":"domain","list":[{"op":"2","expr":"kolide.co"}],"affinity":"TEXT"},{"name":"email","list":"","affinity":"TEXT"}]}`,
			&QueryContext{
				Constraints: map[string]ConstraintList{
					"domain": ConstraintList{Affinity: "TEXT", Constraints: []Constraint{Constraint{Operator: OperatorEquals, Expression: "kolide.co"}}},
					"email":  ConstraintList{Affinity: "TEXT", Constraints: []Constraint{}},
				},
			},
			false,
		},
		{ // Strongly typed JSON from osquery version > 3
			`{"constraints":[{"name":"domain","list":[{"op":2,"expr":"kolide.co"}],"affinity":"TEXT"},{"name":"email","list":[],"affinity":"TEXT"}]}`,
			&QueryContext{
				Constraints: map[string]ConstraintList{
					"domain": ConstraintList{Affinity: "TEXT", Constraints: []Constraint{Constraint{Operator: OperatorEquals, Expression: "kolide.co"}}},
					"email":  ConstraintList{Affinity: "TEXT", Constraints: []Constraint{}},
				},
			},
			false,
		},
		{ // Stringy
			`{"constraints":[{"name":"path","list":[{"op":"65","expr":"%foobar"}],"affinity":"TEXT"},{"name":"query","list":[{"op":"2","expr":"kMDItemFSName = \"google*\""}],"affinity":"TEXT"}]}`,
			&QueryContext{
				Constraints: map[string]ConstraintList{
					"path":  ConstraintList{Affinity: "TEXT", Constraints: []Constraint{Constraint{Operator: OperatorLike, Expression: "%foobar"}}},
					"query": ConstraintList{Affinity: "TEXT", Constraints: []Constraint{Constraint{Operator: OperatorEquals, Expression: "kMDItemFSName = \"google*\""}}},
				},
			},
			false,
		},
		{ // Strong
			`{"constraints":[{"name":"path","list":[{"op":65,"expr":"%foobar"}],"affinity":"TEXT"},{"name":"query","list":[{"op":2,"expr":"kMDItemFSName = \"google*\""}],"affinity":"TEXT"}]}`,
			&QueryContext{
				Constraints: map[string]ConstraintList{
					"path":  ConstraintList{Affinity: "TEXT", Constraints: []Constraint{Constraint{Operator: OperatorLike, Expression: "%foobar"}}},
					"query": ConstraintList{Affinity: "TEXT", Constraints: []Constraint{Constraint{Operator: OperatorEquals, Expression: "kMDItemFSName = \"google*\""}}},
				},
			},
			false,
		},

		// Error cases
		{`{bad json}`, nil, true},
		{`{"constraints":[{"name":"foo","list":["bar", "baz"],"affinity":"TEXT"}]`, nil, true},
	}

	for _, tt := range testCases {
		t.Run("", func(t *testing.T) {
			context, err := parseQueryContext(tt.json)
			if tt.shouldErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.expectedContext, context)
		})
	}
}
