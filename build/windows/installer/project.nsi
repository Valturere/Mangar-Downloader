# Mangar Downloader: NSIS owns replacement/rollback; helpers own only guards/registry.
Unicode true
RequestExecutionLevel user
ManifestDPIAware true
SetCompressor /SOLID lzma
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"
!include "Sections.nsh"
!include "WinVer.nsh"
!include "x64.nsh"
!ifndef APP_VERSION
!error "Use scripts/build-installer.ps1"
!endif
!define PRODUCT_NAME "Mangar Downloader"
!define PRODUCT_ID "YTDLPGUIMangar"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\ValturereMangar"
!define STORE_URL "https://chromewebstore.google.com/detail/mangar/paimennadokhgjdpgilfcebameolopbm"
Name "${PRODUCT_NAME}"
OutFile "${INSTALLER_OUTPUT}"
InstallDir "$LOCALAPPDATA\Programs\Mangar"
VIProductVersion "${APP_VERSION}.0"
VIFileVersion "${APP_VERSION}.0"
VIAddVersionKey /LANG=1049 "ProductName" "${PRODUCT_NAME}"
VIAddVersionKey /LANG=1049 "CompanyName" "Valturere"
VIAddVersionKey /LANG=1049 "FileDescription" "Установщик Mangar Downloader"
VIAddVersionKey /LANG=1049 "FileVersion" "${APP_VERSION}.0"
VIAddVersionKey /LANG=1049 "ProductVersion" "${APP_VERSION}"
VIAddVersionKey /LANG=1049 "LegalCopyright" "© 2026 Valturere"
!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION LaunchApp
!define MUI_FINISHPAGE_RUN_TEXT "Открыть Mangar Downloader"
!define MUI_FINISHPAGE_RUN_NOTCHECKED
!define MUI_FINISHPAGE_SHOWREADME
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION OpenStore
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Открыть страницу расширения Chrome Web Store"
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Russian"
Var ParentPID
Var GuardState
Var Counter

!macro WriteRegistration VERSION
  WriteRegStr HKCU "${UNINSTALL_KEY}" "ProductID" "${PRODUCT_ID}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "Valturere"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\YTDLP-GUI-Mangar.exe"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '$\"$INSTDIR\uninstall.exe$\"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '$\"$INSTDIR\uninstall.exe$\" /S'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
!macroend
!macro RestoreRegistration
  ReadINIStr $R2 "$INSTDIR\transaction.ini" "registry" "version"
  ${If} $R2 != ""
    !insertmacro WriteRegistration "$R2"
  ${Else}
    ReadRegStr $R0 HKCU "${UNINSTALL_KEY}" "ProductID"
    ReadRegStr $R1 HKCU "${UNINSTALL_KEY}" "InstallLocation"
    ${If} $R0 == "${PRODUCT_ID}"
    ${AndIf} $R1 == "$INSTDIR"
      DeleteRegKey HKCU "${UNINSTALL_KEY}"
    ${EndIf}
  ${EndIf}
!macroend

!macro Owned OP
!insertmacro ${OP} "YTDLP-GUI-Mangar.exe"
!insertmacro ${OP} "mangar-browser-host.exe"
!insertmacro ${OP} "mangar-updater.exe"
!insertmacro ${OP} "installation.json"
!insertmacro ${OP} "ru.mangar.browser.json"
!insertmacro ${OP} "uninstall.exe"
!macroend
!macro Restore FILE
ReadINIStr $R0 "$INSTDIR\transaction.ini" "previous" "${FILE}"
${If} ${FileExists} "$INSTDIR\.rollback\${FILE}"
  ClearErrors
  Delete "$INSTDIR\${FILE}"
  Rename "$INSTDIR\.rollback\${FILE}" "$INSTDIR\${FILE}"
  ${If} ${Errors}
    Goto failed
  ${EndIf}
${ElseIf} $R0 == "0"
  Delete "$INSTDIR\${FILE}"
${EndIf}
Delete "$INSTDIR\.new\${FILE}"
!macroend
!macro Backup FILE
ClearErrors
${If} ${FileExists} "$INSTDIR\${FILE}"
  WriteINIStr "$INSTDIR\transaction.ini" "previous" "${FILE}" "1"
  ${If} ${Errors}
    Goto rollback
  ${EndIf}
  ClearErrors
  Rename "$INSTDIR\${FILE}" "$INSTDIR\.rollback\${FILE}"
  ${If} ${Errors}
    Goto rollback
  ${EndIf}
${Else}
  WriteINIStr "$INSTDIR\transaction.ini" "previous" "${FILE}" "0"
  ${If} ${Errors}
    Goto rollback
  ${EndIf}
${EndIf}
!macroend
!macro CommitFile FILE
${If} ${FileExists} "$INSTDIR\.new\${FILE}"
  ClearErrors
  Rename "$INSTDIR\.new\${FILE}" "$INSTDIR\${FILE}"
  ${If} ${Errors}
    Goto rollback
  ${EndIf}
