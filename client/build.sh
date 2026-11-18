#!/usr/bin/env bash
# Сборка C#-клиента (WPF + служба) под Windows из Linux.
# Результат: client/out/ClubShell.exe + ClubAgent.Service.exe (self-contained).
set -e
cd "$(dirname "$0")"

docker run --rm -v "$PWD":/app -w /app mcr.microsoft.com/dotnet/sdk:8.0 sh -c '
  dotnet publish ClubShell/ClubShell.csproj -c Release -r win-x64 --self-contained true -o /app/out &&
  dotnet publish ClubAgent.Service/ClubAgent.Service.csproj -c Release -r win-x64 --self-contained true -o /app/out
'

echo ""
echo "Готово: client/out/ ($(du -sh out | cut -f1))"
echo "  - ClubShell.exe           (оболочка/лаунчер)"
echo "  - ClubAgent.Service.exe   (служба-сторож)"
echo ""
echo "Скопируйте папку out/ на ВМ, рядом положите agent.json и games/."
