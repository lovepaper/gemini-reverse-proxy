@echo off
REM ============================================================
REM  Gemini 网页协议反代 —— 匿名免 Key 启动脚本
REM  使用前请先确认 Clash / 本地代理(127.0.0.1:7897) 已开启
REM  启动后：
REM    API 地址  http://localhost:8083/v1
REM    API Key  sk-local-test
REM    管理面板  http://localhost:8083/admin  (token: admin123)
REM ============================================================
cd /d "%~dp0"
gemini-web2api-go.exe --port 8083 --proxy http://127.0.0.1:7897 --api-key sk-local-test --admin-token admin123
pause
