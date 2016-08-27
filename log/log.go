package log

import (
	"fmt"
	"io"
)

var log io.Writer

func Init(logWriter io.Writer) {
	log = logWriter
}

func Print(a ...interface{})                 { logToFile(fmt.Sprint(a...)) }
func Printf(format string, a ...interface{}) { logToFile(fmt.Sprintf(format, a...)) }
func Println(a ...interface{})               { logToFile(fmt.Sprintln(a...)) }

func logToFile(msg string) {
	fmt.Print(msg)

	if log != nil {
		log.Write([]byte(msg))
	}
}

func Fatal(a ...interface{}) {
	msg := fmt.Sprint(a...)
	fail(msg)
}

func Fatalf(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fail(msg)
}

func fail(msg string) {
	Println("fatal error:", msg)
	panic(msg)
}
