package subscriber

type Subscriber interface {
	Push(rid string, loc []byte) error
	Closed() bool
	Name() string
}
