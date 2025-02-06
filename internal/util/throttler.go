package util

import "time"

func NewThrottler(durationMillisecond int) (throttler chan struct{}) {
	throttler = make(chan struct{}, 0)
	go func() {
		for {
			throttler <- struct{}{}
			time.Sleep(time.Millisecond * time.Duration(durationMillisecond))
		}
	}()
	return
}