${EndIf}
!macroend
!macro Cleanup FILE
Delete "$INSTDIR\.new\${FILE}"
Delete "$INSTDIR\.rollback\${FILE}"
!macroend
!macro DeleteOwned FILE
Delete "$INSTDIR\${FILE}"
!macroend

Function .onInit
  SetShellVarContext current
  SetRegView 64
  StrCpy $INSTDIR "$LOCALAPPDATA\Programs\Mangar"
  ${IfNot} ${AtLeastWin10}
    SetErrorLevel 64
    Quit
  ${EndIf}
  ${IfNot} ${IsNativeAMD64}
    SetErrorLevel 65
    Quit
  ${EndIf}
FunctionEnd

!macro StartGuard PREFIX
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=mangar-updater.exe "${PAYLOAD_DIR}\mangar-updater.exe"
  System::Call 'kernel32::GetCurrentProcessId() i.r0'
  StrCpy $ParentPID $0
  Exec '"$PLUGINSDIR\mangar-updater.exe" --guard $ParentPID "$PLUGINSDIR\guard.ini"'
  StrCpy $Counter 0
  ${Do}
    ReadINIStr $GuardState "$PLUGINSDIR\guard.ini" "guard" "status"
    ${If} $GuardState == "ready"
      ${ExitDo}
    ${EndIf}
    ${If} $GuardState == "failed"
      IfSilent +2
      MessageBox MB_OK|MB_ICONEXCLAMATION "Закройте Mangar Downloader и другие установщики, затем повторите установку."
      SetErrorLevel 10
      Quit
    ${EndIf}
    IntOp $Counter $Counter + 1
    ${If} $Counter > 100
      SetErrorLevel 10
      Quit
    ${EndIf}
    Sleep 100
  ${Loop}
!macroend

Section "Mangar Downloader" Main
  SectionIn RO
  !insertmacro StartGuard ""
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --safe-directory "$INSTDIR"' $0
  ${If} $0 != 0
    Goto failed
  ${EndIf}
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --registry-preflight "$INSTDIR"' $0
  ${If} $0 != 0
    Goto failed
  ${EndIf}
  # A journal belongs to this fixed installation only. Recover before preflight.
  ${If} ${FileExists} "$INSTDIR\transaction.ini"
    ReadINIStr $R0 "$INSTDIR\transaction.ini" "transaction" "owner"
    ${If} $R0 != "${PRODUCT_ID}"
      Goto failed
    ${EndIf}
    ReadINIStr $R0 "$INSTDIR\transaction.ini" "transaction" "state"
    ${If} $R0 != "committed"
      !insertmacro Owned Restore
      !insertmacro RestoreRegistration
    ${EndIf}
    !insertmacro Owned Cleanup
    RMDir "$INSTDIR\.new"
    RMDir "$INSTDIR\.rollback"
    Delete "$INSTDIR\transaction.ini"
  ${EndIf}
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --preflight "$INSTDIR" "${APP_VERSION}"' $0
  ${If} $0 != 0
    Goto failed
  ${EndIf}
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --check-shortcuts "$INSTDIR"' $0
  ${If} $0 != 0
    Goto failed
  ${EndIf}
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --webview' $0
  ${If} $0 != 0
    SetOutPath "$PLUGINSDIR"
    File /oname=WebView2Bootstrapper.exe "${WEBVIEW_BOOTSTRAPPER}"
    DetailPrint "Установка Microsoft WebView2 Runtime…"
    ExecWait '"$PLUGINSDIR\WebView2Bootstrapper.exe" /silent /install' $0
    ExecWait '"$PLUGINSDIR\mangar-updater.exe" --webview' $0
    ${If} $0 != 0
      SetErrorLevel 11
      IfSilent +2
      MessageBox MB_OK|MB_ICONSTOP "Не удалось установить WebView2 Runtime. Проверьте сеть и повторите установку."
      Quit
    ${EndIf}
  ${EndIf}
  StrCpy $R2 ""
  ReadRegStr $R2 HKCU "${UNINSTALL_KEY}" "DisplayVersion"
  ClearErrors
  CreateDirectory "$INSTDIR"
  WriteINIStr "$INSTDIR\transaction.ini" "transaction" "owner" "${PRODUCT_ID}"
  WriteINIStr "$INSTDIR\transaction.ini" "transaction" "state" "pending"
  WriteINIStr "$INSTDIR\transaction.ini" "registry" "version" "$R2"
  CreateDirectory "$INSTDIR\.new"
  CreateDirectory "$INSTDIR\.rollback"
  ${If} ${Errors}
    Goto rollback
  ${EndIf}
  SetOutPath "$INSTDIR\.new"
  ClearErrors
  File "${PAYLOAD_DIR}\YTDLP-GUI-Mangar.exe"
  File "${PAYLOAD_DIR}\mangar-browser-host.exe"
  File "${PAYLOAD_DIR}\mangar-updater.exe"
  File "${PAYLOAD_DIR}\installation.json"
  WriteUninstaller "$INSTDIR\.new\uninstall.exe"
  ${If} ${Errors}
    Goto rollback
  ${EndIf}
  # Windows keeps the current working directory open. Leave .new before cleanup.
  SetOutPath "$INSTDIR"
  !insertmacro Owned Backup
  !insertmacro Owned CommitFile
  !ifdef MANGAR_TEST_FAIL_AFTER_REPLACE
    Goto rollback
  !endif
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --register "$INSTDIR"' $0
  ${If} $0 != 0
    Goto rollback
  ${EndIf}
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --verify-installed "$INSTDIR" "${APP_VERSION}"' $0
  ${If} $0 != 0
    Goto rollback
  ${EndIf}
  ClearErrors
  !insertmacro WriteRegistration "${APP_VERSION}"
  CreateShortCut "$SMPROGRAMS\${PRODUCT_NAME}.lnk" "$INSTDIR\YTDLP-GUI-Mangar.exe" "" "$INSTDIR\YTDLP-GUI-Mangar.exe" 0
  ${If} ${Errors}
    Goto rollback
  ${EndIf}
  !ifdef MANGAR_TEST_FAIL_AFTER_REGISTRATION
    Goto rollback
  !endif
  ClearErrors
  WriteINIStr "$INSTDIR\transaction.ini" "transaction" "state" "committed"
  ${If} ${Errors}
    Goto rollback
  ${EndIf}
  !insertmacro Owned Cleanup
  RMDir "$INSTDIR\.new"
  RMDir "$INSTDIR\.rollback"
  Delete "$INSTDIR\transaction.ini"
  SetErrorLevel 0
  Goto done
