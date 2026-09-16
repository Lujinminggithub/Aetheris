#pragma once

#include <string>

namespace aetheris {
std::string utf8(const std::wstring& value);
std::string json_escape(const std::wstring& value);
std::string json_get_string(const std::string& body, const std::string& key);
}
