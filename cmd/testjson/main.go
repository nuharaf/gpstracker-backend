package main

import (
	"encoding/json"
	"fmt"
)

type JsonBool struct {
	Value bool
	IsSet bool
}

func (jb *JsonBool) UnmarshalJSON(b []byte) error {
	var v *bool
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v == nil {
		jb.IsSet = false
	} else {
		jb.Value = *v
		jb.IsSet = true
	}
	return nil
}

func (jb *JsonBool) MarshalJSON() ([]byte, error) {
	if jb.IsSet {
		return json.Marshal(jb.Value)
	} else {
		return json.Marshal(nil)
	}

}

type Request struct {
	AllowConnect *JsonBool `json:"allow_connect,omitempty"`
	Store        *JsonBool `json:"store,omitempty"`
}

func main() {
	// str := `{"allow_connect":}`
	// r := Request{}
	// _ = json.Unmarshal([]byte(str), &r)
	// fmt.Printf("%#v", r)
	// t := Dfs{}
	t := Request{}
	t.AllowConnect = &JsonBool{}
	t.AllowConnect.IsSet = false
	t.AllowConnect.Value = false
	t.Store = &JsonBool{}
	t.Store.IsSet = false
	t.Store.Value = true
	d, _ := json.Marshal(&t)
	fmt.Print(string(d))

}
