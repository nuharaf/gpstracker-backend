package subscriber

type Subscriber interface {
	Push(tid uint64, loc []byte) bool
}
