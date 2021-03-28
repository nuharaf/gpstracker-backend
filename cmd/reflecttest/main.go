package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
)

type svcMap struct {
	svcs map[string]svc
	*validator.Validate
}

func NewSvcMap(val *validator.Validate) *svcMap {
	sm := &svcMap{}
	sm.svcs = make(map[string]svc)
	sm.Validate = val
	return sm
}

type svc struct {
	reqType reflect.Type
	handler reflect.Value
}

func (sm *svcMap) Add(tag string, i interface{}) {
	s := svc{}
	s.handler = reflect.ValueOf(i)
	if s.handler.Type().NumIn() == 0 {
		s.reqType = nil
	} else {
		s.reqType = s.handler.Type().In(0).Elem()
	}

	fmt.Printf("%s %s\n", s.handler, s.reqType)
	sm.svcs[tag] = s
}


func (sm *svcMap) Call(tag string, request string) string {
	svc := sm.svcs[tag]
	var res []reflect.Value
	if svc.reqType != nil {
		r := reflect.New(svc.reqType)
		err := json.Unmarshal([]byte(request), r.Interface())

		if err != nil {
			return err.Error()
		}
		err = sm.Struct(r.Interface())
		if err != nil {
			return err.Error()
		}
		res = svc.handler.Call([]reflect.Value{r})
	} else {
		res = svc.handler.Call([]reflect.Value{})
	}

	if !res[1].IsNil() {
		err := res[1].Interface().(error)
		return err.Error()
	}
	s, _ := json.Marshal(res[0].Interface())
	return string(s)
}

var p1 = `{"data":"blablabla"}`

func main() {
	serviceMap := NewSvcMap(validator.New())
	svc := Service{}
	serviceMap.Add("funca", svc.aSvc)
	serviceMap.Add("funcb", svc.bSvc)
	serviceMap.Add("funcc", svc.cSvc)
	s := serviceMap.Call("funcc", p1)
	fmt.Printf("%q", s)

}

type Service struct {
}

type a1Struct struct {
	Data string `json:"data" validate:"required"`
}

type a2Struct struct {
	Status string
}

type b1Struct struct {
	Input string `json:"input" validate:"required"`
}

type b2Struct struct {
	Output string
}

func (s *Service) aSvc(x *a1Struct) (*a2Struct, error) {
	// return &a2Struct{Status: x.Data}, nil
	return nil, errors.New("error bro")
}

func (s *Service) bSvc(x *b1Struct) (*b2Struct, error) {
	return &b2Struct{Output: x.Input}, nil

}

func (s *Service) cSvc() (*b2Struct, error) {
	return &b2Struct{Output: "foobar"}, nil

}
