#include "aetheris/core_service_install.hpp"

#include <aclapi.h>
#include <sddl.h>
#include <wtsapi32.h>
#include <wintrust.h>
#include <softpub.h>

#include <chrono>
#include <array>
#include <filesystem>
#include <fstream>
#include <iomanip>
#include <sstream>
#include <vector>

namespace {
class ServiceHandle {
public:
    explicit ServiceHandle(SC_HANDLE handle = nullptr) : handle_(handle) {}
    ~ServiceHandle() { if (handle_) CloseServiceHandle(handle_); }
    ServiceHandle(const ServiceHandle&) = delete;
    ServiceHandle& operator=(const ServiceHandle&) = delete;
    ServiceHandle(ServiceHandle&& other) noexcept : handle_(other.handle_) { other.handle_ = nullptr; }
    ServiceHandle& operator=(ServiceHandle&& other) noexcept {
        if (this != &other) {
            if (handle_) CloseServiceHandle(handle_);
            handle_ = other.handle_;
            other.handle_ = nullptr;
        }
        return *this;
    }
    SC_HANDLE get() const { return handle_; }
    explicit operator bool() const { return handle_ != nullptr; }
private:
    SC_HANDLE handle_;
};

aetheris::ServiceResult win32_error(const wchar_t* stage, DWORD error = GetLastError()) {
    std::wostringstream value;
    value << stage << L'_' << std::hex << std::setw(8) << std::setfill(L'0') << error;
    return {false, value.str()};
}

std::wstring active_console_user_sid() {
    const DWORD session_id = WTSGetActiveConsoleSessionId();
    if (session_id == 0xffffffff) return {};
    wchar_t* user_raw = nullptr;
    wchar_t* domain_raw = nullptr;
    DWORD user_bytes = 0;
    DWORD domain_bytes = 0;
    if (!WTSQuerySessionInformationW(WTS_CURRENT_SERVER_HANDLE, session_id, WTSUserName, &user_raw, &user_bytes) ||
        !user_raw || !*user_raw) {
        if (user_raw) WTSFreeMemory(user_raw);
        return {};
    }
    WTSQuerySessionInformationW(WTS_CURRENT_SERVER_HANDLE, session_id, WTSDomainName, &domain_raw, &domain_bytes);
    const std::wstring account = domain_raw && *domain_raw ? std::wstring(domain_raw) + L"\\" + user_raw : std::wstring(user_raw);
    WTSFreeMemory(user_raw);
    if (domain_raw) WTSFreeMemory(domain_raw);
    DWORD sid_size = 0;
    DWORD name_size = 0;
    SID_NAME_USE use{};
    LookupAccountNameW(nullptr, account.c_str(), nullptr, &sid_size, nullptr, &name_size, &use);
    std::vector<unsigned char> buffer(sid_size);
    std::vector<wchar_t> resolved_domain(name_size);
    if (!sid_size || !LookupAccountNameW(nullptr, account.c_str(), buffer.data(), &sid_size,
        resolved_domain.data(), &name_size, &use)) return {};
    wchar_t* raw = nullptr;
    if (!ConvertSidToStringSidW(buffer.data(), &raw)) return {};
    std::wstring sid(raw);
    LocalFree(raw);
    return sid;
}

bool write_registry_string(HKEY key, const wchar_t* name, const std::wstring& value) {
    return RegSetValueExW(key, name, 0, REG_SZ, reinterpret_cast<const BYTE*>(value.c_str()),
        static_cast<DWORD>((value.size() + 1) * sizeof(wchar_t))) == ERROR_SUCCESS;
}

bool protect_service_directory(const std::filesystem::path& directory) {
    PSECURITY_DESCRIPTOR descriptor = nullptr;
    if (!ConvertStringSecurityDescriptorToSecurityDescriptorW(
        L"D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", SDDL_REVISION_1, &descriptor, nullptr)) return false;
    const bool success = SetFileSecurityW(
        directory.c_str(), DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION, descriptor) != FALSE;
    LocalFree(descriptor);
    return success;
}

std::wstring quoted_service_path(const std::filesystem::path& executable) {
    return L"\"" + executable.wstring() + L"\"";
}

std::wstring unquoted_service_path(std::wstring value) {
    if (value.size() >= 2 && value.front() == L'\"' && value.back() == L'\"') return value.substr(1, value.size() - 2);
    return value;
}

void delete_core_registry() {
    HKEY parent = nullptr;
    if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, L"SOFTWARE\\Aetheris", 0,
        KEY_WRITE | KEY_WOW64_64KEY, &parent) == ERROR_SUCCESS) {
        RegDeleteTreeW(parent, L"Core");
        RegCloseKey(parent);
    }
}

