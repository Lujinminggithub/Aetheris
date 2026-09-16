#pragma once

#include <filesystem>
#include <string>

#include <windows.h>

namespace aetheris {

constexpr wchar_t core_service_name[] = L"AetherisCoreService";

struct ServiceResult {
    bool success = false;
    std::wstring error_code;
};

enum class CoreServiceState { missing, stopped, start_pending, stop_pending, running, mismatch, error };

struct CoreServiceSpec {
    std::filesystem::path binary_path;
    std::wstring account;
    DWORD start_type = SERVICE_AUTO_START;
    bool delayed_auto_start = true;
    DWORD recovery_delay_ms = 60000;
};

CoreServiceSpec core_service_spec(const std::filesystem::path& service_executable);
const wchar_t* core_service_binary_sddl();
ServiceResult prepare_core_service_directory(const std::filesystem::path& service_directory);
ServiceResult deploy_core_service_binary(
    const std::filesystem::path& source,
    const std::filesystem::path& destination);
ServiceResult restore_core_service_binary(const std::filesystem::path& destination);
ServiceResult verify_core_service_binary_access(const std::filesystem::path& executable);
ServiceResult verify_core_service_binary_signature(const std::filesystem::path& executable);
ServiceResult install_core_service(
    const std::filesystem::path& service_executable,
    const std::filesystem::path& core_root,
    const std::wstring& version);
ServiceResult start_core_service();
ServiceResult stop_core_service(DWORD timeout_ms = 30000);
ServiceResult remove_core_service();
CoreServiceState query_core_service(const std::filesystem::path& expected_executable = {});

}
