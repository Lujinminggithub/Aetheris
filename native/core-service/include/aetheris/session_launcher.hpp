#pragma once

#include "aetheris/service_config.hpp"

#include <windows.h>

#include <filesystem>
#include <optional>
#include <string>

namespace aetheris::service {

enum class SessionState { none, active_console };

struct SessionInfo {
    SessionState state = SessionState::none;
    DWORD id = 0xffffffff;
    std::wstring user_sid;
};

struct LaunchResult {
    bool started = false;
    DWORD process_id = 0;
    HANDLE process = nullptr;
    std::wstring error_code;
};

class SessionApi {
public:
    virtual ~SessionApi() = default;
    virtual DWORD active_console_session_id() = 0;
    virtual bool is_active_session(DWORD session_id) = 0;
    virtual std::optional<std::wstring> user_sid(DWORD session_id) = 0;
    virtual LaunchResult launch_as_user(
        DWORD session_id,
        const std::filesystem::path& application,
        const std::wstring& command_line,
        const std::filesystem::path& working_directory,
        const std::wstring& desktop) = 0;
};

class Win32SessionApi final : public SessionApi {
public:
    DWORD active_console_session_id() override;
    bool is_active_session(DWORD session_id) override;
    std::optional<std::wstring> user_sid(DWORD session_id) override;
    LaunchResult launch_as_user(
        DWORD session_id,
        const std::filesystem::path& application,
        const std::wstring& command_line,
        const std::filesystem::path& working_directory,
        const std::wstring& desktop) override;
};

SessionInfo active_console_session(SessionApi& api, const std::wstring& expected_user_sid);
std::wstring core_command_line(const ServiceConfig& config, DWORD session_id);
LaunchResult launch_core_for_session(const ServiceConfig& config, const SessionInfo& session, SessionApi& api);
bool process_matches_expected_user(HANDLE process, const SessionInfo& session, const ServiceConfig& config);

}
