package main

import (
	"encoding/json"
	"fmt"

	"github.com/go-playground/validator/v10"
)

type Login struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password,omitempty" validate:"required"`
}

const LoginJson = `{"username":"nuha","password":"pwd"}`

func main() {
	validate := validator.New()
	login := Login{}
	err := json.Unmarshal([]byte(LoginJson), &login)
	fmt.Println(err)
	err = validate.Struct(login)
	fmt.Println(err)
	fmt.Printf("%+v\n", login)

}
