package msgstore

import "time"

const (
	COMMAND  string = "command"
	RESPONSE string = "response"
)

type MsgStore interface {
	StoreSubmission(fsn string, message string, server_time time.Time) uint64
	FlagSubmission(submission_id uint64, flag string)
	StoreResponse(fsn string, message string, server_time time.Time)
}
