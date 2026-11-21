; Bondr-Installer: installiert für den angemeldeten Benutzer, ohne Adminrechte.
Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"

!ifndef FASSUNG
  !define FASSUNG "0.0.0"
!endif

Name "Bondr"
OutFile "dist\Bondr-${FASSUNG}-Setup.exe"
InstallDir "$LOCALAPPDATA\Programs\Bondr"
RequestExecutionLevel user
SetCompressor /SOLID lzma
BrandingText "Bondr ${FASSUNG}"
VIProductVersion "${FASSUNG}.0"
VIAddVersionKey "ProductName" "Bondr"
VIAddVersionKey "FileDescription" "Bondr Installer"
VIAddVersionKey "FileVersion" "${FASSUNG}"
VIAddVersionKey "LegalCopyright" "MIT"

!define MUI_ICON "res\bondr.ico"
!define MUI_UNICON "res\bondr.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\Bondr.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Bondr jetzt starten"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "German"

!define UNINST "Software\Microsoft\Windows\CurrentVersion\Uninstall\Bondr"

Section "Bondr"
  ; laufende Fassung beenden, damit die Dateien ersetzt werden können; der Umleiter
  ; (mit Adminrechten) endet von selbst, sobald bondrd weg ist
  nsExec::Exec 'taskkill /im Bondr.exe /f'
  nsExec::Exec 'taskkill /im bondrd.exe /f'
  Sleep 800
  nsExec::Exec 'schtasks /end /tn "Bondr Umleitung"'
  Sleep 4000
  SetOutPath "$INSTDIR"
  File "dist\Bondr.exe"
  File "dist\bondrd.exe"
  File "dist\WinDivert.dll"
  File "dist\WinDivert64.sys"
  File "dist\WinDivert-LIZENZ.txt"
  WriteUninstaller "$INSTDIR\Deinstallieren.exe"
  CreateShortCut "$SMPROGRAMS\Bondr.lnk" "$INSTDIR\Bondr.exe"
  WriteRegStr HKCU "${UNINST}" "DisplayName" "Bondr"
  WriteRegStr HKCU "${UNINST}" "DisplayVersion" "${FASSUNG}"
  WriteRegStr HKCU "${UNINST}" "Publisher" "Dschonas04"
  WriteRegStr HKCU "${UNINST}" "DisplayIcon" "$INSTDIR\Bondr.exe"
  WriteRegStr HKCU "${UNINST}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINST}" "UninstallString" '"$INSTDIR\Deinstallieren.exe"'
  WriteRegDWORD HKCU "${UNINST}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINST}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  WriteRegDWORD HKCU "${UNINST}" "EstimatedSize" "$0"
SectionEnd

Section "Uninstall"
  nsExec::Exec 'taskkill /im Bondr.exe /f'
  nsExec::Exec 'taskkill /im bondrd.exe /f'
  nsExec::Exec 'schtasks /end /tn "Bondr Umleitung"'
  Sleep 4000
  nsExec::Exec 'schtasks /delete /tn "Bondr Umleitung" /f'
  ; Windows-Proxy abschalten, falls er noch auf Bondr zeigt
  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyServer"
  StrCmp $0 "127.0.0.1:1080" 0 +2
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Internet Settings" "ProxyEnable" 0
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Bondr"
  Delete "$SMPROGRAMS\Bondr.lnk"
  Delete "$INSTDIR\Bondr.exe"
  Delete "$INSTDIR\bondrd.exe"
  Delete "$INSTDIR\WinDivert.dll"
  Delete "$INSTDIR\WinDivert64.sys"
  Delete "$INSTDIR\WinDivert-LIZENZ.txt"
  Delete "$INSTDIR\Deinstallieren.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKCU "${UNINST}"
SectionEnd
