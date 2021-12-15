package main

import (
	"fmt"
	"runtime"
	"time"
)

func main() {
	timer := time.NewTimer(500 * time.Millisecond)
	stopchan := make(chan struct{})
	go func() {
		select {
		case t := <-timer.C:
			fmt.Print(t)
		case <-stopchan:

		}

	}()
	time.Sleep(490 * time.Millisecond)
	if !timer.Stop() {
		fmt.Print("too late")
	} else {
		fmt.Print("timer stopped")
		stopchan <- struct{}{}

	}

	time.Sleep(1 * time.Second)
	fmt.Print(runtime.NumGoroutine())

}
