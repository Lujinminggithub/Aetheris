# Aetheris Windows 安装说明

系统要求：Windows 10/11。

1. 下载并运行原生 NSIS 安装程序 `AetherisSetup-0.4.16.exe`，在 UAC 中确认 Windows 服务安装。
   当前内网测试包使用 `PersonalSafer Test` Authenticode 证书；目标终端必须预先信任该证书。正式外部分发必须使用公共 CA 代码签名证书。
2. 选择安装目录；Git/SVN 项目可以现在扫描，也可以跳过后从本地 Lens 动态添加。
3. 确认用户标识并输入 enrollment code。
4. Setup 优先验证并复用现有设备身份；缺失或失效时才执行 bootstrap。DPAPI credential、服务和 Core 启动验证全部成功后才显示安装完成。
5. `AetherisCoreService` 自动监管用户所选目录中的 `AetherisCore.exe`；托盘主动退出后可从开始菜单重新启动。

Core 首次启动时会自动补全原生 Setup 未显式写入的本地 AI 会话源，包括 Codex、Claude Code、GitHub Copilot 和 Cursor；只采集授权项目下的结构化会话记录。

安装包不包含服务器密码、enrollment code 或 device token。截图不会持久化。
