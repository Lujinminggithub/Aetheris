#pragma once

#include "aetheris/recovery_policy.hpp"

#include <filesystem>
#include <optional>
#include <string>

namespace aetheris::service {

struct PersistedSupervisorState {
    int schema_version = 1;
    std::wstring user_hash;
    SuppressionSnapshot suppression;
};

std::string serialize_state(const PersistedSupervisorState& state);
std::optional<PersistedSupervisorState> parse_state(
    const std::string& text,
    std::uint64_t current_boot_epoch,
    std::uint64_t boot_tolerance_100ns = 50000000ULL);
bool save_state_atomic(const std::filesystem::path& path, const PersistedSupervisorState& state);
std::optional<PersistedSupervisorState> load_state(
    const std::filesystem::path& path,
    std::uint64_t current_boot_epoch);

}