bool files_equal(const std::filesystem::path& left, const std::filesystem::path& right) {
    std::error_code error;
    if (std::filesystem::file_size(left, error) != std::filesystem::file_size(right, error) || error) return false;
    std::ifstream a(left, std::ios::binary);
    std::ifstream b(right, std::ios::binary);
    std::array<char, 64 * 1024> a_buffer{};
    std::array<char, 64 * 1024> b_buffer{};
    while (a && b) {
        a.read(a_buffer.data(), a_buffer.size());
        b.read(b_buffer.data(), b_buffer.size());
        if (a.gcount() != b.gcount() || !std::equal(a_buffer.begin(), a_buffer.begin() + a.gcount(), b_buffer.begin())) return false;
    }
    return a.eof() && b.eof();
}

bool copy_verified(const std::filesystem::path& source, const std::filesystem::path& destination) {
    const HANDLE input = CreateFileW(source.c_str(), GENERIC_READ, FILE_SHARE_READ, nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (input == INVALID_HANDLE_VALUE) return false;
    const HANDLE output = CreateFileW(destination.c_str(), GENERIC_WRITE, 0, nullptr, CREATE_ALWAYS,
        FILE_ATTRIBUTE_NORMAL | FILE_FLAG_WRITE_THROUGH, nullptr);
    if (output == INVALID_HANDLE_VALUE) { CloseHandle(input); return false; }
    std::array<unsigned char, 64 * 1024> buffer{};
    bool copied = true;
    for (;;) {
        DWORD read = 0;
        if (!ReadFile(input, buffer.data(), static_cast<DWORD>(buffer.size()), &read, nullptr)) { copied = false; break; }
        if (read == 0) break;
        DWORD written = 0;
        if (!WriteFile(output, buffer.data(), read, &written, nullptr) || written != read) { copied = false; break; }
    }
    const bool flushed = copied && FlushFileBuffers(output) != FALSE;
    CloseHandle(output);
    CloseHandle(input);
    if (!flushed || !files_equal(source, destination)) { DeleteFileW(destination.c_str()); return false; }
    return true;
}

std::filesystem::path service_state_directory() {
    wchar_t program_data[32768]{};
    const DWORD length = GetEnvironmentVariableW(L"ProgramData", program_data, static_cast<DWORD>(std::size(program_data)));
    if (!length || length >= std::size(program_data)) return LR"(C:\ProgramData\Aetheris\Service)";
    return std::filesystem::path(program_data) / L"Aetheris" / L"Service";
}

bool same_path(const std::wstring& left, const std::filesystem::path& right) {
    std::error_code error;
    const auto actual = std::filesystem::weakly_canonical(left, error);
    if (error) return false;
    const auto expected = std::filesystem::weakly_canonical(right, error);
    return !error && _wcsicmp(actual.c_str(), expected.c_str()) == 0;
}

bool wait_for_state(SC_HANDLE service, DWORD target, DWORD timeout_ms) {
    const auto deadline = std::chrono::steady_clock::now() + std::chrono::milliseconds(timeout_ms);
    SERVICE_STATUS_PROCESS status{};
    DWORD bytes = 0;
    do {
        if (!QueryServiceStatusEx(service, SC_STATUS_PROCESS_INFO, reinterpret_cast<BYTE*>(&status), sizeof(status), &bytes)) return false;
        if (status.dwCurrentState == target) return true;
        Sleep(250);
    } while (std::chrono::steady_clock::now() < deadline);
    return false;
}
}

