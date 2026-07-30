@echo off
setlocal
chcp 65001 >nul

net session >nul 2>&1
if %errorlevel% neq 0 (
  echo  Ejecuta este archivo como ADMINISTRADOR ^(clic derecho ^> Ejecutar como administrador^).
  pause
  exit /b 1
)

set "DEST=%ProgramFiles%\InvoicesUp Agent"

echo  Deteniendo y quitando el servicio...
"%DEST%\invoicesup-agent.exe" stop
"%DEST%\invoicesup-agent.exe" uninstall

echo.
echo  Servicio desinstalado. Puedes borrar la carpeta "%DEST%".
echo  La configuracion sigue en %ProgramData%\InvoicesUp (incluye el token).
echo.
pause
