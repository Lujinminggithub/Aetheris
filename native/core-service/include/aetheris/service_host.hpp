#pragma once

#include "aetheris/ipc_server.hpp"
#include "aetheris/recovery_policy.hpp"
#include "aetheris/service_config.hpp"
#include "aetheris/service_log.hpp"
#include "aetheris/session_launcher.hpp"

#include <windows.h>

#include <chrono>
#include <mutex>
#include <optional>

namespace aetheris::service {

constexpr wchar_t service_name[] = L"AetherisCoreService";

class CoreSupervisor {
public:
    CoreSupervisor(ServiceConfig config, std::filesystem::path state_root);
    ~CoreSupervisor();
    int run(HANDLE stop_event);
    void on_session_change(DWORD event_type, DWORD session_id);

private:
    std::optional<ClientIdentity> expected_client();
    void on_ipc_message(const IpcMessage& message);
    void inspect_process();
    void launch_if_due();
    bool adopt_resume_process(const IpcMessage& message);
    bool adopt_existing_core(const SessionInfo& session);
    void persist_suppression();
    void close_process();

    ServiceConfig config_;
    std::wstring user_hash_;
    std::filesystem::path state_path_;
    ServiceLog log_;
    Win32SessionApi sessions_;
    RecoveryPolicy recovery_;
    std::unique_ptr<IpcServer> ipc_;
    std::mutex mutex_;
    HANDLE core_process_ = nullptr;
    DWORD core_pid_ = 0;
    DWORD core_session_id_ = 0xffffffff;
    std::chrono::steady_clock::time_point launched_at_{};
    std::chrono::steady_clock::time_point normal_exit_at_{};
    bool normal_exit_received_ = false;
};

void WINAPI service_main(DWORD argc, wchar_t** argv);

}
