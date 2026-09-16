Function EnrollmentCreate
  existing_device_retry:
  Push "$INSTDIR"
  Push "${GATEWAY_URL}"
  AetherisProvisioning::VerifyExistingDevice /NOUNLOAD
  Pop $0
  StrCmp $0 "reusable" existing_device_reusable
  StrCmp $0 "server_unavailable" 0 enrollment_required
    MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "暂时无法验证现有设备凭据。请检查网络后重试；取消将退出安装且不会修改本地数据。" IDRETRY existing_device_retry IDCANCEL existing_device_cancel
  existing_device_cancel:
    Quit
  existing_device_reusable:
    StrCpy $ReuseExistingDevice "1"
    Abort
  enrollment_required:
  StrCpy $ReuseExistingDevice "0"
  !insertmacro MUI_HEADER_TEXT "注册设备" "输入组织用户标识和管理员提供的 enrollment code。"
  nsDialogs::Create 1018
  Pop $0
  ${If} $0 == error
    Abort
  ${EndIf}
  ${NSD_CreateLabel} 0 8u 100% 12u "用户标识"
  Pop $0
  ${NSD_CreateText} 0 24u 100% 14u "$UserIdentifier"
  Pop $UserInput
  ${NSD_CreateLabel} 0 50u 100% 12u "Enrollment code"
  Pop $0
  ${NSD_CreatePassword} 0 66u 100% 14u ""
  Pop $EnrollmentInput
  ${NSD_CreateLabel} 0 92u 100% 30u "服务器地址由安装包自动配置。Device token 将自动签发并使用 Windows DPAPI 加密保存。"
  Pop $0
  nsDialogs::Show
FunctionEnd

Function EnrollmentLeave
  StrCmp $ReuseExistingDevice "1" enrollment_valid
  ${NSD_GetText} $UserInput $UserIdentifier
  ${NSD_GetText} $EnrollmentInput $EnrollmentCode
  StrCmp $UserIdentifier "" invalid_user
  StrCmp $EnrollmentCode "" invalid_enrollment
  enrollment_valid:
  Return
  invalid_user:
    MessageBox MB_ICONEXCLAMATION "用户标识不能为空。"
    Abort
  invalid_enrollment:
    MessageBox MB_ICONEXCLAMATION "Enrollment code 不能为空。"
    Abort
FunctionEnd
