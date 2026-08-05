@echo off
setlocal EnableExtensions EnableDelayedExpansion
title SealChat Administrator Password Reset

rem ============================================================
rem SealChat Administrator Password Reset Helper
rem
rem Workflow:
rem   1. Find the SealChat server executable beside this script.
rem   2. Display the database configuration selected by priority:
rem        SEALCHAT_DSN -> config.yaml dbUrl -> .\data\chat.db
rem   3. List platform administrator accounts.
rem   4. Ask the operator to enter one or more usernames.
rem   5. Require explicit confirmation.
rem   6. Reset the selected administrator passwords to 123456.
rem ============================================================

set "SCRIPT_DIR=%~dp0"
set "EXE_PATH=%SCRIPT_DIR%sealchat_server.exe"

echo.
echo ============================================================
echo  SealChat Administrator Password Reset
echo ============================================================
echo.

rem Prefer the standard executable name.
if exist "%EXE_PATH%" goto executable_found

rem If the standard name is absent, find possible SealChat server EXEs.
set "EXE_COUNT=0"
for /f "delims=" %%F in ('dir /b /a-d "%SCRIPT_DIR%*.exe" 2^>nul ^| findstr /i /r "sealchat.*server server.*sealchat"') do (
    set /a EXE_COUNT+=1
    set "EXE_!EXE_COUNT!=%SCRIPT_DIR%%%F"
)

if !EXE_COUNT! equ 0 (
    echo ERROR: No SealChat server executable was found.
    echo Place this script in the same directory as sealchat_server.exe.
    echo.
    pause
    exit /b 1
)

if !EXE_COUNT! equ 1 (
    set "EXE_PATH=!EXE_1!"
    goto executable_found
)

echo Multiple possible SealChat server executables were found:
echo.
for /l %%I in (1,1,!EXE_COUNT!) do (
    for %%F in ("!EXE_%%I!") do echo   [%%I] %%~nxF
)
echo.

:select_executable
set "EXE_SELECTION="
set /p "EXE_SELECTION=Select the executable number: "

if not defined EXE_SELECTION (
    echo Invalid selection.
    goto select_executable
)

set "EXE_PATH="
for /l %%I in (1,1,!EXE_COUNT!) do (
    if "!EXE_SELECTION!"=="%%I" set "EXE_PATH=!EXE_%%I!"
)

if not defined EXE_PATH (
    echo Invalid selection.
    goto select_executable
)

:executable_found
for %%F in ("%EXE_PATH%") do (
    set "EXE_PATH=%%~fF"
    set "EXE_DIR=%%~dpF"
    set "EXE_NAME=%%~nxF"
)

echo Executable:
echo   !EXE_PATH!
echo.

rem Detect and display the database source using SealChat's priority.
set "DB_SOURCE="
set "DB_VALUE="

if defined SEALCHAT_DSN (
    set "DB_SOURCE=SEALCHAT_DSN environment variable"
    set "DB_VALUE=!SEALCHAT_DSN!"
    goto database_found
)

set "CONFIG_PATH=!EXE_DIR!config.yaml"
if exist "!CONFIG_PATH!" (
    set "DB_LINE="
    for /f "usebackq delims=" %%L in (`findstr /r /i /c:"^[ ]*dbUrl[ ]*:" "!CONFIG_PATH!" 2^>nul`) do (
        if not defined DB_LINE set "DB_LINE=%%L"
    )

    if defined DB_LINE (
        for /f "tokens=1,* delims=:" %%A in ("!DB_LINE!") do set "DB_VALUE=%%B"
        for /f "tokens=* delims= " %%A in ("!DB_VALUE!") do set "DB_VALUE=%%A"

        if defined DB_VALUE (
            set "DB_SOURCE=config.yaml dbUrl"
            goto database_found
        )
    )
)

set "DB_SOURCE=Default database"
set "DB_VALUE=!EXE_DIR!data\chat.db"

:database_found
echo Database configuration:
echo   Source: !DB_SOURCE!
echo   Value : !DB_VALUE!
echo.
echo Priority:
echo   SEALCHAT_DSN ^> config.yaml dbUrl ^> .\data\chat.db
echo.

rem Run the CLI from the executable directory so relative paths are correct.
pushd "!EXE_DIR!" >nul
if errorlevel 1 (
    echo ERROR: Unable to enter the executable directory.
    echo.
    pause
    exit /b 1
)

echo Platform administrator accounts:
echo ------------------------------------------------------------
"!EXE_PATH!" --user-secret list
set "LIST_EXIT_CODE=!ERRORLEVEL!"
echo ------------------------------------------------------------
echo.

if not "!LIST_EXIT_CODE!"=="0" (
    echo ERROR: The administrator list command failed.
    echo Exit code: !LIST_EXIT_CODE!
    popd
    echo.
    pause
    exit /b !LIST_EXIT_CODE!
)

echo Enter one or more usernames shown above.
echo Separate multiple usernames with commas, for example: alice,bob
echo.

set "USER_INPUT="
set /p "USER_INPUT=Administrator username(s): "

if not defined USER_INPUT (
    echo.
    echo No username was entered. Operation cancelled.
    popd
    pause
    exit /b 0
)

set "USER_LIST=!USER_INPUT:,= !"
set "USER_COUNT=0"

for %%U in (!USER_LIST!) do (
    set /a USER_COUNT+=1
    set "USER_!USER_COUNT!=%%~U"
)

if !USER_COUNT! equ 0 (
    echo.
    echo No valid username was entered. Operation cancelled.
    popd
    pause
    exit /b 0
)

echo.
echo The following administrator password(s) will be reset to 123456:
for /l %%I in (1,1,!USER_COUNT!) do echo   - !USER_%%I!
echo.
echo This operation uses --admin-only and cannot reset ordinary users.
echo.

set "CONFIRMATION="
set /p "CONFIRMATION=Type RESET to continue: "

if not "!CONFIRMATION!"=="RESET" (
    echo.
    echo Operation cancelled.
    popd
    pause
    exit /b 0
)

set "RESET_ARGS=--user-secret reset --admin-only"
for /l %%I in (1,1,!USER_COUNT!) do (
    set "RESET_ARGS=!RESET_ARGS! --username "!USER_%%I!""
)
set "RESET_ARGS=!RESET_ARGS! --yes"

echo.
echo Resetting password(s)...
"!EXE_PATH!" !RESET_ARGS!
set "RESET_EXIT_CODE=!ERRORLEVEL!"

popd

echo.
if not "!RESET_EXIT_CODE!"=="0" (
    echo ERROR: The password reset command failed.
    echo Exit code: !RESET_EXIT_CODE!
    echo.
    pause
    exit /b !RESET_EXIT_CODE!
)

echo Password reset completed successfully.
echo The temporary password is: 123456
echo Ask each administrator to sign in and change it immediately.
echo.
pause
exit /b 0
