package main

type TestStruct struct {
	Key *string `json:"key" validate:"required"`
}

func main() {
	x := TestStruct{Key: nil}

}
