Function ShowProvisionError
  StrCpy $0 $ProvisionResult "" 6
  StrCmp $0 "invalid_enrollment" 0 +3
    MessageBox MB_ICONSTOP "Enrollment code 无效，请返回后重新输入。"
    Return
  StrCmp $0 "server_unavailable" 0 +3
    MessageBox MB_ICONSTOP "暂时无法连接 Aetheris Server，请检查网络后重试。"
    Return
  MessageBox MB_ICONSTOP "设备注册失败：$0"
FunctionEnd
