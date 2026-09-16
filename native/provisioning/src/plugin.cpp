#include "nsis_plugin.hpp"
#include "aetheris/project_scan.hpp"
#include "aetheris/provision.hpp"
#include "aetheris/file.hpp"
#include "aetheris/json.hpp"
#include "aetheris/core_service_install.hpp"

#include <windows.h>

#include <stdexcept>
#include <vector>

namespace {
aetheris::ProjectScanner project_scanner;
std::vector<aetheris::ProjectSource> selected_projects;

void return_status(nsis::stack_t** stack, int string_size, const wchar_t* status) {
    nsis::push(stack, string_size, status);
}

const wchar_t* state_name(aetheris::ScanState state) {
    switch (state) {
    case aetheris::ScanState::running: return L"running";
    case aetheris::ScanState::complete: return L"complete";
    case aetheris::ScanState::warning: return L"warning";
    default: return L"idle";
    }
}

std::wstring machine_guid() {
    wchar_t value[256]{};
    DWORD size = sizeof(value);
    const auto status = RegGetValueW(
        HKEY_LOCAL_MACHINE, L"SOFTWARE\\Microsoft\\Cryptography", L"MachineGuid",
        RRF_RT_REG_SZ | RRF_SUBKEY_WOW6464KEY, nullptr, value, &size);
    if (status != ERROR_SUCCESS || !value[0]) throw std::runtime_error("machine_guid_unavailable");
    return value;
}

std::wstring hostname() {
    wchar_t value[MAX_COMPUTERNAME_LENGTH + 1]{};
    DWORD size = MAX_COMPUTERNAME_LENGTH + 1;
    if (!GetComputerNameW(value, &size)) throw std::runtime_error("hostname_unavailable");
    return std::wstring(value, size);
}
}

NSIS_PLUGIN_FUNCTION(StartProjectScan) {
    const auto root = nsis::pop(stack);
    try {
        project_scanner.start(root, 8, 100);
        return_status(stack, string_size, L"started");
    } catch (...) {
        return_status(stack, string_size, L"scan_start_failed");
    }
}

NSIS_PLUGIN_FUNCTION(PollProjectScan) {
    const auto snapshot = project_scanner.snapshot();
    const auto result = std::wstring(state_name(snapshot.state)) + L"|" +
        std::to_wstring(snapshot.visited) + L"|" + std::to_wstring(snapshot.projects.size()) + L"|" +
        (snapshot.truncated ? L"1" : L"0") + L"|" + snapshot.error_code;
    nsis::push(stack, string_size, result);
}

