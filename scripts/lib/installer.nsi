; A vero app's Windows installer, made by scripts/package-windows.sh with
; NSIS, which runs on the Mac. Everything that names the app, and whoever
; publishes it, comes from the script's flags.
;
; For the person installing it alone - no administrator needed, and an
; update replaces it - in %LOCALAPPDATA%\Programs\NAME, with a Start menu
; entry, an entry in Windows' list of installed apps, and, when the app
; asks for it (STARTUP), an option, ticked, to open it at sign-in; and,
; for an app that needs it (WEBVIEW2), Microsoft Edge WebView2 where it's
; missing.

Unicode true
!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"

Name "${NAME}"
OutFile "${OUT}"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\${NAME}"
SetCompressor /SOLID lzma
!define UNINSTALL "Software\Microsoft\Windows\CurrentVersion\Uninstall\${ID}"
!define MUI_ICON "${ICON}"
!define MUI_UNICON "${ICON}"
!define MUI_FINISHPAGE_RUN "$INSTDIR\${EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Open ${NAME}"

!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${NAME}"
VIAddVersionKey "ProductVersion" "${VERSION}"
VIAddVersionKey "CompanyName" "${PUBLISHER}"
VIAddVersionKey "LegalCopyright" "${PUBLISHER}"
VIAddVersionKey "FileDescription" "${NAME} installer"
VIAddVersionKey "FileVersion" "${VERSION}"

Function .onInit
!if "${ARCH}" == "arm64"
  ${IfNot} ${IsNativeARM64}
    MessageBox MB_ICONSTOP "This is ${NAME} for Windows on ARM. Download the x64 one for this computer."
    Abort
  ${EndIf}
!endif
FunctionEnd

; A copy that is running holds its files: it is closed first, and opened
; again from the last page.
!macro Close
  nsExec::Exec 'taskkill /IM "${EXE}" /F'
  nsExec::Exec 'taskkill /IM "${WORKER}" /F'
  Sleep 500
!macroend

Section "${NAME}" SecApp
  SectionIn RO
  !insertmacro Close
  SetOutPath "$INSTDIR"
  File /r "${SRC}\*.*"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateShortCut "$SMPROGRAMS\${NAME}.lnk" "$INSTDIR\${EXE}"
  WriteRegStr HKCU "${UNINSTALL}" "DisplayName" "${NAME}"
  WriteRegStr HKCU "${UNINSTALL}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL}" "Publisher" "${PUBLISHER}"
  WriteRegStr HKCU "${UNINSTALL}" "URLInfoAbout" "${URL}"
  WriteRegStr HKCU "${UNINSTALL}" "DisplayIcon" "$INSTDIR\${EXE}"
  WriteRegStr HKCU "${UNINSTALL}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegDWORD HKCU "${UNINSTALL}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL}" "NoRepair" 1
SectionEnd

!ifdef WEBVIEW2
; Microsoft Edge WebView2, which the app shows web pages with. It's
; installed when its client under EdgeUpdate has a version, for everyone
; or for this person, as Microsoft's guide to distributing it says. Where
; it isn't, Microsoft's bootstrapper downloads and installs it; without an
; administrator, for this person. If that fails - no internet, say - the
; app is installed anyway.
!define WV2 "Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}"
Section "-WebView2" SecWebView2
  ReadRegStr $0 HKLM "SOFTWARE\${WV2}" "pv"
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKCU "Software\${WV2}" "pv"
  ${EndIf}
!ifdef WEBVIEW2_TEST
  StrCpy $0 ""
!endif
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    DetailPrint "Installing Microsoft Edge WebView2..."
    InitPluginsDir
    File "/oname=$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" "${WEBVIEW2}"
    ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install' $1
    DetailPrint "WebView2's installer finished with code $1"
  ${Else}
    DetailPrint "Microsoft Edge WebView2 $0 is installed"
  ${EndIf}
SectionEnd
!endif

!ifdef STARTUP
Section "${STARTUP}" SecStartup
  CreateShortCut "$SMSTARTUP\${NAME}.lnk" "$INSTDIR\${EXE}"
SectionEnd
!endif

Section "Uninstall"
  !insertmacro Close
  Delete "$SMPROGRAMS\${NAME}.lnk"
  Delete "$SMSTARTUP\${NAME}.lnk"
  RMDir /r "$INSTDIR"
  DeleteRegKey HKCU "${UNINSTALL}"
SectionEnd
