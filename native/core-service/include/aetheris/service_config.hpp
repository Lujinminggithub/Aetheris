#pragma once

#include <filesystem>
#include <optional>
#include <stdexcept>
#include <string>

namespace aetheris::service {

class ConfigError final : public std::runtime_error {
public:
    using std::runtime_error::runtime_error;
};

class RegistryReader {
public:
    virtual ~RegistryReader() = default;
    virtual std::optional<std::wstring> read_string(const std::wstring& name) const = 0;
};

class Win32RegistryReader final : public RegistryReader {
public:
    std::optional<std::wstring> read_string(const std::wstring& name) const override;
};

struct ServiceConfig {
    std::filesystem::path core_root;
    std::filesystem::path core_executable;
    std::filesystem::path config_file;
    std::wstring install_user_sid;
    std::wstring expected_version;
};

ServiceConfig load_service_config(const RegistryReader& registry);
std::uint64_t current_boot_epoch_100ns();

}
