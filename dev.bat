@echo off
:loop
echo Building and starting message broker service...
go run ./cmd/server
echo Service stopped. Press any key to restart or Ctrl+C to exit.
pause > nul
goto loop