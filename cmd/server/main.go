package main

import (
	"github.com/ToxicSozo/GoDraw/internal/wsserver"
	"github.com/sirupsen/logrus"
)

const (
	addr = "0.0.0.0:8080"
)

func main() {
	wsSrv := wsserver.NewWsServer(addr)

	logrus.Info("Запуск WebSocket сервера на ", addr)
	if err := wsSrv.Start(); err != nil {
		logrus.WithError(err).Fatal("Ошибка при запуске сервера")
	}
}
