; Cadence NSIS Installer Script
!define PRODUCT_NAME "Cadence"
!define PRODUCT_VERSION "1.0.0"
!define PRODUCT_PUBLISHER "Adesh Deshmukh"
!define PRODUCT_WEB_SITE "https://github.com/AdeshDeshmukh/cadence"
!define PRODUCT_DIR_REGKEY "Software\Microsoft\Windows\CurrentVersion\App Paths\cadence.exe"
!define PRODUCT_UNINST_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${PRODUCT_NAME}"

SetCompressor /SOLID lzma
Name "${PRODUCT_NAME} ${PRODUCT_VERSION}"
OutFile "..\..\dist\Cadence-Installer-${PRODUCT_VERSION}.exe"
InstallDir "$LOCALAPPDATA\Cadence"
InstallDirRegKey HKCU "${PRODUCT_DIR_REGKEY}" ""
ShowInstDetails show
ShowUnInstDetails show

Section "MainSection" SEC01
  SetOutPath "$INSTDIR"
  SetOverwrite ifnewer
  File "..\..\dist\bin\windows\cadence.exe"
  File "..\..\dist\bin\windows\cadenced.exe"
  File "..\..\README.md"
  File "..\..\LICENSE"

  ; Register in PATH
  EnVar::AddValue "Path" "$INSTDIR"

  ; Shortcut in Start Menu
  CreateDirectory "$SMPROGRAMS\Cadence"
  CreateShortcut "$SMPROGRAMS\Cadence\Cadence CLI.lnk" "$INSTDIR\cadence.exe"
  CreateShortcut "$SMPROGRAMS\Cadence\Uninstall Cadence.lnk" "$INSTDIR\uninst.exe"
SectionEnd

Section -Post
  WriteUninstaller "$INSTDIR\uninst.exe"
  WriteRegStr HKCU "${PRODUCT_DIR_REGKEY}" "" "$INSTDIR\cadence.exe"
  WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "UninstallString" "$INSTDIR\uninst.exe"
  WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "URLInfoAbout" "${PRODUCT_WEB_SITE}"
  WriteRegStr HKCU "${PRODUCT_UNINST_KEY}" "Publisher" "${PRODUCT_PUBLISHER}"
SectionEnd

Section Uninstall
  Delete "$INSTDIR\cadence.exe"
  Delete "$INSTDIR\cadenced.exe"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\uninst.exe"

  RMDir "$SMPROGRAMS\Cadence"
  RMDir "$INSTDIR"

  EnVar::DeleteValue "Path" "$INSTDIR"
  DeleteRegKey HKCU "${PRODUCT_UNINST_KEY}"
  DeleteRegKey HKCU "${PRODUCT_DIR_REGKEY}"
SectionEnd
