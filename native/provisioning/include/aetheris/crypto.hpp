#pragma once

#include <cstdint>
#include <string>
#include <vector>

namespace aetheris {
std::string sha256_hex(const std::vector<std::uint8_t>& value);
std::vector<std::uint8_t> dpapi_protect(const std::vector<std::uint8_t>& value);
std::vector<std::uint8_t> dpapi_unprotect(const std::vector<std::uint8_t>& value);
}
