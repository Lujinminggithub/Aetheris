#include "aetheris/service_host.hpp"

#include "aetheris/state_store.hpp"

#include <windows.h>
#include <bcrypt.h>
#include <wtsapi32.h>
#include <tlhelp32.h>

#include <array>
#include <filesystem>
#include <iomanip>
#include <sstream>
#include <vector>

namespace {
SERVICE_STATUS_HANDLE status_handle = nullptr;
SERVICE_STATUS status{};
HANDLE stop_event = nullptr;
HANDLE instance_mutex = nullptr;
aetheris::service::CoreSupervisor* supervisor = nullptr;

void report_status(DWORD state, DWORD error = ERROR_SUCCESS, DWORD wait_hint = 0) {
    status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
    status.dwCurrentState = state;
    status.dwWin32ExitCode = error;
    status.dwWaitHint = wait_hint;
    status.dwControlsAccepted = state == SERVICE_RUNNING
        ? SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN | SERVICE_ACCEPT_SESSIONCHANGE : 0;
    static DWORD checkpoint = 1;
    status.dwCheckPoint = state == SERVICE_START_PENDING || state == SERVICE_STOP_PENDING ? checkpoint++ : 0;
    if (status_handle) SetServiceStatus(status_handle, &status);
}

DWORD WINAPI control_handler(DWORD control, DWORD event_type, void* event_data, void*) {
    if (control == SERVICE_CONTROL_STOP || control == SERVICE_CONTROL_SHUTDOWN) {
        report_status(SERVICE_STOP_PENDING, ERROR_SUCCESS, 15000);
        if (stop_event) SetEvent(stop_event);
        return NO_ERROR;
    }
    if (control == SERVICE_CONTROL_SESSIONCHANGE && supervisor) {
        const auto* notification = static_cast<WTSSESSION_NOTIFICATION*>(event_data);
        supervisor->on_session_change(event_type, notification ? notification->dwSessionId : 0xffffffff);
        return NO_ERROR;
    }
    if (control == SERVICE_CONTROL_INTERROGATE) return NO_ERROR;
    return ERROR_CALL_NOT_IMPLEMENTED;
}

std::filesystem::path program_data_root() {
    wchar_t value[32768]{};
    const DWORD length = GetEnvironmentVariableW(L"ProgramData", value, static_cast<DWORD>(std::size(value)));
    if (!length || length >= std::size(value)) return LR"(C:\ProgramData\Aetheris\Service)";
    return std::filesystem::path(value) / L"Aetheris" / L"Service";
}

bool same_file(const std::filesystem::path& left, const std::filesystem::path& right) {
    std::error_code error;
    const auto canonical_left = std::filesystem::weakly_canonical(left, error);
    if (error) return false;
    const auto canonical_right = std::filesystem::weakly_canonical(right, error);
    return !error && _wcsicmp(canonical_left.c_str(), canonical_right.c_str()) == 0;
}

std::wstring sid_hash(const std::wstring& sid) {
    BCRYPT_ALG_HANDLE algorithm = nullptr;
    BCRYPT_HASH_HANDLE hash = nullptr;
    DWORD object_size = 0;
    DWORD result_size = 0;
    std::vector<unsigned char> object;
    std::array<unsigned char, 32> digest{};
    if (BCryptOpenAlgorithmProvider(&algorithm, BCRYPT_SHA256_ALGORITHM, nullptr, 0) < 0) return {};
    if (BCryptGetProperty(algorithm, BCRYPT_OBJECT_LENGTH, reinterpret_cast<PUCHAR>(&object_size), sizeof(object_size), &result_size, 0) < 0) goto cleanup;
    object.resize(object_size);
    if (BCryptCreateHash(algorithm, &hash, object.data(), object_size, nullptr, 0, 0) < 0) goto cleanup;
    if (BCryptHashData(hash, reinterpret_cast<PUCHAR>(const_cast<wchar_t*>(sid.data())), static_cast<ULONG>(sid.size() * sizeof(wchar_t)), 0) < 0) goto cleanup;
    if (BCryptFinishHash(hash, digest.data(), static_cast<ULONG>(digest.size()), 0) < 0) goto cleanup;
    {
        std::wostringstream value;
        for (std::size_t index = 0; index < 8; ++index) value << std::hex << std::setw(2) << std::setfill(L'0') << static_cast<unsigned>(digest[index]);
        if (hash) BCryptDestroyHash(hash);
        BCryptCloseAlgorithmProvider(algorithm, 0);
        return value.str();
    }
cleanup:
    if (hash) BCryptDestroyHash(hash);
    BCryptCloseAlgorithmProvider(algorithm, 0);
    return {};
}
}

