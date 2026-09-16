Unicode true
ManifestDPIAware true
RequestExecutionLevel admin
SetCompressor /SOLID lzma
SetCompressorDictSize 32

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef CORE_EXE
  !error "CORE_EXE is required"
!endif
!ifndef SERVICE_EXE
  !error "SERVICE_EXE is required"
!endif
!ifndef PLUGIN_DIR
  !error "PLUGIN_DIR is required"
!endif
!ifndef OUTPUT_DIR
  !error "OUTPUT_DIR is required"
!endif
!ifndef GATEWAY_URL
  !error "GATEWAY_URL is required"
!endif

!addplugindir /x86-unicode "${PLUGIN_DIR}"
!include "MUI2.nsh"
!include "nsDialogs.nsh"
!include "LogicLib.nsh"
!include "WinMessages.nsh"
!include "WordFunc.nsh"
!include "FileFunc.nsh"

!define PBS_MARQUEE 0x08
!define SERVICE_ROOT "$PROGRAMFILES64\Aetheris\Service"

Name "Aetheris Core ${VERSION}"
OutFile "${OUTPUT_DIR}\AetherisSetup-${VERSION}.exe"
InstallDir "$LOCALAPPDATA\Aetheris"
InstallDirRegKey HKCU "Software\Aetheris" "InstallDir"
BrandingText "Aetheris Core ${VERSION}"
ShowInstDetails show

Var ProjectsRoot
Var ProjectsList
Var ProjectsSkip
Var ProjectsScanButton
Var ProjectsStatus
Var ProjectsProgress
Var UserIdentifier
Var EnrollmentCode
Var UserInput
Var EnrollmentInput
Var ProvisionResult
Var ReuseExistingDevice
Var HadExistingService
Var PreviousServiceVersion
Var ServiceOnly
Var CleanupOnly

!include "pages\Projects.nsh"
!include "pages\Enrollment.nsh"
!include "pages\Progress.nsh"

!define MUI_ABORTWARNING
!define MUI_WELCOMEPAGE_TITLE "安装 Aetheris Core"
!define MUI_WELCOMEPAGE_TEXT "此向导将安装 Aetheris Core。安装后可从本地 Lens 动态添加或管理项目。"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
Page custom ProjectsCreate ProjectsLeave
Page custom EnrollmentCreate EnrollmentLeave
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_TITLE "Aetheris Core 安装完成"
!define MUI_FINISHPAGE_TEXT "设备已经注册，Aetheris Core 将在通知区域运行。项目可稍后从本地 Lens 添加。"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_COMPONENTS
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
  Delete "$TEMP\AetherisSetup.result"
  ${GetParameters} $0
  ${GetOptions} $0 "/SERVICEONLY=" $1
  StrCmp $1 "1" 0 normal_install_mode
    StrCpy $ServiceOnly "1"
    StrCpy $ReuseExistingDevice "1"
    SetRegView 64
    ReadRegStr $INSTDIR HKLM "Software\Aetheris\Core" "CoreRoot"
    StrCmp $INSTDIR "" 0 +2
      ReadRegStr $INSTDIR HKCU "Software\Aetheris" "InstallDir"
    SetSilent silent
  normal_install_mode:
  ${GetOptions} $0 "/CLEANUPONLY=" $1
  StrCmp $1 "1" 0 normal_cleanup_mode
    StrCpy $CleanupOnly "1"
    SetRegView 64
    SetSilent silent
  normal_cleanup_mode:
  ; provisioning DLL owns the background scanner and selected-project state.
  ; Unloading it between calls would join the scan thread on the installer UI thread.
  StrCpy $ProjectsRoot "$DOCUMENTS"
  ReadEnvStr $0 "USERDOMAIN"
  ReadEnvStr $1 "USERNAME"
  StrCmp $0 "" 0 +2
  StrCpy $UserIdentifier "$1"
  StrCmp $0 "" +2
  StrCpy $UserIdentifier "$0\$1"
  !ifdef TEST_USER
    StrCpy $UserIdentifier "${TEST_USER}"
  !endif
  !ifdef TEST_ENROLLMENT
    StrCpy $EnrollmentCode "${TEST_ENROLLMENT}"
  !endif
FunctionEnd

