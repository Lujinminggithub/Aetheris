#include "aetheris/recovery_policy.hpp"
#include "aetheris/service_config.hpp"
#include "aetheris/session_launcher.hpp"
#include "aetheris/ipc_protocol.hpp"
#include "aetheris/service_log.hpp"
#include "aetheris/state_store.hpp"

#include <filesystem>
#include <iostream>
#include <map>
#include <stdexcept>

namespace {
void require(bool condition, const char* message) {
    if (!condition) throw std::runtime_error(message);
}

class FakeRegistry final : public aetheris::service::RegistryReader {
public:
    std::map<std::wstring, std::wstring> values;
    std::optional<std::wstring> read_string(const std::wstring& name) const override {
        auto found = values.find(name);
        return found == values.end() ? std::nullopt : std::optional<std::wstring>(found->second);
    }
};

class FakeSessionApi final : public aetheris::service::SessionApi {
public:
    DWORD console_id = 2;
    bool active = true;
    std::optional<std::wstring> sid = L"S-1-5-21-test";
    int launch_count = 0;
    std::filesystem::path last_application;
    std::wstring last_command;
    std::filesystem::path last_working;
    std::wstring last_desktop;

    DWORD active_console_session_id() override { return console_id; }
    bool is_active_session(DWORD) override { return active; }
    std::optional<std::wstring> user_sid(DWORD) override { return sid; }
    aetheris::service::LaunchResult launch_as_user(
        DWORD, const std::filesystem::path& application, const std::wstring& command,
        const std::filesystem::path& working, const std::wstring& desktop) override {
        ++launch_count;
        last_application = application;
        last_command = command;
        last_working = working;
        last_desktop = desktop;
        return {true, 42, reinterpret_cast<HANDLE>(1), L""};
    }
};
}