namespace aetheris::service {
CoreSupervisor::CoreSupervisor(ServiceConfig config, std::filesystem::path state_root)
    : config_(std::move(config)), user_hash_(sid_hash(config_.install_user_sid)), state_path_(state_root / L"state.json"),
      log_(state_root / L"service.log"), recovery_([] { return std::chrono::steady_clock::now(); }) {
    const auto restored = load_state(state_path_, current_boot_epoch_100ns());
    if (restored && !user_hash_.empty() && restored->user_hash == user_hash_) recovery_ = RecoveryPolicy([] { return std::chrono::steady_clock::now(); }, std::chrono::seconds(60),
        5, std::chrono::minutes(10), std::chrono::minutes(15), std::chrono::minutes(30), restored->suppression);
    ipc_ = std::make_unique<IpcServer>(
        config_.install_user_sid,
        [this] { return expected_client(); },
        [this](const auto& message) { on_ipc_message(message); });
}

CoreSupervisor::~CoreSupervisor() {
    if (ipc_) ipc_->stop();
    close_process();
}

std::optional<ClientIdentity> CoreSupervisor::expected_client() {
    std::lock_guard lock(mutex_);
    const auto active = active_console_session(sessions_, config_.install_user_sid);
    if (active.state != SessionState::active_console) return std::nullopt;
    const DWORD expected_pid = recovery_.decision().state == SupervisorState::starting ? 0 : core_pid_;
    return ClientIdentity{expected_pid, active.id, config_.install_user_sid};
}

bool CoreSupervisor::adopt_resume_process(const IpcMessage& message) {
    if (core_process_ && core_pid_ == message.pid) return true;
    if (core_process_ && recovery_.decision().state != SupervisorState::starting) return false;
    HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION | SYNCHRONIZE, FALSE, message.pid);
    if (!process) return false;
    std::vector<wchar_t> path(32768, L'\0');
    DWORD length = static_cast<DWORD>(path.size());
    const bool path_ok = QueryFullProcessImageNameW(process, 0, path.data(), &length) != FALSE &&
        same_file(std::filesystem::path(std::wstring(path.data(), length)), config_.core_executable);
    const SessionInfo session{SessionState::active_console, message.session_id, config_.install_user_sid};
    if (!path_ok || !process_matches_expected_user(process, session, config_)) {
        CloseHandle(process);
        return false;
    }
    if (core_process_) CloseHandle(core_process_);
    core_process_ = process;
    core_pid_ = message.pid;
    core_session_id_ = message.session_id;
    launched_at_ = std::chrono::steady_clock::now();
    recovery_.on_resume(core_session_id_, current_boot_epoch_100ns());
    persist_suppression();
    return true;
}

bool CoreSupervisor::adopt_existing_core(const SessionInfo& session) {
    const HANDLE snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (snapshot == INVALID_HANDLE_VALUE) return false;
    PROCESSENTRY32W entry{};
    entry.dwSize = sizeof(entry);
    bool adopted = false;
    if (Process32FirstW(snapshot, &entry)) {
        do {
            if (_wcsicmp(entry.szExeFile, L"AetherisCore.exe") != 0) continue;
            HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION | SYNCHRONIZE, FALSE, entry.th32ProcessID);
            if (!process) continue;
            std::vector<wchar_t> path(32768, L'\0');
            DWORD length = static_cast<DWORD>(path.size());
            const bool path_matches = QueryFullProcessImageNameW(process, 0, path.data(), &length) != FALSE &&
                same_file(std::filesystem::path(std::wstring(path.data(), length)), config_.core_executable);
            if (path_matches && process_matches_expected_user(process, session, config_)) {
                core_process_ = process;
                core_pid_ = entry.th32ProcessID;
                core_session_id_ = session.id;
                launched_at_ = std::chrono::steady_clock::now();
                recovery_.on_resume(core_session_id_, current_boot_epoch_100ns());
                log_.write(ServiceEventCode::core_launch, {{core_session_id_}, {core_pid_}, {}, {}, {}});
                adopted = true;
                break;
            }
            CloseHandle(process);
        } while (Process32NextW(snapshot, &entry));
    }
    CloseHandle(snapshot);
    return adopted;
}

void CoreSupervisor::on_ipc_message(const IpcMessage& message) {
    std::lock_guard lock(mutex_);
    if ((message.type == IpcType::resume || message.type == IpcType::ready || message.type == IpcType::heartbeat) &&
        message.pid != core_pid_ && !adopt_resume_process(message)) return;
    if (message.type == IpcType::resume) return;
    if (message.pid != core_pid_ || message.session_id != core_session_id_) return;
    if (message.type == IpcType::ready) {
        recovery_.on_ready();
        log_.write(ServiceEventCode::core_ready, {{core_session_id_}, {core_pid_}, {}, {}, {}});
    } else if (message.type == IpcType::heartbeat) {
        recovery_.on_heartbeat();
    } else if (message.type == IpcType::normal_exit) {
        normal_exit_received_ = true;
        normal_exit_at_ = std::chrono::steady_clock::now();
        recovery_.on_authenticated_normal_exit(core_session_id_, current_boot_epoch_100ns());
        persist_suppression();
    }
}

void CoreSupervisor::persist_suppression() {
    PersistedSupervisorState state;
    state.user_hash = user_hash_;
    state.suppression = recovery_.suppression();
    save_state_atomic(state_path_, state);
}

