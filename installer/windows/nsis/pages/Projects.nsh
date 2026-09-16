Function ProjectsCreate
  !insertmacro MUI_HEADER_TEXT "项目授权（可选）" "现在选择项目，或安装后从本地 Lens 动态添加。"
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}

  ${NSD_CreateLabel} 0 0 100% 22u "Git/SVN 项目扫描目录"
  Pop $0
  ${NSD_CreateText} 0 24u 74% 13u "$ProjectsRoot"
  Pop $ProjectsRoot
  ${NSD_CreateBrowseButton} 76% 23u 24% 15u "浏览..."
  Pop $0
  ${NSD_OnClick} $0 ProjectsBrowse
  ${NSD_CreateButton} 0 44u 24% 16u "扫描项目"
  Pop $ProjectsScanButton
  ${NSD_OnClick} $ProjectsScanButton ProjectsScan
  ${NSD_CreateLabel} 26% 47u 74% 13u "尚未扫描。"
  Pop $ProjectsStatus
  nsDialogs::CreateControl "msctls_progress32" ${DEFAULT_STYLES}|${PBS_MARQUEE} ${WS_EX_CLIENTEDGE} 0 62u 100% 8u ""
  Pop $ProjectsProgress
  ShowWindow $ProjectsProgress ${SW_HIDE}
  nsDialogs::CreateControl LISTBOX ${DEFAULT_STYLES}|${WS_TABSTOP}|${WS_VSCROLL}|${LBS_NOTIFY}|${LBS_EXTENDEDSEL} ${WS_EX_CLIENTEDGE} 0 74u 100% 43u ""
  Pop $ProjectsList
  ${NSD_CreateCheckbox} 0 121u 100% 14u "稍后添加项目（不影响安装）"
  Pop $ProjectsSkip
  nsDialogs::Show
FunctionEnd

Function ProjectsBrowse
  ${NSD_GetText} $ProjectsRoot $0
  nsDialogs::SelectFolderDialog "选择 Git/SVN 项目扫描目录" "$0"
  Pop $0
  StrCmp $0 error done
  ${NSD_SetText} $ProjectsRoot "$0"
  done:
FunctionEnd

Function ProjectsScan
  ${NSD_GetText} $ProjectsRoot $0
  StrCmp $0 "" no_root
  EnableWindow $ProjectsScanButton 0
  ${NSD_SetText} $ProjectsStatus "正在扫描：已扫描目录 0，已发现项目 0"
  ShowWindow $ProjectsProgress ${SW_SHOW}
  SendMessage $ProjectsProgress ${PBM_SETMARQUEE} 1 30
  Push "$0"
  AetherisProvisioning::StartProjectScan /NOUNLOAD
  Pop $1
  StrCmp $1 "started" 0 scan_failed
  ${NSD_CreateTimer} ProjectsPoll 250
  Return
  no_root:
    MessageBox MB_ICONINFORMATION "请选择要扫描的目录，或勾选稍后添加项目。"
    Return
  scan_failed:
    SendMessage $ProjectsProgress ${PBM_SETMARQUEE} 0 0
    ShowWindow $ProjectsProgress ${SW_HIDE}
    EnableWindow $ProjectsScanButton 1
    ${NSD_SetText} $ProjectsStatus "扫描无法启动，可稍后从本地 Lens 添加项目。"
FunctionEnd

Function ProjectsPoll
  AetherisProvisioning::PollProjectScan /NOUNLOAD
  Pop $0
  StrCpy $1 $0 8
  StrCmp $1 "complete" scan_complete
  StrCpy $1 $0 7
  StrCmp $1 "warning" scan_warning
  ${WordFind} "$0" "|" "+2" $2
  ${WordFind} "$0" "|" "+3" $3
  ${NSD_SetText} $ProjectsStatus "正在扫描：已扫描目录 $2，已发现项目 $3"
  Return
  scan_complete:
  scan_warning:
    ${NSD_KillTimer} ProjectsPoll
    SendMessage $ProjectsProgress ${PBM_SETMARQUEE} 0 0
    ShowWindow $ProjectsProgress ${SW_HIDE}
    Push "$ProjectsList"
    AetherisProvisioning::PopulateProjectList /NOUNLOAD
    Pop $2
    SendMessage $ProjectsList ${LB_SETSEL} ${BST_CHECKED} -1
    EnableWindow $ProjectsScanButton 1
    ${NSD_SetText} $ProjectsStatus "发现 $2 个项目。可取消不需要授权的项目。"
FunctionEnd

Function ProjectsLeave
  !ifdef TEST_DIAGNOSTICS
    FileOpen $9 "$TEMP\aetheris-projects-leave.log" w
    FileWrite $9 "entered$\r$\n"
  !endif
  ${NSD_KillTimer} ProjectsPoll
  SendMessage $ProjectsProgress ${PBM_SETMARQUEE} 0 0
  !ifdef TEST_DIAGNOSTICS
    FileWrite $9 "timer-cleared$\r$\n"
  !endif
  ${NSD_GetState} $ProjectsSkip $0
  ${If} $0 == ${BST_CHECKED}
    Push "0"
  ${Else}
    Push "$ProjectsList"
  ${EndIf}
  AetherisProvisioning::WriteConfiguration /NOUNLOAD
  Pop $1
  !ifdef TEST_DIAGNOSTICS
    FileWrite $9 "plugin-returned:$1$\r$\n"
    FileClose $9
  !endif
FunctionEnd
