@echo off
:loop
echo.
echo ============================================
echo Starting message broker service...
echo ============================================
echo.

REM Check if port 4000 is in use and kill the process
echo Checking port 4000...
for /f "tokens=5" %%a in ('netstat -ano ^| findstr :4000 ^| findstr LISTENING') do (
    echo Port 4000 is in use by PID %%a, killing process...
    taskkill /PID %%a /F >nul 2>&1
    timeout /t 1 >nul
)

echo Starting service on port 4000...
go run ./cmd/server

echo.
echo ============================================
echo Service stopped. Press any key to restart or Ctrl+C to exit.
echo ============================================
pause > nul
goto loop