#include "aetheris/session_launcher.hpp"

#include <sddl.h>
#include <userenv.h>
#include <wtsapi32.h>

#include <vector>

namespace {
class Handle {
public:
    explicit Handle(HANDLE value = nullptr) : value_(value) {}
    ~Handle() { if (value_) CloseHandle(value_); }
    Handle(const Handle&) = delete;
    Handle& operator=(const Handle&) = delete;
    HANDLE get() const { return value_; }
private:
    HANDLE value_;
};

std::wstring sid_string(PSID sid) {
    wchar_t* raw = nullptr;
    if (!ConvertSidToStringSidW(sid, &raw)) return {};
    std::wstring value(raw);
    LocalFree(raw);
    return value;
}

std::wstring token_sid(HANDLE token) {
    DWORD size = 0;
    GetTokenInformation(token, TokenUser, nullptr, 0, &size);
    if (!size) return {};
    std::vector<unsigned char> buffer(size);
    if (!GetTokenInformation(token, TokenUser, buffer.data(), size, &size)) return {};
    return sid_string(reinterpret_cast<TOKEN_USER*>(buffer.data())->User.Sid);
}

std::wstring quote(const std::wstring& value) {
    std::wstring result = L"\"";
    std::size_t slashes = 0;
    for (const wchar_t character : value) {
        if (character == L'\\') {
            ++slashes;
        } else if (character == L'\"') {
            result.append(slashes * 2 + 1, L'\\');
            result += character;
            slashes = 0;
        } else {
            result.append(slashes, L'\\');
            slashes = 0;
            result += character;
        }
    }
    result.append(slashes * 2, L'\\');
    result += L'\"';
    return result;
}
}

namespace aetheris::service {
DWORD Win32SessionApi::active_console_session_id() { return WTSGetActiveConsoleSessionId(); }

bool Win32SessionApi::is_active_session(DWORD session_id) {
    WTS_CONNECTSTATE_CLASS* state = nullptr;
    DWORD bytes = 0;
    const bool success = WTSQuerySessionInformationW(
        WTS_CURRENT_SERVER_HANDLE, session_id, WTSConnectState,
        reinterpret_cast<wchar_t**>(&state), &bytes) != FALSE;
    const bool active = success && state && bytes >= sizeof(*state) && *state == WTSActive;
    if (state) WTSFreeMemory(state);
    return active;
}

std::optional<std::wstring> Win32SessionApi::user_sid(DWORD session_id) {
    HANDLE raw_token = nullptr;
    if (!WTSQueryUserToken(session_id, &raw_token)) return std::nullopt;
    Handle token(raw_token);
    const auto value = token_sid(token.get());
    return value.empty() ? std::nullopt : std::optional<std::wstring>(value);
}

LaunchResult Win32SessionApi::launch_as_user(
    DWORD session_id,
    const std::filesystem::path& application,
    const std::wstring& command_line,
    const std::filesystem::path& working_directory,
    const std::wstring& desktop) {
    HANDLE raw_user = nullptr;
    if (!WTSQueryUserToken(session_id, &raw_user)) return {false, 0, nullptr, L"user_token_failed"};
    Handle user(raw_user);
    HANDLE raw_primary = nullptr;
    if (!DuplicateTokenEx(user.get(), MAXIMUM_ALLOWED, nullptr, SecurityIdentification, TokenPrimary, &raw_primary)) {
        return {false, 0, nullptr, L"primary_token_failed"};
    }
    Handle primary(raw_primary);
    void* environment = nullptr;
    if (!CreateEnvironmentBlock(&environment, primary.get(), FALSE)) return {false, 0, nullptr, L"environment_failed"};
    std::vector<wchar_t> mutable_command(command_line.begin(), command_line.end());
    mutable_command.push_back(L'\0');
    STARTUPINFOW startup{};
    startup.cb = sizeof(startup);
    startup.lpDesktop = const_cast<wchar_t*>(desktop.c_str());
    PROCESS_INFORMATION process{};
    const bool created = CreateProcessAsUserW(
        primary.get(), application.c_str(), mutable_command.data(), nullptr, nullptr, FALSE,
        CREATE_UNICODE_ENVIRONMENT | CREATE_NO_WINDOW, environment, working_directory.c_str(),
        &startup, &process) != FALSE;
    DestroyEnvironmentBlock(environment);
    if (!created) return {false, 0, nullptr, L"core_launch_failed_" + std::to_wstring(GetLastError())};
    CloseHandle(process.hThread);
    return {true, process.dwProcessId, process.hProcess, L""};
}

SessionInfo active_console_session(SessionApi& api, const std::wstring& expected_user_sid) {
    const DWORD session_id = api.active_console_session_id();
    if (session_id == 0xffffffff || !api.is_active_session(session_id)) return {};
    const auto sid = api.user_sid(session_id);
    if (!sid || _wcsicmp(sid->c_str(), expected_user_sid.c_str()) != 0) return {};
    return {SessionState::active_console, session_id, *sid};
}

std::wstring core_command_line(const ServiceConfig& config, DWORD session_id) {
    return quote(config.core_executable.wstring()) + L" --config " + quote(config.config_file.wstring()) +
        L" --supervised --service-session " + std::to_wstring(session_id);
}

LaunchResult launch_core_for_session(const ServiceConfig& config, const SessionInfo& session, SessionApi& api) {
    if (session.state != SessionState::active_console) return {false, 0, nullptr, L"active_console_unavailable"};
    return api.launch_as_user(
        session.id, config.core_executable, core_command_line(config, session.id), config.core_root, L"winsta0\\default");
}

bool process_matches_expected_user(HANDLE process, const SessionInfo& session, const ServiceConfig& config) {
    if (!process) return false;
    DWORD process_id = GetProcessId(process);
    DWORD session_id = 0;
    if (!process_id || !ProcessIdToSessionId(process_id, &session_id) || session_id != session.id) return false;
    HANDLE raw_token = nullptr;
    if (!OpenProcessToken(process, TOKEN_QUERY, &raw_token)) return false;
    Handle token(raw_token);
    const auto sid = token_sid(token.get());
    return !sid.empty() && _wcsicmp(sid.c_str(), config.install_user_sid.c_str()) == 0;
}
}
