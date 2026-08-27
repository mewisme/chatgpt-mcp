@echo off
setlocal
rmdir /s /q internal\web\dist
mkdir internal\web\dist
xcopy /e /i /y web\dist\* internal\web\dist\
