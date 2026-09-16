Unicode true
RequestExecutionLevel user
SilentInstall silent
!ifndef PLUGIN_DIR
  !error "PLUGIN_DIR required"
!endif
!ifndef OUT_FILE
  !error "OUT_FILE required"
!endif
!addplugindir /x86-unicode "${PLUGIN_DIR}"
OutFile "${OUT_FILE}"
Section
  AetherisProvisioning::PollProjectScan
  Pop $0
  Push "0"
  AetherisProvisioning::WriteConfiguration
  Pop $2
  FileOpen $1 "$TEMP\aetheris-plugin-smoke.txt" w
  FileWrite $1 "$0|$2"
  FileClose $1
SectionEnd