int wmain() {
    try {
        FakeRegistry invalid;
        invalid.values = {{L"CoreRoot", L"..\\Core"}, {L"InstallUserSid", L"S-1-5-21"}, {L"ExpectedVersion", L"0.4.8"}};
        try { (void)aetheris::service::load_service_config(invalid); require(false, "relative config accepted"); }
        catch (const aetheris::service::ConfigError&) {}

        std::chrono::steady_clock::time_point now{};
        aetheris::service::RecoveryPolicy policy([&] { return now; });
        policy.on_user_available(2, 100);
        require(policy.decision().launch, "active user did not launch");
        policy.on_ready();
        for (int index = 0; index < 5; ++index) {
            policy.on_process_exit(0xc0000005, false);
            now += std::chrono::seconds(61);
        }
        require(policy.decision().state == aetheris::service::SupervisorState::crash_loop_backoff, "failure backoff missing");
        now += std::chrono::minutes(15);
        require(policy.decision().launch, "backoff did not expire");

        policy.on_user_available(2, 100);
        policy.on_authenticated_normal_exit(2, 100);
        policy.on_process_exit(0, true);
        now += std::chrono::minutes(2);
        require(!policy.decision().launch && policy.decision().state == aetheris::service::SupervisorState::user_suppressed, "normal exit restarted");
        policy.on_logoff(2);
        require(policy.decision().state == aetheris::service::SupervisorState::waiting_for_user, "logoff did not clear suppression");

        const auto state_root = std::filesystem::temp_directory_path() / L"aetheris-service-state-test";
        std::filesystem::remove_all(state_root);
        aetheris::service::PersistedSupervisorState state;
        state.user_hash = L"0123456789abcdef";
        state.suppression = {2, 100, std::chrono::system_clock::now(), true};
        const auto state_path = state_root / L"state.json";
        require(aetheris::service::save_state_atomic(state_path, state), "state save failed");
        const auto restored = aetheris::service::load_state(state_path, 100);
        require(restored && restored->suppression.suppressed && restored->suppression.session_id == 2, "state restore failed");
        require(!aetheris::service::parse_state("not-json", 100), "invalid state accepted");
        const auto rebooted = aetheris::service::load_state(state_path, 100000000);
        require(rebooted && !rebooted->suppression.suppressed, "boot change did not clear suppression");
        std::filesystem::remove_all(state_root);

        aetheris::service::ServiceConfig launch_config{
            LR"(D:\Aetheris)", LR"(D:\Aetheris\AetherisCore.exe)",
            LR"(D:\Aetheris\config\aetheris.json)", L"S-1-5-21-test", L"0.4.8"};
        FakeSessionApi session_api;
        const auto active_session = aetheris::service::active_console_session(session_api, launch_config.install_user_sid);
        require(active_session.state == aetheris::service::SessionState::active_console, "console session not selected");
        const auto launched = aetheris::service::launch_core_for_session(launch_config, active_session, session_api);
        require(launched.started && session_api.launch_count == 1, "core not launched");
        require(session_api.last_application == launch_config.core_executable, "application changed");
        require(session_api.last_working == launch_config.core_root, "working directory changed");
        require(session_api.last_desktop == L"winsta0\\default", "interactive desktop missing");
        require(session_api.last_command.find(L"--service-session 2") != std::wstring::npos, "session argument missing");
        aetheris::service::RecoveryPolicy adopted_policy([&] { return now; });
        adopted_policy.on_user_available(2, 100);
        adopted_policy.on_heartbeat();
        require(adopted_policy.decision().state == aetheris::service::SupervisorState::running, "heartbeat did not confirm adopted core");
        session_api.active = false;
        require(aetheris::service::active_console_session(session_api, launch_config.install_user_sid).state == aetheris::service::SessionState::none, "inactive session accepted");
        session_api.active = true;
        session_api.sid = L"S-1-5-21-other";
        require(aetheris::service::active_console_session(session_api, launch_config.install_user_sid).state == aetheris::service::SessionState::none, "wrong user accepted");

        const auto message = aetheris::service::parse_ipc_message(R"({"version":1,"type":"ready","pid":42,"session_id":2,"monotonic_ms":100})");
        require(message.type == aetheris::service::IpcType::ready && message.pid == 42, "IPC message parse failed");
        require(aetheris::service::parse_ipc_message(aetheris::service::serialize_ipc_message(message)).session_id == 2, "IPC round trip failed");
        try { (void)aetheris::service::parse_ipc_message(std::string(aetheris::service::max_ipc_frame + 1, 'x')); require(false, "oversized IPC accepted"); }
        catch (const aetheris::service::ProtocolError&) {}
        try { (void)aetheris::service::parse_ipc_message(R"({"version":1,"type":"run_path","pid":42,"session_id":2,"monotonic_ms":100})"); require(false, "unknown IPC type accepted"); }
        catch (const aetheris::service::ProtocolError&) {}
        require(aetheris::service::client_identity_matches({42, 2, L"S-1-5-21-test"}, {42, 2, L"s-1-5-21-TEST"}), "valid IPC identity rejected");
        require(aetheris::service::client_identity_matches({0, 2, L"S-1-5-21-test"}, {77, 2, L"S-1-5-21-test"}), "resume candidate rejected");
        require(!aetheris::service::client_identity_matches({42, 2, L"S-1-5-21-test"}, {42, 3, L"S-1-5-21-test"}), "wrong IPC session accepted");

        const auto log_root = std::filesystem::temp_directory_path() / L"aetheris-service-log-test";
        std::filesystem::remove_all(log_root);
        aetheris::service::ServiceLog service_log(log_root / L"service.log", 80, 2);
        for (int index = 0; index < 10; ++index) service_log.write(aetheris::service::ServiceEventCode::core_exit, {{2}, {42}, {1}, {}, {}});
        require(std::filesystem::exists(log_root / L"service.log.1"), "service log did not rotate");
        require(!std::filesystem::exists(log_root / L"service.log.3"), "service log exceeded retention");
        std::filesystem::remove_all(log_root);
        std::wcout << L"core service task 1 tests passed\n";
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 1;
    }
}
