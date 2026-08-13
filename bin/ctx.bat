@echo off
REM ctx - agent context tool (Windows cmd/PowerShell wrapper)
REM
REM Single entry point: the caller does not know which implementation runs.
REM Order: compiled binary, then Python.
REM
REM ASCII ONLY, DELIBERATELY. cmd reads a .bat file in the OEM codepage, not
REM UTF-8; a typographic dash or a Turkish letter in a REM line gets decoded
REM into something cmd tries to EXECUTE ("'context' is not recognized ..." on
REM every single call). Same trap as the .ps1 one already recorded in memory.
REM Turkish belongs in bin/ctx (sh) and the docs, never here.
setlocal
set "CTX_EXE=%~dp0ctx.exe"
set "CTX_PY=%~dp0ctx.py"

if exist "%CTX_EXE%" (
  "%CTX_EXE%" %*
  exit /b %errorlevel%
)

if not exist "%CTX_PY%" (
  echo HATA: ctx bulunamadi ^(ne binary ne ctx.py^). 1>&2
  exit /b 127
)

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

echo HATA: Python bulunamadi ve derlenmis ctx yok. 1>&2
exit /b 127
