package logging

import (
	"log"
	"os"
)

func New() *log.Logger {
	return log.New(os.Stdout, "[tranche] ", log.LstdFlags|log.Lmicroseconds|log.Lshortfile)
}
