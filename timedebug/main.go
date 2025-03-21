package timedebug

import (
	"context"
	"log"
	"time"
)

func Timer(name string) context.CancelFunc {
	tStart := time.Now()
	return func() {
		tEnd := time.Now()
		elapsed := tEnd.Sub(tStart)
		log.Println(name, "took", elapsed)
	}
}
