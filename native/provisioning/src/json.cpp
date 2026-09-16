#include "aetheris/json.hpp"

#include <windows.h>

#include <iomanip>
#include <sstream>
#include <stdexcept>

namespace aetheris {
std::string utf8(const std::wstring& value) {
    if (value.empty()) return {};
    const int size = WideCharToMultiByte(CP_UTF8, WC_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), nullptr, 0, nullptr, nullptr);
    if (size <= 0) throw std::runtime_error("WideCharToMultiByte size failed");
    std::string result(static_cast<std::size_t>(size), '\0');
    if (!WideCharToMultiByte(CP_UTF8, WC_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), result.data(), size, nullptr, nullptr)) {
        throw std::runtime_error("WideCharToMultiByte failed");
    }
    return result;
}

std::string json_escape(const std::wstring& value) {
    const auto source = utf8(value);
    std::ostringstream output;
    for (const unsigned char byte : source) {
        switch (byte) {
        case '"': output << "\\\""; break;
        case '\\': output << "\\\\"; break;
        case '\b': output << "\\b"; break;
        case '\f': output << "\\f"; break;
        case '\n': output << "\\n"; break;
        case '\r': output << "\\r"; break;
        case '\t': output << "\\t"; break;
        default:
            if (byte < 0x20) {
                output << "\\u" << std::hex << std::setw(4) << std::setfill('0') << static_cast<unsigned>(byte) << std::dec;
            } else {
                output << static_cast<char>(byte);
            }
        }
    }
    return output.str();
}

std::string json_get_string(const std::string& body, const std::string& key) {
    const auto marker = "\"" + key + "\"";
    auto position = body.find(marker);
    if (position == std::string::npos) return {};
    position = body.find(':', position + marker.size());
    if (position == std::string::npos) return {};
    position = body.find('"', position + 1);
    if (position == std::string::npos) return {};
    std::string result;
    bool escaped = false;
    for (++position; position < body.size(); ++position) {
        const char value = body[position];
        if (escaped) {
            if (value == 'n') result.push_back('\n');
            else if (value == 'r') result.push_back('\r');
            else if (value == 't') result.push_back('\t');
            else result.push_back(value);
            escaped = false;
        } else if (value == '\\') {
            escaped = true;
        } else if (value == '"') {
            return result;
        } else {
            result.push_back(value);
        }
    }
    return {};
}
}
