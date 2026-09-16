#pragma once

#include <cstdint>
#include <filesystem>
#include <vector>

namespace aetheris {
void atomic_write(const std::filesystem::path& path, const std::vector<std::uint8_t>& value);
std::vector<std::uint8_t> read_bytes(const std::filesystem::path& path);
}
