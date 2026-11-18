package main

import (
	"log"
	"os"
	"path/filepath"
)

// initLogFile направляет log-вывод в easylock.log рядом с exe
// (нужно на Windows, где консоли нет).
func initLogFile() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(filepath.Dir(exe), "easylock.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}
