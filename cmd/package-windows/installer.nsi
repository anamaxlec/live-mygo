; 生成安装包：makensis /INPUTCHARSET UTF8 cmd\package-windows\installer.nsi
!cd "..\.."

Unicode true
SetCompressor /SOLID lzma
SetCompressorDictSize 64

!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "WinMessages.nsh"
!include "x64.nsh"

!define APP_NAME "Live MyGo"
!define APP_VERSION "0.4.1-beta.2"
!define APP_EXE "LiveMyGo.exe"
!define PAYLOAD "build\windows-amd64"
!define UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\LiveMyGo"

Name "${APP_NAME}"
OutFile "build\releases\LiveMyGo-v0.4.1-beta.2-windows-amd64-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\LiveMyGo"
InstallDirRegKey HKCU "${UNINST_KEY}" "InstallLocation"
RequestExecutionLevel user
Icon "${PAYLOAD}\AppIcon.ico"
UninstallIcon "${PAYLOAD}\AppIcon.ico"
BrandingText "${APP_NAME} ${APP_VERSION}"

VIProductVersion "0.4.1.2"
VIAddVersionKey "ProductName" "${APP_NAME}"
VIAddVersionKey "ProductVersion" "${APP_VERSION}"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "FileDescription" "${APP_NAME} 安装程序"
VIAddVersionKey "LegalCopyright" "MIT License"

!define MUI_ABORTWARNING
!define MUI_ICON "${PAYLOAD}\AppIcon.ico"

!define APP_WINDOW_CLASS "MyGoWindow"
!define APP_WINDOW_TITLE "Live MyGo"
!define MUI_UNICON "${PAYLOAD}\AppIcon.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "运行 ${APP_NAME}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${PAYLOAD}\LICENSE"
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"

LangString DESC_Main ${LANG_SIMPCHINESE} "程序、播放运行库和说明文档（必需）。"
LangString DESC_Desktop ${LANG_SIMPCHINESE} "在桌面创建快捷方式。"

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "Live MyGo 仅支持 64 位 Windows 10 或更高版本。"
    Abort
  ${EndIf}
FunctionEnd

; 目标目录里的程序正在运行时，覆盖 LiveMyGo.exe 会失败。
; 先请求关闭；拒绝则取消安装，无法自动关闭时再询问是否强制结束。
Function CloseRunningApp
  IfFileExists "$INSTDIR\${APP_EXE}" 0 closeDone
  ClearErrors
  FileOpen $0 "$INSTDIR\${APP_EXE}" "a"
  IfErrors 0 closeOpened
  IfSilent closeNow
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Live MyGo 正在运行，需要关闭后才能继续安装。$\r$\n$\r$\n是否关闭 Live MyGo 并继续安装？" IDYES closeNow
  MessageBox MB_OK|MB_ICONINFORMATION "安装已取消。请退出 Live MyGo 后重新运行安装程序。"
  Abort

closeNow:
  FindWindow $1 "${APP_WINDOW_CLASS}" "${APP_WINDOW_TITLE}"
  StrCmp $1 0 closeWait
  SendMessage $1 ${WM_CLOSE} 0 0

closeWait:
  StrCpy $2 0
closeWaitLoop:
  Sleep 250
  IntOp $2 $2 + 1
  ClearErrors
  FileOpen $0 "$INSTDIR\${APP_EXE}" "a"
  IfErrors closeStillLocked
  FileClose $0
  Goto closeDone
closeStillLocked:
  IntCmp $2 32 closeForce closeWaitLoop

closeForce:
  IfSilent closeForceNow
  MessageBox MB_YESNO|MB_ICONEXCLAMATION|MB_DEFBUTTON2 "无法自动关闭 Live MyGo。是否强制结束该应用并继续安装？" IDYES closeForceNow
  MessageBox MB_OK|MB_ICONINFORMATION "安装已取消。请退出 Live MyGo 后重新运行安装程序。"
  Abort

closeForceNow:
  nsExec::ExecToLog 'taskkill /F /T /IM LiveMyGo.exe'
  Sleep 600
  ClearErrors
  FileOpen $0 "$INSTDIR\${APP_EXE}" "a"
  IfErrors closeFailed
  FileClose $0
  Goto closeDone

closeFailed:
  MessageBox MB_OK|MB_ICONSTOP "仍无法关闭 Live MyGo，请手动退出应用后重新运行安装程序。"
  Abort

closeOpened:
  FileClose $0
  Goto closeDone

closeDone:
FunctionEnd

Section "主程序" SEC_MAIN
  SectionIn RO
  Call CloseRunningApp
  SetOutPath "$INSTDIR"
  File /r "${PAYLOAD}\*.*"

  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateDirectory "$SMPROGRAMS\${APP_NAME}"
  CreateShortCut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
  CreateShortCut "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk" "$INSTDIR\uninstall.exe"

  WriteRegStr HKCU "${UNINST_KEY}" "DisplayName" "${APP_NAME}"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayVersion" "${APP_VERSION}"
  WriteRegStr HKCU "${UNINST_KEY}" "Publisher" "Live MyGo"
  WriteRegStr HKCU "${UNINST_KEY}" "DisplayIcon" "$INSTDIR\${APP_EXE}"
  WriteRegStr HKCU "${UNINST_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINST_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST_KEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINST_KEY}" "EstimatedSize" "$0"
SectionEnd

Section "桌面快捷方式" SEC_DESKTOP
  CreateShortCut "$DESKTOP\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}" "" "$INSTDIR\${APP_EXE}" 0
SectionEnd

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_MAIN} $(DESC_Main)
  !insertmacro MUI_DESCRIPTION_TEXT ${SEC_DESKTOP} $(DESC_Desktop)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
  Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
  Delete "$SMPROGRAMS\${APP_NAME}\卸载 ${APP_NAME}.lnk"
  RMDir "$SMPROGRAMS\${APP_NAME}"
  Delete "$DESKTOP\${APP_NAME}.lnk"
  DeleteRegKey HKCU "${UNINST_KEY}"
  RMDir /r "$INSTDIR"
SectionEnd
