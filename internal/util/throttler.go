package util

import "time"

type Throttler struct {
	ResetChannel chan struct{}
	ReadyChannel chan struct{}
}

func NewThrottler(freqHz int) (throttler Throttler) {
	throttler.ResetChannel = make(chan struct{}, 1)
	throttler.ReadyChannel = make(chan struct{}, 1)
	go func() {
		for {
			throttler.ReadyChannel <- struct{}{}
			<-throttler.ResetChannel
			time.Sleep(time.Second / time.Duration(freqHz))
		}
	}()
	return
}
