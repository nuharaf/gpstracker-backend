package sublist

import (
	"errors"
	"testing"
)

type mockSub struct {
	err    bool
	closed bool
}

func (m *mockSub) Push(sender string, d []byte) error {
	if m.err {
		return errors.New("subscriber closed")
	} else {
		return nil
	}

}

func (m *mockSub) Closed() bool {
	return m.closed
}

func (m *mockSub) Name() string {
	return "mocksub"
}

func TestNoPrune(t *testing.T) {
	subs := Sublist{}
	subs.list = make([]subflag, 10)
	for i := range subs.list {
		subs.list[i].err = nil
		subs.list[i].sub = &mockSub{}
	}
	subs.Prune()
	if len(subs.list) != 10 {
		t.Error()
	}
}

func TestPrune1(t *testing.T) {
	subs := Sublist{}
	subs.list = make([]subflag, 10)
	for i := range subs.list {
		subs.list[i].err = nil
		subs.list[i].sub = &mockSub{}
	}
	subs.list[8].sub.(*mockSub).closed = true
	subs.Prune()
	if len(subs.list) != 9 {
		t.Error()
	}
}

func TestPrune2(t *testing.T) {
	subs := Sublist{}
	subs.list = make([]subflag, 10)
	for i := range subs.list {
		subs.list[i].err = nil
		subs.list[i].sub = &mockSub{}
	}
	subs.list[4].sub.(*mockSub).closed = true
	subs.list[8].sub.(*mockSub).closed = true
	subs.list[9].sub.(*mockSub).closed = true
	subs.Prune()
	if len(subs.list) != 7 {
		t.Error()
	}
}

func BenchmarkSend(b *testing.B) {
	p := make([]byte, 100)
	subs := Sublist{}
	subs.list = make([]subflag, 100)
	for i := range subs.list {
		subs.list[i].err = nil
		subs.list[i].sub = &mockSub{}
	}
	b.ResetTimer()
	subs.Send("mocksender", p)
}

func BenchmarkSend50(b *testing.B) {
	p := make([]byte, 100)
	subs := Sublist{}
	subs.list = make([]subflag, 50)
	for i := range subs.list {
		subs.list[i].err = nil
		subs.list[i].sub = &mockSub{}
	}
	b.ResetTimer()
	subs.Send("mocksender", p)
}

// func BenchmarkSendPrune(b *testing.B) {
// 	p := make([]byte, 100)
// 	subs := Sublist{}
// 	subs.list = make([]subflag, 100)
// 	for i := range subs.list {
// 		subs.list[i].err = nil
// 		subs.list[i].sub = &mockSub{}
// 	}
// 	b.ResetTimer()
// 	subs.SendPrune("mocksender", p)
// }
