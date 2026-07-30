@echo off
setlocal
chcp 65001 >nul

net session >nul 2>&1
if %errorlevel% neq 0 (
  echo.
  echo  Este instalador debe ejecutarse como ADMINISTRADOR.
  echo  Cierra esta ventana, haz clic con el boton derecho sobre instalar.bat
  echo  y elige "Ejecutar como administrador".
  echo.
  pause
  exit /b 1
)

echo ============================================
echo   Instalacion del Agente InvoicesUp
echo ============================================
echo.

set "BASEURL="
set /p "BASEURL=URL de InvoicesUp [https://invoicesup.kordino.com]: "
if "%BASEURL%"=="" set "BASEURL=https://invoicesup.kordino.com"

set "TOKEN="
set /p "TOKEN=Token de conector: "
if "%TOKEN%"=="" (
  echo.
  echo  El token es obligatorio. Vuelve a ejecutar el instalador.
  pause
  exit /b 1
)

set "FOLDER="
set /p "FOLDER=Carpeta local para Golden [C:\InvoicesUp\exports]: "
if "%FOLDER%"=="" set "FOLDER=C:\InvoicesUp\exports"

set "DEST=%ProgramFiles%\InvoicesUp Agent"
set "CFGDIR=%ProgramData%\InvoicesUp"

echo.
echo  Copiando el agente...
if not exist "%DEST%" mkdir "%DEST%"
copy /Y "%~dp0invoicesup-agent.exe" "%DEST%\invoicesup-agent.exe" >nul
if not exist "%CFGDIR%" mkdir "%CFGDIR%"
if not exist "%FOLDER%" mkdir "%FOLDER%"

echo  Guardando la configuracion...
rem PowerShell escribe el JSON de forma segura (escapa el token y las barras).
powershell -NoProfile -Command "[System.IO.File]::WriteAllText(\"$env:CFGDIR\config.json\", (@{ base_url=$env:BASEURL; token=$env:TOKEN; folder=$env:FOLDER; poll_seconds=30 } | ConvertTo-Json))"

echo  Registrando el servicio...
"%DEST%\invoicesup-agent.exe" install
echo.
if %errorlevel% equ 0 (
  echo  Listo. El servicio "InvoicesUp Connector Agent" ya esta en marcha.
  echo  Los archivos apareceran en: %FOLDER%
) else (
  echo  Hubo un problema al registrar el servicio.
  echo  Revisa el registro en: %CFGDIR%\agent.log
)
echo.
pause
