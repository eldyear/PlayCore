package main

import (
	"encoding/hex"
	"errors"
	"net"
	"strings"
)

// parseMAC разбирает MAC в 6 байт (форматы: AA:BB:CC:DD:EE:FF, AA-BB-..., без разделителей).
func parseMAC(s string) ([6]byte, error) {
	var mac [6]byte
	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.TrimSpace(s))
	if len(clean) != 12 {
		return mac, errors.New("некорректный MAC-адрес")
	}
	b, err := hex.DecodeString(clean)
	if err != nil || len(b) != 6 {
		return mac, errors.New("некорректный MAC-адрес")
	}
	copy(mac[:], b)
	return mac, nil
}

// sendWOL отправляет «волшебный пакет» Wake-on-LAN на широковещательный адрес.
func sendWOL(macStr string) error {
	mac, err := parseMAC(macStr)
	if err != nil {
		return err
	}
	// 6 × 0xFF + 16 × MAC
	packet := make([]byte, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac[:])
	}

	// шлём на broadcast порт 9 (и 7 для совместимости)
	for _, port := range []int{9, 7} {
		addr := &net.UDPAddr{IP: net.IPv4bcast, Port: port}
		conn, err := net.DialUDP("udp4", nil, addr)
		if err != nil {
			return err
		}
		_, _ = conn.Write(packet)
		conn.Close()
	}
	return nil
}
