package main

import (
	"net"
	"net/http"
	"sync"
)

// ServerController — управление HTTP-сервером (старт/стоп) для трея.
type ServerController struct {
	mu      sync.Mutex
	ln      net.Listener
	running bool
	cert    string
	key     string
}

// Start запускает сервер в фоне. Если уже запущен — ничего не делает.
func (c *ServerController) Start(addr string, handler http.Handler, cert, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	c.ln = ln
	c.running = true
	c.cert = cert
	c.key = key
	srv := &http.Server{Handler: handler}
	go func() {
		if cert != "" && key != "" {
			srv.ServeTLS(ln, cert, key)
		} else {
			srv.Serve(ln)
		}
		c.mu.Lock()
		c.running = false
		c.ln = nil
		c.mu.Unlock()
	}()
	return nil
}

// Stop останавливает сервер (закрывает слушатель).
func (c *ServerController) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running && c.ln != nil {
		c.ln.Close()
		c.running = false
		c.ln = nil
	}
}

// IsRunning сообщает, запущен ли сервер.
func (c *ServerController) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}
