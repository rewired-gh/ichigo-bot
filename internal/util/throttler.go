package util

import "time"

func NewThrottler(freqHz int) (throttler chan struct{}) {
	throttler = make(chan struct{}, 0)
	go func() {
		for {
			throttler <- struct{}{}
			time.Sleep(time.Second / time.Duration(freqHz))
		}
	}()
	return
}
