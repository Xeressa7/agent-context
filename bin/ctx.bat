@echo off
REM ctx — agent context aracı (Windows cmd/PowerShell sarmalayıcısı)
REM Python yorumlayıcısını bulup ctx.py'yi çağırır; tüm argümanlar aynen aktarılır.
setlocal
set "CTX_PY=%~dp0ctx.py"

where py >nul 2>nul
if %errorlevel%==0 (
  py -3 "%CTX_PY%" %*
  exit /b %errorlevel%
)

where python >nul 2>nul
if %errorlevel%==0 (
  python "%CTX_PY%" %*
  exit /b %errorlevel%
)

echo HATA: Python bulunamadi (py ya da python PATH'te olmali). 1>&2
exit /b 127
