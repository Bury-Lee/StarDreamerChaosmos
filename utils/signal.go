package utils

import (
	"os"
	"os/signal"
	"syscall"
)

// WaitSignal 阻塞直到收到中断信号(Interrupt / SIGTERM)。
func WaitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}
