package main

import (
	"fmt"
	"math/rand"
	"time"

	"nuha.dev/gpstracker/internal/broker"
)

func main() {

	b := broker.NewBroker(&broker.BrokerConfig{Addr: ":5000"})
	go b.Run()
	for i := 0; i < 100; i++ {
		n := i
		go func() {
			c := 0
			for {
				time.Sleep(time.Duration(rand.Int31n(1000)) * time.Millisecond)
				b.Broadcast([]byte(fmt.Sprintf("%d testing %d...", n, c)))
				c++
			}

		}()
	}
	for {
		time.Sleep(time.Second)
	}

}
