package lib

import (
	"fmt"
	"os"
	"sync"

	"github.com/rs/zerolog"
)

type logger struct {
	*zerolog.Logger
}

type event struct {
	logger *logger
	format string
	args   []interface{}
}

type void struct{}

var loggerInstance *logger
var once sync.Once

func getInstance() *logger {
	once.Do(func() {
		loggerInstance = _logger()
	})
	return loggerInstance
}

func _logger() *logger {
	var z zerolog.Logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	z = zerolog.New(os.Stdout).With().Timestamp().Logger()
	l := &logger{Logger: &z}
	return l
}

func log(e event) *void {
	e.logger.Info().Msg(fmt.Sprintf(e.format, e.args...))
	return &void{}
}

func Logger() func(string, ...interface{}) *void {
	logger := getInstance()
	return func(format string, args ...interface{}) *void {
		return log(event{logger, format, args})
	}
}
