Unicode true
RequestExecutionLevel user
SilentInstall silent
!addplugindir /x86-unicode "${PLUGIN_DIR}"
OutFile "${OUT_FILE}"
Section
  Push "${INSTALL_ROOT}"
  Push "0.4.16"
  AetherisProvisioning::VerifyCoreStatus
  Pop $0
  FileOpen $1 "$TEMP\aetheris-verify-smoke.txt" w
  FileWrite $1 "$0"
  FileClose $1
SectionEnd
