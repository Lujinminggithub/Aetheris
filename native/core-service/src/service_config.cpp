#include "aetheris/service_config.hpp"

#include <windows.h>

#include <cstdint>
#include <stdexcept>

namespace {
std::filesystem::path absolute_existing_path(const std::wstring& raw, const char* name) {
    if (raw.empty()) throw aetheris::service::ConfigError(std::string(name) + "_missing");
    std::filesystem::path path(raw);
    if (!path.is_absolute() || raw.find(L'\0') != std::wstring::npos) {
        throw aetheris::service::ConfigError(std::string(name) + "_invalid");
    }
    std::error_code error;
    path = std::filesystem::weakly_canonical(path, error);
    if (error || !std::filesystem::is_directory(path)) {
        throw aetheris::service::ConfigError(std::string(name) + "_not_found");
    }
    return path;
}
}

namespace aetheris::service {
std::optional<std::wstring> Win32RegistryReader::read_string(const std::wstring& name) const {
    DWORD bytes = 0;
    const auto first = RegGetValueW(
        HKEY_LOCAL_MACHINE, L"SOFTWARE\\Aetheris\\Core", name.c_str(),
        RRF_RT_REG_SZ | RRF_SUBKEY_WOW6464KEY, nullptr, nullptr, &bytes);
    if (first != ERROR_SUCCESS || bytes < sizeof(wchar_t)) return std::nullopt;
    std::wstring value(bytes / sizeof(wchar_t), L'\0');
    DWORD type = 0;
    const auto second = RegGetValueW(
        HKEY_LOCAL_MACHINE, L"SOFTWARE\\Aetheris\\Core", name.c_str(),
        RRF_RT_REG_SZ | RRF_SUBKEY_WOW6464KEY, &type, value.data(), &bytes);
    if (second != ERROR_SUCCESS || type != REG_SZ) return std::nullopt;
    while (!value.empty() && value.back() == L'\0') value.pop_back();
    return value;
}

ServiceConfig load_service_config(const RegistryReader& registry) {
    const auto root = registry.read_string(L"CoreRoot");
    const auto sid = registry.read_string(L"InstallUserSid");
    const auto version = registry.read_string(L"ExpectedVersion");
    if (!sid || sid->empty()) throw ConfigError("install_user_sid_missing");
    if (!version || version->empty()) throw ConfigError("expected_version_missing");
    const auto core_root = absolute_existing_path(root.value_or(L""), "core_root");
    const auto core_executable = core_root / L"AetherisCore.exe";
    const auto config_file = core_root / L"config" / L"aetheris.json";
    if (!std::filesystem::is_regular_file(core_executable)) throw ConfigError("core_executable_missing");
    if (!std::filesystem::is_regular_file(config_file)) throw ConfigError("config_file_missing");
    return ServiceConfig{core_root, core_executable, config_file, *sid, *version};
}

std::uint64_t current_boot_epoch_100ns() {
    FILETIME file_time{};
    GetSystemTimeAsFileTime(&file_time);
    ULARGE_INTEGER now{};
    now.LowPart = file_time.dwLowDateTime;
    now.HighPart = file_time.dwHighDateTime;
    return now.QuadPart - GetTickCount64() * 10000ULL;
}
}
