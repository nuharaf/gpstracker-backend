package common

import "time"

type ApiContextKeyType string

type BasicResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message,omitempty"`
}

type StringResponse struct {
	Value string `json:"value"`
}

type UserSessionAtrribute struct {
	RequireChangePassword bool
	Roles                 string
	ValidUntil            time.Time
	SessionId             string
}
