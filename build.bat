@echo off
chcp 65001 >nul
echo ========================================
echo   MultiGit - Wails Build Script
echo ========================================

REM Generate ICO from appicon.png
if not exist "appicon.png" (
    echo [ERROR] appicon.png not found
    exit /b 1
)

echo [1/5] Generating appicon.ico...
go run gen_icon.go
if %errorlevel% neq 0 (
    echo [ERROR] Failed to generate appicon.ico
    exit /b 1
)

REM Clean previous build
echo [2/5] Cleaning previous build...
if exist "build" rmdir /s /q "build"
if exist "frontend\wailsjs" rmdir /s /q "frontend\wailsjs"

REM Copy icon for Wails build
echo [3/5] Preparing icon...
if not exist "build\windows" mkdir "build\windows"
copy /Y "appicon.ico" "build\windows\icon.ico" >nul

REM Build with Wails
echo [4/5] Building with Wails...
for /f "tokens=*" %%i in ('git describe --tags --abbrev^=0 2^>nul') do set VERSION=%%i
if not defined VERSION set VERSION=dev
wails build -ldflags="-s -w -X main.version=%VERSION%"
if %errorlevel% neq 0 (
    echo [ERROR] Wails build failed
    exit /b 1
)

echo [5/5] Build complete
echo.
echo Output: build\bin\MultiGit.exe
echo ========================================
