package lib

import (
	"os"
	"sync"

	"github.com/rs/zerolog"
)

type logger struct {
	*zerolog.Logger
}

type event struct {
	logger *logger
	msg string
}

type void struct {}

var loggerInstance *logger
var once sync.Once

func getInstance() *logger {
	once.Do(func () {
		loggerInstance = _logger()
  })
	return loggerInstance
}

func _logger() *logger {
	var z zerolog.Logger
  zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	z = zerolog.New(os.Stdout).With().Timestamp().Logger()
	l := &logger { Logger: &z, }
	return l
}

func log(e event) *void {
	e.logger.Info().Msg(e.msg)
	return &void{}
}

func Logger () func(string) *void {
	logger := getInstance()
	return func (msg string) *void {
		return log(event{ logger, msg })
	}
}