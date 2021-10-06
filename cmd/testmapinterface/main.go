package main

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
)

func main() {
	ft := reflect.ValueOf(getkv)
	rt := ft.Type().In(0).Elem()
	rv := reflect.New(rt)
	ft.Call([]reflect.Value{rv})
	d, err := json.Marshal(rv.Interface())
	if err != nil {
		log.Print(err)
	} else {
		fmt.Print(string(d))
	}
}

type MyStruct struct {
	Key string
	Add map[string]interface{}
}

func getkv(res *[]MyStruct) {
	*res = inner()
}

func inner() []MyStruct {
	structList := make([]MyStruct, 0, 10)
	s1 := MyStruct{}

	s1.Key = "BLABLA"
	s1.Add = make(map[string]interface{})
	s1.Add["test"] = "yes"
	s1.Add["test2"] = 1
	structList = append(structList, s1)
	s2 := MyStruct{}
	s2.Key = "BLABLA2"
	s2.Add = make(map[string]interface{})
	s2.Add["test2"] = "yes"
	s2.Add["test3"] = 1
	structList = append(structList, s2)
	return structList

}