rollback:
  SetOutPath "$PLUGINSDIR"
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --unregister "$INSTDIR"' $0
  !insertmacro Owned Restore
  !insertmacro RestoreRegistration
  !insertmacro Owned Cleanup
  RMDir "$INSTDIR\.new"
  RMDir "$INSTDIR\.rollback"
  Delete "$INSTDIR\transaction.ini"
  ${If} ${FileExists} "$INSTDIR\installation.json"
    ExecWait '"$PLUGINSDIR\mangar-updater.exe" --register "$INSTDIR"' $0
  ${EndIf}
failed:
  SetErrorLevel 12
  IfSilent +2
  MessageBox MB_OK|MB_ICONSTOP "Установка не завершена. Проверьте доступ к папке, версии и регистрацию Native Messaging. Повторите установку."
  Quit
done:
SectionEnd
Section /o "Ярлык на рабочем столе" DesktopShortcut
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --check-desktop-shortcut "$INSTDIR"' $0
  ${If} $0 == 0
    CreateShortCut "$DESKTOP\${PRODUCT_NAME}.lnk" "$INSTDIR\YTDLP-GUI-Mangar.exe" "" "$INSTDIR\YTDLP-GUI-Mangar.exe" 0
  ${Else}
    IfSilent +2
    MessageBox MB_OK|MB_ICONEXCLAMATION "Ярлык на рабочем столе не создан: одноимённый ярлык недоступен или ведёт к другому файлу."
  ${EndIf}
SectionEnd
Function LaunchApp
  Exec '"$INSTDIR\YTDLP-GUI-Mangar.exe"'
FunctionEnd
Function OpenStore
  ExecShell "open" "${STORE_URL}"
FunctionEnd
Function un.onInit
  SetShellVarContext current
  SetRegView 64
FunctionEnd
Section "uninstall"
  !insertmacro StartGuard "un."
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --preflight "$INSTDIR" "${APP_VERSION}"' $0
  ${If} $0 != 0
    SetErrorLevel 12
    Quit
  ${EndIf}
  ${If} ${FileExists} "$INSTDIR\transaction.ini"
    SetErrorLevel 12
    Quit
  ${EndIf}
  ReadRegStr $R0 HKCU "${UNINSTALL_KEY}" "ProductID"
  ReadRegStr $R1 HKCU "${UNINSTALL_KEY}" "InstallLocation"
  ${If} $R0 != "${PRODUCT_ID}"
  ${OrIf} $R1 != "$INSTDIR"
    SetErrorLevel 12
    Quit
  ${EndIf}
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --unregister "$INSTDIR"' $0
  ${If} $0 != 0
    SetErrorLevel 12
    Quit
  ${EndIf}
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --remove-credential' $0
  ${If} $0 != 0
    SetErrorLevel 12
    Quit
  ${EndIf}
  ExecWait '"$PLUGINSDIR\mangar-updater.exe" --remove-shortcuts "$INSTDIR"' $0
  ${If} $0 != 0
    SetErrorLevel 12
    Quit
  ${EndIf}
  !insertmacro Owned DeleteOwned
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  RMDir "$INSTDIR"
  SetErrorLevel 0
SectionEnd