NSIS_PLUGIN_FUNCTION(PopulateProjectList) {
    const auto raw_handle = nsis::pop(stack);
    const auto handle = reinterpret_cast<HWND>(static_cast<UINT_PTR>(_wcstoui64(raw_handle.c_str(), nullptr, 0)));
    const auto snapshot = project_scanner.snapshot();
    if (handle) {
        SendMessageW(handle, LB_RESETCONTENT, 0, 0);
        for (const auto& project : snapshot.projects) {
            const auto label = L"[" + project.vcs + L"] " + project.path.wstring();
            SendMessageW(handle, LB_ADDSTRING, 0, reinterpret_cast<LPARAM>(label.c_str()));
        }
    }
    nsis::push(stack, string_size, std::to_wstring(snapshot.projects.size()));
}
NSIS_PLUGIN_FUNCTION(ProvisionDevice) {
    auto version = nsis::pop(stack);
    auto gateway = nsis::pop(stack);
    auto enrollment = nsis::pop(stack);
    auto user = nsis::pop(stack);
    auto install_root = nsis::pop(stack);
    try {
        aetheris::ProvisionRequest request{
            gateway, enrollment, user, version, hostname(), machine_guid(), install_root, selected_projects,
        };
        const auto result = aetheris::provision_device(request);
        if (!enrollment.empty()) SecureZeroMemory(enrollment.data(), enrollment.size() * sizeof(wchar_t));
        if (!request.enrollment_code.empty()) SecureZeroMemory(request.enrollment_code.data(), request.enrollment_code.size() * sizeof(wchar_t));
        if (result.success) {
            nsis::push(stack, string_size, L"ok|" + result.tenant_id + L"|" + result.subject_id + L"|" + result.device_id);
        } else {
            nsis::push(stack, string_size, L"error|" + result.error_code);
        }
    } catch (const std::exception&) {
        if (!enrollment.empty()) SecureZeroMemory(enrollment.data(), enrollment.size() * sizeof(wchar_t));
        nsis::push(stack, string_size, L"error|provisioning_failed");
    }
}
NSIS_PLUGIN_FUNCTION(WriteConfiguration) {
    const auto raw_handle = nsis::pop(stack);
    const auto handle = reinterpret_cast<HWND>(static_cast<UINT_PTR>(_wcstoui64(raw_handle.c_str(), nullptr, 0)));
    selected_projects.clear();
    if (handle) {
        const auto count = static_cast<int>(SendMessageW(handle, LB_GETSELCOUNT, 0, 0));
        if (count > 0) {
            std::vector<int> selected(static_cast<std::size_t>(count));
            SendMessageW(handle, LB_GETSELITEMS, count, reinterpret_cast<LPARAM>(selected.data()));
            for (const int index : selected) {
                const auto length = static_cast<int>(SendMessageW(handle, LB_GETTEXTLEN, index, 0));
                if (length <= 0) continue;
                std::wstring label(static_cast<std::size_t>(length + 1), L'\0');
                SendMessageW(handle, LB_GETTEXT, index, reinterpret_cast<LPARAM>(label.data()));
                label.resize(static_cast<std::size_t>(length));
                const auto marker = label.find(L"] ");
                if (label.size() > 6 && label.front() == L'[' && marker != std::wstring::npos) {
                    selected_projects.push_back(aetheris::ProjectSource{label.substr(marker + 2), label.substr(1, marker - 1)});
                }
            }
        }
    }
    nsis::push(stack, string_size, std::to_wstring(selected_projects.size()));
}
NSIS_PLUGIN_FUNCTION(VerifyCoreStatus) {
    const auto expected_version = nsis::pop(stack);
    const std::filesystem::path install_root(nsis::pop(stack));
    try {
        const auto config_raw = aetheris::read_bytes(install_root / L"config" / L"aetheris.json");
        const std::string config(config_raw.begin(), config_raw.end());
        const auto device = aetheris::json_get_string(config, "device_id");
        const bool valid = aetheris::verify_core_status(
            install_root / L"data" / L"core-status.json",
            expected_version,
            std::wstring(device.begin(), device.end()));
        return_status(stack, string_size, valid ? L"ok" : L"pending");
    } catch (...) {
        return_status(stack, string_size, L"pending");
    }
}
NSIS_PLUGIN_FUNCTION(RevokeDevice) {
    const std::filesystem::path install_root(nsis::pop(stack));
    const auto result = aetheris::revoke_device(install_root);
    if (result.local_exit_requested && result.credential_revoked) return_status(stack, string_size, L"ok");
    else return_status(stack, string_size, result.error_code.empty() ? L"revocation_pending" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(RequestCoreExit) {
    const auto install_root = std::filesystem::path(nsis::pop(stack));
    return_status(stack, string_size, aetheris::request_existing_core_exit(install_root) ? L"ok" : L"core_exit_failed");
}

NSIS_PLUGIN_FUNCTION(VerifyExistingDevice) {
    const auto gateway = nsis::pop(stack);
    const auto install_root = std::filesystem::path(nsis::pop(stack));
    const auto result = aetheris::verify_existing_device(install_root, gateway);
    switch (result.state) {
    case aetheris::ExistingDeviceState::reusable: return_status(stack, string_size, L"reusable"); break;
    case aetheris::ExistingDeviceState::credential_invalid: return_status(stack, string_size, L"credential_invalid"); break;
    case aetheris::ExistingDeviceState::server_unavailable: return_status(stack, string_size, L"server_unavailable"); break;
    default: return_status(stack, string_size, L"missing"); break;
    }
}

NSIS_PLUGIN_FUNCTION(UpdateExistingCoreVersion) {
    const auto version = nsis::pop(stack);
    const auto install_root = std::filesystem::path(nsis::pop(stack));
    return_status(stack, string_size, aetheris::update_existing_core_version(install_root, version) ? L"ok" : L"config_update_failed");
}

NSIS_PLUGIN_FUNCTION(InstallCoreService) {
    const auto version = nsis::pop(stack);
    const auto core_root = std::filesystem::path(nsis::pop(stack));
    const auto service_executable = std::filesystem::path(nsis::pop(stack));
    const auto result = aetheris::install_core_service(service_executable, core_root, version);
    return_status(stack, string_size, result.success ? L"ok" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(PrepareCoreServiceDirectory) {
    const auto result = aetheris::prepare_core_service_directory(std::filesystem::path(nsis::pop(stack)));
    return_status(stack, string_size, result.success ? L"ok" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(DeployCoreServiceBinary) {
    const auto destination = std::filesystem::path(nsis::pop(stack));
    const auto source = std::filesystem::path(nsis::pop(stack));
    const auto result = aetheris::deploy_core_service_binary(source, destination);
    return_status(stack, string_size, result.success ? L"ok" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(RestoreCoreServiceBinary) {
    const auto result = aetheris::restore_core_service_binary(std::filesystem::path(nsis::pop(stack)));
    return_status(stack, string_size, result.success ? L"ok" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(VerifyCoreServiceBinaryAccess) {
    const auto result = aetheris::verify_core_service_binary_access(std::filesystem::path(nsis::pop(stack)));
    return_status(stack, string_size, result.success ? L"ready" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(VerifyCoreServiceBinarySignature) {
    const auto result = aetheris::verify_core_service_binary_signature(std::filesystem::path(nsis::pop(stack)));
    return_status(stack, string_size, result.success ? L"valid" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(QueryCoreService) {
    const auto state = aetheris::query_core_service(std::filesystem::path(nsis::pop(stack)));
    switch (state) {
    case aetheris::CoreServiceState::running: return_status(stack, string_size, L"running"); break;
    case aetheris::CoreServiceState::stopped: return_status(stack, string_size, L"stopped"); break;
    case aetheris::CoreServiceState::start_pending: return_status(stack, string_size, L"start_pending"); break;
    case aetheris::CoreServiceState::stop_pending: return_status(stack, string_size, L"stop_pending"); break;
    case aetheris::CoreServiceState::missing: return_status(stack, string_size, L"missing"); break;
    case aetheris::CoreServiceState::mismatch: return_status(stack, string_size, L"mismatch"); break;
    default: return_status(stack, string_size, L"error"); break;
    }
}

NSIS_PLUGIN_FUNCTION(StartCoreService) {
    const auto result = aetheris::start_core_service();
    return_status(stack, string_size, result.success ? L"ok" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(StopCoreService) {
    const auto result = aetheris::stop_core_service();
    return_status(stack, string_size, result.success ? L"ok" : result.error_code.c_str());
}

NSIS_PLUGIN_FUNCTION(RemoveCoreService) {
    const auto result = aetheris::remove_core_service();
    return_status(stack, string_size, result.success ? L"ok" : result.error_code.c_str());
}

BOOL WINAPI DllMain(HINSTANCE, DWORD, LPVOID) { return TRUE; }
