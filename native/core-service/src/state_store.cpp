#include "aetheris/state_store.hpp"

#include <windows.h>

#include <charconv>
#include <fstream>
#include <limits>
#include <string_view>

namespace {
constexpr std::uintmax_t max_state_bytes = 64 * 1024;

std::optional<std::uint64_t> unsigned_field(const std::string& text, std::string_view name) {
    const auto marker = std::string("\"") + std::string(name) + "\":";
    const auto start = text.find(marker);
    if (start == std::string::npos) return std::nullopt;
    const char* first = text.data() + start + marker.size();
    const char* last = first;
    while (last != text.data() + text.size() && *last >= '0' && *last <= '9') ++last;
    if (first == last) return std::nullopt;
    std::uint64_t value = 0;
    const auto result = std::from_chars(first, last, value);
    return result.ec == std::errc{} ? std::optional<std::uint64_t>(value) : std::nullopt;
}

std::optional<std::string> string_field(const std::string& text, std::string_view name) {
    const auto marker = std::string("\"") + std::string(name) + "\":\"";
    const auto start = text.find(marker);
    if (start == std::string::npos) return std::nullopt;
    const auto value_start = start + marker.size();
    const auto end = text.find('"', value_start);
    if (end == std::string::npos) return std::nullopt;
    const auto value = text.substr(value_start, end - value_start);
    if (value.find_first_not_of("0123456789abcdef") != std::string::npos || value.size() > 64) return std::nullopt;
    return value;
}

bool boolean_field(const std::string& text, std::string_view name, bool& value) {
    const auto marker = std::string("\"") + std::string(name) + "\":";
    const auto start = text.find(marker);
    if (start == std::string::npos) return false;
    const auto value_start = start + marker.size();
    if (text.compare(value_start, 4, "true") == 0) { value = true; return true; }
    if (text.compare(value_start, 5, "false") == 0) { value = false; return true; }
    return false;
}

std::uint64_t system_time_millis(const std::chrono::system_clock::time_point value) {
    return static_cast<std::uint64_t>(std::chrono::duration_cast<std::chrono::milliseconds>(value.time_since_epoch()).count());
}
}

namespace aetheris::service {
std::string serialize_state(const PersistedSupervisorState& state) {
    std::string user_hash;
    user_hash.reserve(state.user_hash.size());
    for (const wchar_t value : state.user_hash) {
        if (!((value >= L'0' && value <= L'9') || (value >= L'a' && value <= L'f'))) return {};
        user_hash.push_back(static_cast<char>(value));
    }
    return "{\"schema_version\":" + std::to_string(state.schema_version) +
        ",\"user_hash\":\"" + user_hash +
        "\",\"session_id\":" + std::to_string(state.suppression.session_id) +
        ",\"boot_epoch_100ns\":" + std::to_string(state.suppression.boot_epoch_100ns) +
        ",\"exited_at_ms\":" + std::to_string(system_time_millis(state.suppression.exited_at)) +
        ",\"suppressed\":" + (state.suppression.suppressed ? "true" : "false") + "}";
}

std::optional<PersistedSupervisorState> parse_state(
    const std::string& text,
    std::uint64_t current_boot_epoch,
    std::uint64_t boot_tolerance_100ns) {
    if (text.empty() || text.size() > max_state_bytes || text.front() != '{' || text.back() != '}') return std::nullopt;
    const auto schema = unsigned_field(text, "schema_version");
    const auto user_hash = string_field(text, "user_hash");
    const auto session = unsigned_field(text, "session_id");
    const auto boot = unsigned_field(text, "boot_epoch_100ns");
    const auto exited = unsigned_field(text, "exited_at_ms");
    bool suppressed = false;
    if (!schema || *schema != 1 || !user_hash || !session || *session > (std::numeric_limits<DWORD>::max)() ||
        !boot || !exited || !boolean_field(text, "suppressed", suppressed)) return std::nullopt;
    const auto difference = *boot > current_boot_epoch ? *boot - current_boot_epoch : current_boot_epoch - *boot;
    if (difference > boot_tolerance_100ns) suppressed = false;
    PersistedSupervisorState state;
    state.user_hash.assign(user_hash->begin(), user_hash->end());
    state.suppression.session_id = static_cast<DWORD>(*session);
    state.suppression.boot_epoch_100ns = *boot;
    state.suppression.exited_at = std::chrono::system_clock::time_point(std::chrono::milliseconds(*exited));
    state.suppression.suppressed = suppressed;
    return state;
}

bool save_state_atomic(const std::filesystem::path& path, const PersistedSupervisorState& state) {
    std::error_code error;
    std::filesystem::create_directories(path.parent_path(), error);
    if (error) return false;
    const auto temporary = path.wstring() + L".tmp";
    const auto body = serialize_state(state);
    const HANDLE file = CreateFileW(temporary.c_str(), GENERIC_WRITE, 0, nullptr, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (file == INVALID_HANDLE_VALUE) return false;
    DWORD written = 0;
    const bool ok = WriteFile(file, body.data(), static_cast<DWORD>(body.size()), &written, nullptr) &&
        written == body.size() && FlushFileBuffers(file);
    CloseHandle(file);
    if (!ok || !MoveFileExW(temporary.c_str(), path.c_str(), MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)) {
        DeleteFileW(temporary.c_str());
        return false;
    }
    return true;
}

std::optional<PersistedSupervisorState> load_state(const std::filesystem::path& path, std::uint64_t current_boot_epoch) {
    std::error_code error;
    if (!std::filesystem::is_regular_file(path, error) || std::filesystem::file_size(path, error) > max_state_bytes) return std::nullopt;
    std::ifstream input(path, std::ios::binary);
    if (!input) return std::nullopt;
    const std::string body((std::istreambuf_iterator<char>(input)), std::istreambuf_iterator<char>());
    return parse_state(body, current_boot_epoch);
}
}