void CoreSupervisor::close_process() {
    if (core_process_) CloseHandle(core_process_);
    core_process_ = nullptr;
    core_pid_ = 0;
    core_session_id_ = 0xffffffff;
}

void CoreSupervisor::inspect_process() {
    std::lock_guard lock(mutex_);
    if (!core_process_) return;
    const DWORD wait = WaitForSingleObject(core_process_, 0);
    if (wait == WAIT_TIMEOUT) {
        if (recovery_.decision().state == SupervisorState::starting &&
            std::chrono::steady_clock::now() - launched_at_ > std::chrono::seconds(60)) {
            TerminateProcess(core_process_, ERROR_TIMEOUT);
        }
        return;
    }
    DWORD exit_code = 1;
    GetExitCodeProcess(core_process_, &exit_code);
    const bool graceful = normal_exit_received_ && exit_code == 0 &&
        std::chrono::steady_clock::now() - normal_exit_at_ <= std::chrono::seconds(10);
    log_.write(ServiceEventCode::core_exit, {{core_session_id_}, {core_pid_}, {exit_code}, {}, {}});
    close_process();
    recovery_.on_process_exit(exit_code, graceful);
    persist_suppression();
    normal_exit_received_ = false;
}

void CoreSupervisor::launch_if_due() {
    std::lock_guard lock(mutex_);
    if (core_process_) return;
    const auto session = active_console_session(sessions_, config_.install_user_sid);
    if (session.state == SessionState::none) return;
    if (adopt_existing_core(session)) return;
    recovery_.on_user_available(session.id, current_boot_epoch_100ns());
    if (!recovery_.decision().launch) return;
    auto launched = launch_core_for_session(config_, session, sessions_);
    if (!launched.started) {
        recovery_.on_process_exit(1, false);
        return;
    }
    if (!process_matches_expected_user(launched.process, session, config_)) {
        TerminateProcess(launched.process, ERROR_ACCESS_DENIED);
        CloseHandle(launched.process);
        recovery_.on_process_exit(ERROR_ACCESS_DENIED, false);
        return;
    }
    core_process_ = launched.process;
    core_pid_ = launched.process_id;
    core_session_id_ = session.id;
    launched_at_ = std::chrono::steady_clock::now();
    log_.write(ServiceEventCode::core_launch, {{core_session_id_}, {core_pid_}, {}, {}, {}});
}

int CoreSupervisor::run(HANDLE service_stop_event) {
    log_.write(ServiceEventCode::service_start);
    ipc_->start();
    while (WaitForSingleObject(service_stop_event, 1000) == WAIT_TIMEOUT) {
        inspect_process();
        launch_if_due();
    }
    ipc_->stop();
    std::lock_guard lock(mutex_);
    if (core_process_ && WaitForSingleObject(core_process_, 0) == WAIT_TIMEOUT) {
        TerminateProcess(core_process_, ERROR_PROCESS_ABORTED);
        WaitForSingleObject(core_process_, 5000);
    }
    close_process();
    log_.write(ServiceEventCode::service_stop);
    return 0;
}

void CoreSupervisor::on_session_change(DWORD event_type, DWORD session_id) {
    std::lock_guard lock(mutex_);
    if (event_type == WTS_SESSION_LOGOFF) {
        recovery_.on_logoff(session_id);
        persist_suppression();
    }
}

void WINAPI service_main(DWORD, wchar_t**) {
    status_handle = RegisterServiceCtrlHandlerExW(service_name, control_handler, nullptr);
    if (!status_handle) return;
    report_status(SERVICE_START_PENDING, ERROR_SUCCESS, 15000);
    stop_event = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    if (!stop_event) { report_status(SERVICE_STOPPED, GetLastError()); return; }
    instance_mutex = CreateMutexW(nullptr, FALSE, L"Global\\AetherisCoreService");
    if (!instance_mutex || GetLastError() == ERROR_ALREADY_EXISTS) {
        const DWORD error = instance_mutex ? ERROR_SERVICE_ALREADY_RUNNING : GetLastError();
        if (instance_mutex) CloseHandle(instance_mutex);
        instance_mutex = nullptr;
        CloseHandle(stop_event);
        stop_event = nullptr;
        report_status(SERVICE_STOPPED, error);
        return;
    }
    try {
        Win32RegistryReader registry;
        auto config = load_service_config(registry);
        CoreSupervisor host(std::move(config), program_data_root());
        supervisor = &host;
        report_status(SERVICE_RUNNING);
        host.run(stop_event);
        supervisor = nullptr;
        CloseHandle(stop_event);
        stop_event = nullptr;
        CloseHandle(instance_mutex);
        instance_mutex = nullptr;
        report_status(SERVICE_STOPPED);
    } catch (...) {
        supervisor = nullptr;
        CloseHandle(stop_event);
        stop_event = nullptr;
        if (instance_mutex) CloseHandle(instance_mutex);
        instance_mutex = nullptr;
        report_status(SERVICE_STOPPED, ERROR_SERVICE_SPECIFIC_ERROR);
    }
}
}