Section "Aetheris Core" CoreSection
  SectionIn RO
  SetShellVarContext current
  SetRegView 64
  StrCmp $CleanupOnly "1" 0 normal_core_install
    AetherisProvisioning::RemoveCoreService /NOUNLOAD
    Pop $0
    WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "cleanup_$0"
    StrCmp $0 "ok" 0 cleanup_failed
      Delete "${SERVICE_ROOT}\AetherisCoreService.exe"
      Delete "${SERVICE_ROOT}\AetherisCoreService.exe.new"
      Delete "${SERVICE_ROOT}\AetherisCoreService.exe.previous"
      RMDir "${SERVICE_ROOT}"
      SetErrorLevel 0
      Goto install_done
    cleanup_failed:
      SetErrorLevel 1
      Abort
  normal_core_install:
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "section_started"
  StrCmp $ServiceOnly "1" 0 service_only_identity_ready
    Push "$INSTDIR"
    Push "${GATEWAY_URL}"
    AetherisProvisioning::VerifyExistingDevice /NOUNLOAD
    Pop $0
    StrCmp $0 "reusable" service_only_identity_ready
      FileOpen $1 "$TEMP\AetherisSetup.result" w
      FileWrite $1 "$0"
      FileClose $1
      Abort
  service_only_identity_ready:
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "identity_ready"
  Push "${SERVICE_ROOT}\AetherisCoreService.exe"
  AetherisProvisioning::QueryCoreService /NOUNLOAD
  Pop $0
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "service_directory_$0"
  StrCmp $0 "missing" 0 +3
    StrCpy $HadExistingService "0"
    Goto service_presence_known
  StrCpy $HadExistingService "1"
  ReadRegStr $PreviousServiceVersion HKLM "Software\Aetheris\Core" "ExpectedVersion"
  service_presence_known:
  Push "$INSTDIR"
  AetherisProvisioning::RequestCoreExit /NOUNLOAD
  Pop $0
  StrCmp $0 "ok" +3
    MessageBox MB_ICONSTOP "无法请求现有 Aetheris Core 安全退出：$0"
    Abort
  Sleep 3000
  DetailPrint "正在停止旧版 Aetheris Core 服务..."
  AetherisProvisioning::StopCoreService /NOUNLOAD
  Pop $0
  SetOutPath "$INSTDIR"
  CreateDirectory "$INSTDIR\config"
  CreateDirectory "$INSTDIR\data"
  CreateDirectory "$INSTDIR\logs"
  CreateDirectory "$INSTDIR\.installing\${VERSION}"
  SetOutPath "$INSTDIR\.installing\${VERSION}"
  File /oname=AetherisCore.exe "${CORE_EXE}"
  Delete "$INSTDIR\AetherisCore.exe.previous"
  ClearErrors
  IfFileExists "$INSTDIR\AetherisCore.exe" 0 +2
    Rename "$INSTDIR\AetherisCore.exe" "$INSTDIR\AetherisCore.exe.previous"
  IfErrors core_stage_failed
  Rename "$INSTDIR\.installing\${VERSION}\AetherisCore.exe" "$INSTDIR\AetherisCore.exe"
  IfErrors core_stage_failed
  RMDir "$INSTDIR\.installing\${VERSION}"
  RMDir "$INSTDIR\.installing"

  !ifdef VSIX_FILE
    CreateDirectory "$INSTDIR\integrations"
    SetOutPath "$INSTDIR\integrations"
    File /oname=AetherisVSCode.vsix "${VSIX_FILE}"
  !endif

  StrCmp $ReuseExistingDevice "1" reuse_device
  DetailPrint "正在注册设备并安全保存凭据..."
  Push "$INSTDIR"
  Push "$UserIdentifier"
  Push "$EnrollmentCode"
  Push "${GATEWAY_URL}"
  Push "${VERSION}"
  AetherisProvisioning::ProvisionDevice /NOUNLOAD
  Pop $ProvisionResult
  StrCpy $EnrollmentCode ""
  ${NSD_SetText} $EnrollmentInput ""
  StrCpy $0 $ProvisionResult 3
  StrCmp $0 "ok|" provision_ok
    Call ShowProvisionError
    Goto install_rollback

  provision_ok:
  Goto identity_ready
  reuse_device:
  identity_ready:
  WriteRegStr HKCU "Software\Aetheris" "InstallDir" "$INSTDIR"
  Push "${SERVICE_ROOT}"
  AetherisProvisioning::PrepareCoreServiceDirectory /NOUNLOAD
  Pop $0
  StrCmp $0 "ok" service_directory_ready
    FileOpen $1 "$TEMP\AetherisSetup.result" w
    FileWrite $1 "$0"
    FileClose $1
    MessageBox MB_ICONSTOP "无法准备 Aetheris Core Service 目录：$0"
    Goto install_rollback
  service_directory_ready:
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File /oname=AetherisCoreService.exe "${SERVICE_EXE}"
  Push "$PLUGINSDIR\AetherisCoreService.exe"
  Push "${SERVICE_ROOT}\AetherisCoreService.exe"
  AetherisProvisioning::DeployCoreServiceBinary /NOUNLOAD
  Pop $0
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "service_binary_$0"
  StrCmp $0 "ok" service_binary_ready
    FileOpen $1 "$TEMP\AetherisSetup.result" w
    FileWrite $1 "$0"
    FileClose $1
    MessageBox MB_ICONSTOP "无法原子更新 Aetheris Core Service：$0"
    Goto install_rollback
  service_binary_ready:
  Push "${SERVICE_ROOT}\AetherisCoreService.exe"
  AetherisProvisioning::VerifyCoreServiceBinaryAccess /NOUNLOAD
  Pop $0
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "service_access_$0"
  StrCmp $0 "ready" service_access_ready
    MessageBox MB_ICONSTOP "Aetheris Core Service 文件权限无效：$0"
    Goto install_rollback
  service_access_ready:
  Push "${SERVICE_ROOT}\AetherisCoreService.exe"
  AetherisProvisioning::VerifyCoreServiceBinarySignature /NOUNLOAD
  Pop $0
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "service_signature_$0"
  StrCmp $0 "valid" service_signature_ready
    MessageBox MB_ICONSTOP "Aetheris Core Service 签名无效：$0"
    Goto install_rollback
  service_signature_ready:
  Push "${SERVICE_ROOT}\AetherisCoreService.exe"
  Push "$INSTDIR"
  Push "${VERSION}"
  AetherisProvisioning::InstallCoreService /NOUNLOAD
  Pop $0
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "service_install_$0"
  StrCmp $0 "ok" service_installed
    FileOpen $1 "$TEMP\AetherisSetup.result" w
    FileWrite $1 "$0"
    FileClose $1
    MessageBox MB_ICONSTOP "无法安装 Aetheris Core 服务：$0"
    Goto install_rollback
  service_installed:
  DetailPrint "正在启动 Aetheris Core 服务..."
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "service_start_calling"
  AetherisProvisioning::StartCoreService /NOUNLOAD
  Pop $0
  WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "service_start_$0"
  StrCmp $0 "ok" service_started
    FileOpen $1 "$TEMP\AetherisSetup.result" w
    FileWrite $1 "$0"
    FileClose $1
    StrCmp $ServiceOnly "1" service_only_start_failed
    MessageBox MB_ICONSTOP "无法启动 Aetheris Core 服务：$0"
    Goto install_rollback
  service_only_start_failed:
    SetErrorLevel 1
    Abort
  service_started:
  StrCpy $0 0
  verify_core_loop:
    Sleep 1000
    Push "$INSTDIR"
    Push "${VERSION}"
    AetherisProvisioning::VerifyCoreStatus /NOUNLOAD
    Pop $1
    StrCmp $1 "ok" verify_core_done
    IntOp $0 $0 + 1
    IntCmp $0 45 verify_core_failed verify_core_loop verify_core_failed
  verify_core_failed:
    WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "core_verify_failed"
    MessageBox MB_ICONSTOP "Aetheris Core 未能在 45 秒内完成注册状态验证。安装文件已保留，可查看 logs 目录。"
    Goto install_rollback
  verify_core_done:
    StrCmp $ServiceOnly "1" service_only_ready
    Push "$INSTDIR"
    Push "${VERSION}"
    AetherisProvisioning::UpdateExistingCoreVersion /NOUNLOAD
    Pop $0
    StrCmp $0 "ok" config_version_ready
      MessageBox MB_ICONSTOP "无法提交 Core 配置版本：$0"
      Goto install_rollback
  config_version_ready:
    WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "ready"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "AetherisCore"
    Delete "$APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\Aetheris Core.lnk"
    CreateDirectory "$SMPROGRAMS\Aetheris"
    CreateShortCut "$SMPROGRAMS\Aetheris\Aetheris Core.lnk" "$INSTDIR\AetherisCore.exe" "" "$INSTDIR\AetherisCore.exe"
    WriteUninstaller "$INSTDIR\Uninstall.exe"
    CreateShortCut "$SMPROGRAMS\Aetheris\卸载 Aetheris Core.lnk" "$INSTDIR\Uninstall.exe"
    Delete "$INSTDIR\AetherisCore.exe.previous"
    Delete "${SERVICE_ROOT}\AetherisCoreService.exe.previous"
    FileOpen $1 "$TEMP\AetherisSetup.result" w
    FileWrite $1 "ready"
    FileClose $1
    DetailPrint "Aetheris Core 已启动并完成注册验证。"
    Goto install_done
  service_only_ready:
    Delete "$INSTDIR\AetherisCore.exe.previous"
    Delete "${SERVICE_ROOT}\AetherisCoreService.exe.previous"
    WriteRegStr HKLM "Software\Aetheris\Installer" "LastStage" "ready"
    FileOpen $1 "$TEMP\AetherisSetup.result" w
    FileWrite $1 "ready"
    FileClose $1
    SetErrorLevel 0
    Goto install_done
  core_stage_failed:
    FileOpen $1 "$TEMP\AetherisSetup.result" w
    FileWrite $1 "core_stage_failed"
    FileClose $1
    MessageBox MB_ICONSTOP "无法原子更新 Aetheris Core，现有进程可能仍在运行。"
    Goto install_rollback
  install_rollback:
    AetherisProvisioning::RemoveCoreService /NOUNLOAD
    Pop $0
    Push "${SERVICE_ROOT}\AetherisCoreService.exe"
    AetherisProvisioning::RestoreCoreServiceBinary /NOUNLOAD
    Pop $0
    Delete "$INSTDIR\AetherisCore.exe"
    Delete "$INSTDIR\.installing\${VERSION}\AetherisCore.exe"
    RMDir "$INSTDIR\.installing\${VERSION}"
    RMDir "$INSTDIR\.installing"
    IfFileExists "$INSTDIR\AetherisCore.exe.previous" 0 +2
      Rename "$INSTDIR\AetherisCore.exe.previous" "$INSTDIR\AetherisCore.exe"
    StrCmp $HadExistingService "1" 0 rollback_done
    StrCmp $PreviousServiceVersion "" 0 +2
      StrCpy $PreviousServiceVersion "0.4.7"
    Push "${SERVICE_ROOT}\AetherisCoreService.exe"
    Push "$INSTDIR"
    Push "$PreviousServiceVersion"
    AetherisProvisioning::InstallCoreService /NOUNLOAD
    Pop $0
    StrCmp $0 "ok" 0 rollback_done
      AetherisProvisioning::StartCoreService /NOUNLOAD
      Pop $0
    rollback_done:
    Abort
  install_done:
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  Push "$INSTDIR"
  AetherisProvisioning::RevokeDevice /NOUNLOAD
  Pop $0
  AetherisProvisioning::RemoveCoreService /NOUNLOAD
  Pop $1
  StrCmp $1 "ok" service_removed
    MessageBox MB_ICONSTOP "Aetheris Core 服务删除失败：$1。服务程序将保留。"
    Goto service_remove_failed
  service_removed:
  Delete "${SERVICE_ROOT}\AetherisCoreService.exe"
  Delete "${SERVICE_ROOT}\AetherisCoreService.exe.new"
  Delete "${SERVICE_ROOT}\AetherisCoreService.exe.previous"
  RMDir /r "${SERVICE_ROOT}"
  service_remove_failed:
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "AetherisCore"
  DeleteRegKey HKCU "Software\Aetheris"
  SetRegView 64
  DeleteRegKey HKLM "Software\Aetheris\Core"
  DeleteRegKey HKLM "Software\Aetheris\Installer"
  Delete "$SMPROGRAMS\Aetheris\Aetheris Core.lnk"
  Delete "$SMPROGRAMS\Aetheris\卸载 Aetheris Core.lnk"
  RMDir "$SMPROGRAMS\Aetheris"
  RMDir /r "$PROGRAMDATA\Aetheris"
  RMDir /r "$INSTDIR"
SectionEnd

Section /o "同时删除本地数据与项目设置" un.RemoveLocalData
  RMDir /r "$INSTDIR\data"
  RMDir /r "$INSTDIR\logs"
  RMDir /r "$INSTDIR\config"
  RMDir "$INSTDIR"
SectionEnd
