package test

import (
	"encoding/json"
)

func init() {
	marshalCases = append(marshalCases,
		json.RawMessage("{}"),
		selectedMarshalCase{struct {
			Env   string          `json:"env"`
			Extra json.RawMessage `json:"extra,omitempty"`
		}{
			Env: "jfdk",
		}},
	)
	unmarshalCases = append(unmarshalCases, unmarshalCase{
		ptr:   (*json.RawMessage)(nil),
		input: `[1,2,3]`,
	})
}
