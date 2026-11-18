#!/usr/bin/env bash
# Сборка Windows-версии сервера (easylock.exe):
# - встраивает иконку (playcorelogo.svg -> icon.ico -> .syso);
# - GUI-режим (без окна консоли), работает в трее.
set -e
cd "$(dirname "$0")"

docker run --rm -v "$PWD":/app -w /app golang:1.24-bookworm sh -c '
  set -e
  apt-get update -qq >/dev/null 2>&1
  apt-get install -y -qq imagemagick >/dev/null 2>&1

  # иконка: SVG -> PNG -> ICO (несколько размеров)
  convert -background none /app/playcorelogo.svg -resize 256x256 /app/icon256.png
  convert /app/icon256.png -define icon:auto-resize=16,24,32,48,64,128,256 /app/icon.ico

  # ресурсы Windows (иконка + манифест GUI); удаляем старый .syso, чтобы не подтянулась старая иконка
  rm -f /app/rsrc_windows_amd64.syso
  go install github.com/tc-hib/go-winres@latest >/dev/null 2>&1
  /go/bin/go-winres simply --icon /app/icon.ico --manifest gui --out rsrc --arch amd64

  # сборка: без консоли (-H=windowsgui), с иконкой
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w -H=windowsgui" -o /app/easylock.exe .
'

echo ""
echo "Готово: easylock.exe"