namespace aetheris {
CoreServiceSpec core_service_spec(const std::filesystem::path& service_executable) {
    return {service_executable, L"LocalSystem", SERVICE_AUTO_START, true, 60000};
}

const wchar_t* core_service_binary_sddl() {
    return L"D:P(A;;FRFX;;;SY)(A;;FA;;;BA)";
}

ServiceResult prepare_core_service_directory(const std::filesystem::path& service_directory) {
    if (!service_directory.is_absolute()) return {false, L"service_directory_invalid"};
    std::error_code error;
    std::filesystem::create_directories(service_directory, error);
    if (error) return {false, L"service_directory_create_failed_" + std::to_wstring(error.value())};
    if (!protect_service_directory(service_directory)) return win32_error(L"service_acl_failed");
    const auto probe = service_directory / L".aetheris-write-probe";
    const HANDLE file = CreateFileW(
        probe.c_str(), GENERIC_WRITE | DELETE, 0, nullptr, CREATE_ALWAYS,
        FILE_ATTRIBUTE_TEMPORARY | FILE_FLAG_DELETE_ON_CLOSE, nullptr);
    if (file == INVALID_HANDLE_VALUE) return win32_error(L"service_directory_write_failed");
    CloseHandle(file);
    return {true, L""};
}

ServiceResult deploy_core_service_binary(
    const std::filesystem::path& source,
    const std::filesystem::path& destination) {
    if (!source.is_absolute() || !std::filesystem::is_regular_file(source) || !destination.is_absolute()) {
        return {false, L"service_binary_path_invalid"};
    }
    const auto prepared = prepare_core_service_directory(destination.parent_path());
    if (!prepared.success) return prepared;
    const auto staged = std::filesystem::path(destination.wstring() + L".new");
    const auto previous = std::filesystem::path(destination.wstring() + L".previous");
    DeleteFileW(staged.c_str());
    if (!CopyFileW(source.c_str(), staged.c_str(), FALSE)) return win32_error(L"service_binary_copy_failed");
    DeleteFileW(previous.c_str());
    if (std::filesystem::exists(destination) && !MoveFileExW(destination.c_str(), previous.c_str(), MOVEFILE_WRITE_THROUGH)) {
        const auto failed = win32_error(L"service_binary_backup_failed");
        DeleteFileW(staged.c_str());
        return failed;
    }
    if (!MoveFileExW(staged.c_str(), destination.c_str(), MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)) {
        const DWORD move_error = GetLastError();
        if (move_error == ERROR_ACCESS_DENIED && copy_verified(staged, destination)) {
            DeleteFileW(staged.c_str());
            return {true, L""};
        }
        const auto failed = win32_error(L"service_binary_commit_failed", move_error);
        if (std::filesystem::exists(previous)) MoveFileExW(previous.c_str(), destination.c_str(), MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH);
        DeleteFileW(staged.c_str());
        return failed;
    }
    return {true, L""};
}

ServiceResult restore_core_service_binary(const std::filesystem::path& destination) {
    const auto previous = std::filesystem::path(destination.wstring() + L".previous");
    if (!std::filesystem::exists(previous)) {
        DeleteFileW(destination.c_str());
        return {true, L""};
    }
    DeleteFileW(destination.c_str());
    if (!MoveFileExW(previous.c_str(), destination.c_str(), MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)) {
        return win32_error(L"service_binary_restore_failed");
    }
    return {true, L""};
}

ServiceResult verify_core_service_binary_access(const std::filesystem::path& executable) {
    PACL dacl = nullptr;
    PSECURITY_DESCRIPTOR descriptor = nullptr;
    const DWORD security_error = GetNamedSecurityInfoW(
        const_cast<wchar_t*>(executable.c_str()), SE_FILE_OBJECT, DACL_SECURITY_INFORMATION,
        nullptr, nullptr, &dacl, nullptr, &descriptor);
    if (security_error != ERROR_SUCCESS) return win32_error(L"service_access_read_failed", security_error);
    BYTE sid_buffer[SECURITY_MAX_SID_SIZE]{};
    DWORD sid_size = sizeof(sid_buffer);
    if (!CreateWellKnownSid(WinLocalSystemSid, nullptr, sid_buffer, &sid_size)) {
        LocalFree(descriptor);
        return win32_error(L"service_system_sid_failed");
    }
    TRUSTEEW trustee{};
    trustee.TrusteeForm = TRUSTEE_IS_SID;
    trustee.TrusteeType = TRUSTEE_IS_USER;
    trustee.ptstrName = reinterpret_cast<wchar_t*>(sid_buffer);
    ACCESS_MASK rights = 0;
    const DWORD rights_error = GetEffectiveRightsFromAclW(dacl, &trustee, &rights);
    LocalFree(descriptor);
    if (rights_error != ERROR_SUCCESS) return win32_error(L"service_access_check_failed", rights_error);
    GENERIC_MAPPING mapping{FILE_GENERIC_READ, FILE_GENERIC_WRITE, FILE_GENERIC_EXECUTE, FILE_ALL_ACCESS};
    MapGenericMask(&rights, &mapping);
    const ACCESS_MASK required = FILE_GENERIC_READ | FILE_GENERIC_EXECUTE;
    return (rights & required) == required ? ServiceResult{true, L""} : ServiceResult{false, L"service_system_access_missing"};
}

ServiceResult verify_core_service_binary_signature(const std::filesystem::path& executable) {
    WINTRUST_FILE_INFO file{};
    file.cbStruct = sizeof(file);
    file.pcwszFilePath = executable.c_str();
    WINTRUST_DATA trust{};
    trust.cbStruct = sizeof(trust);
    trust.dwUIChoice = WTD_UI_NONE;
    trust.fdwRevocationChecks = WTD_REVOKE_NONE;
    trust.dwUnionChoice = WTD_CHOICE_FILE;
    trust.pFile = &file;
    trust.dwStateAction = WTD_STATEACTION_VERIFY;
    trust.dwProvFlags = WTD_CACHE_ONLY_URL_RETRIEVAL;
    GUID action = WINTRUST_ACTION_GENERIC_VERIFY_V2;
    const LONG result = WinVerifyTrust(nullptr, &action, &trust);
    trust.dwStateAction = WTD_STATEACTION_CLOSE;
    WinVerifyTrust(nullptr, &action, &trust);
    return result == ERROR_SUCCESS ? ServiceResult{true, L""} : win32_error(L"service_signature_invalid", static_cast<DWORD>(result));
}

ServiceResult install_core_service(
    const std::filesystem::path& service_executable,
    const std::filesystem::path& core_root,
    const std::wstring& version) {
    if (!service_executable.is_absolute() || !std::filesystem::is_regular_file(service_executable)) return {false, L"service_executable_invalid"};
    if (!core_root.is_absolute() || !std::filesystem::is_regular_file(core_root / L"AetherisCore.exe") ||
        !std::filesystem::is_regular_file(core_root / L"config" / L"aetheris.json")) return {false, L"core_root_invalid"};
    if (version.empty()) return {false, L"service_version_invalid"};
    const auto prepared = prepare_core_service_directory(service_executable.parent_path());
    if (!prepared.success) return prepared;
    const auto state_directory = service_state_directory();
    std::error_code state_error;
    std::filesystem::create_directories(state_directory, state_error);
    if (state_error || !protect_service_directory(state_directory)) return win32_error(L"service_state_acl_failed");
    const auto sid = active_console_user_sid();
    if (sid.empty()) return {false, L"install_user_sid_failed"};

    HKEY raw_key = nullptr;
    if (RegCreateKeyExW(HKEY_LOCAL_MACHINE, L"SOFTWARE\\Aetheris\\Core", 0, nullptr, 0,
        KEY_SET_VALUE | KEY_WOW64_64KEY, nullptr, &raw_key, nullptr) != ERROR_SUCCESS) return win32_error(L"service_registry_failed");
    const bool registry_ok = write_registry_string(raw_key, L"CoreRoot", core_root.wstring()) &&
        write_registry_string(raw_key, L"InstallUserSid", sid) && write_registry_string(raw_key, L"ExpectedVersion", version);
    RegCloseKey(raw_key);
    if (!registry_ok) return {false, L"service_registry_write_failed"};

    ServiceHandle manager(OpenSCManagerW(nullptr, nullptr, SC_MANAGER_CONNECT | SC_MANAGER_CREATE_SERVICE));
    if (!manager) return win32_error(L"scm_open_failed");
    const auto service_path = quoted_service_path(service_executable);
    ServiceHandle service(CreateServiceW(
        manager.get(), core_service_name, L"Aetheris Core Service",
        SERVICE_QUERY_STATUS | SERVICE_START | SERVICE_STOP | SERVICE_CHANGE_CONFIG | DELETE,
        SERVICE_WIN32_OWN_PROCESS, SERVICE_AUTO_START, SERVICE_ERROR_NORMAL,
        service_path.c_str(), nullptr, nullptr, nullptr, L"LocalSystem", nullptr));
    if (!service && GetLastError() == ERROR_SERVICE_EXISTS) {
        service = ServiceHandle(OpenServiceW(manager.get(), core_service_name,
            SERVICE_QUERY_STATUS | SERVICE_START | SERVICE_STOP | SERVICE_CHANGE_CONFIG | DELETE));
        if (!service) return win32_error(L"service_open_failed");
        if (!ChangeServiceConfigW(service.get(), SERVICE_NO_CHANGE, SERVICE_AUTO_START, SERVICE_NO_CHANGE,
            service_path.c_str(), nullptr, nullptr, nullptr, L"LocalSystem", nullptr, L"Aetheris Core Service")) {
            return win32_error(L"service_update_failed");
        }
    } else if (!service) {
        return win32_error(L"service_create_failed");
    }
    SERVICE_DELAYED_AUTO_START_INFO delayed{TRUE};
    if (!ChangeServiceConfig2W(service.get(), SERVICE_CONFIG_DELAYED_AUTO_START_INFO, &delayed)) return win32_error(L"service_delayed_start_failed");
    SC_ACTION actions[] = {{SC_ACTION_RESTART, 60000}, {SC_ACTION_RESTART, 60000}, {SC_ACTION_RESTART, 60000}};
    SERVICE_FAILURE_ACTIONSW failures{86400, nullptr, nullptr, 3, actions};
    if (!ChangeServiceConfig2W(service.get(), SERVICE_CONFIG_FAILURE_ACTIONS, &failures)) return win32_error(L"service_recovery_failed");
    SERVICE_FAILURE_ACTIONS_FLAG flag{TRUE};
    if (!ChangeServiceConfig2W(service.get(), SERVICE_CONFIG_FAILURE_ACTIONS_FLAG, &flag)) return win32_error(L"service_recovery_flag_failed");
    return {true, L""};
}

ServiceResult start_core_service() {
    ServiceHandle manager(OpenSCManagerW(nullptr, nullptr, SC_MANAGER_CONNECT));
    if (!manager) return win32_error(L"scm_open_failed");
    ServiceHandle service(OpenServiceW(manager.get(), core_service_name, SERVICE_START | SERVICE_QUERY_STATUS));
    if (!service) return win32_error(L"service_open_failed");
    if (!StartServiceW(service.get(), 0, nullptr) && GetLastError() != ERROR_SERVICE_ALREADY_RUNNING) return win32_error(L"service_start_failed");
    return wait_for_state(service.get(), SERVICE_RUNNING, 30000) ? ServiceResult{true, L""} : ServiceResult{false, L"service_start_timeout"};
}

ServiceResult stop_core_service(DWORD timeout_ms) {
    ServiceHandle manager(OpenSCManagerW(nullptr, nullptr, SC_MANAGER_CONNECT));
    if (!manager) return win32_error(L"scm_open_failed");
    ServiceHandle service(OpenServiceW(manager.get(), core_service_name, SERVICE_STOP | SERVICE_QUERY_STATUS));
    if (!service && GetLastError() == ERROR_SERVICE_DOES_NOT_EXIST) return {true, L""};
    if (!service) return win32_error(L"service_open_failed");
    SERVICE_STATUS status{};
    if (!ControlService(service.get(), SERVICE_CONTROL_STOP, &status) && GetLastError() != ERROR_SERVICE_NOT_ACTIVE) return win32_error(L"service_stop_failed");
    return wait_for_state(service.get(), SERVICE_STOPPED, timeout_ms) ? ServiceResult{true, L""} : ServiceResult{false, L"service_stop_timeout"};
}

ServiceResult remove_core_service() {
    const auto stopped = stop_core_service();
    if (!stopped.success) return stopped;
    ServiceHandle manager(OpenSCManagerW(nullptr, nullptr, SC_MANAGER_CONNECT));
    if (!manager) return win32_error(L"scm_open_failed");
    ServiceHandle service(OpenServiceW(manager.get(), core_service_name, DELETE));
    if (!service && GetLastError() == ERROR_SERVICE_DOES_NOT_EXIST) {
        delete_core_registry();
        return {true, L""};
    }
    if (!service) return win32_error(L"service_open_failed");
    if (!DeleteService(service.get()) && GetLastError() != ERROR_SERVICE_MARKED_FOR_DELETE) return win32_error(L"service_delete_failed");
    delete_core_registry();
    return {true, L""};
}

CoreServiceState query_core_service(const std::filesystem::path& expected_executable) {
    ServiceHandle manager(OpenSCManagerW(nullptr, nullptr, SC_MANAGER_CONNECT));
    if (!manager) return CoreServiceState::error;
    ServiceHandle service(OpenServiceW(manager.get(), core_service_name, SERVICE_QUERY_STATUS | SERVICE_QUERY_CONFIG));
    if (!service && GetLastError() == ERROR_SERVICE_DOES_NOT_EXIST) return CoreServiceState::missing;
    if (!service) return CoreServiceState::error;
    if (!expected_executable.empty()) {
        DWORD bytes = 0;
        QueryServiceConfigW(service.get(), nullptr, 0, &bytes);
        std::vector<unsigned char> buffer(bytes);
        auto* config = reinterpret_cast<QUERY_SERVICE_CONFIGW*>(buffer.data());
        if (!bytes || !QueryServiceConfigW(service.get(), config, bytes, &bytes) || !same_path(unquoted_service_path(config->lpBinaryPathName), expected_executable)) return CoreServiceState::mismatch;
    }
    SERVICE_STATUS_PROCESS status{};
    DWORD bytes = 0;
    if (!QueryServiceStatusEx(service.get(), SC_STATUS_PROCESS_INFO, reinterpret_cast<BYTE*>(&status), sizeof(status), &bytes)) return CoreServiceState::error;
    if (status.dwCurrentState == SERVICE_RUNNING) return CoreServiceState::running;
    if (status.dwCurrentState == SERVICE_START_PENDING) return CoreServiceState::start_pending;
    if (status.dwCurrentState == SERVICE_STOP_PENDING) return CoreServiceState::stop_pending;
    return CoreServiceState::stopped;
}
}
