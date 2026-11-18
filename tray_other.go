//go:build !windows

package main

import "fmt"

// runTray — на не-Windows просто блокируемся (сервер уже работает в фоне).
func runTray(controller *ServerController, url string) {
	fmt.Printf("Сервер запущен. Админка: %s (Ctrl+C — выход)\n", url)
	select {}
}
