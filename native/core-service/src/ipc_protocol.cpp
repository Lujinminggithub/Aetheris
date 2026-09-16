#include "aetheris/ipc_protocol.hpp"

#include <windows.h>

#include <charconv>
#include <limits>
#include <map>
#include <set>

namespace {
using Fields = std::map<std::string, std::string>;

void skip_space(std::string_view text, std::size_t& index) {
    while (index < text.size() && (text[index] == ' ' || text[index] == '\t' || text[index] == '\r' || text[index] == '\n')) ++index;
}

std::string parse_string(std::string_view text, std::size_t& index) {
    if (index >= text.size() || text[index++] != '"') throw aetheris::service::ProtocolError("string_expected");
    std::string value;
    while (index < text.size()) {
        const char character = text[index++];
        if (character == '"') return value;
        if (character == '\\' || static_cast<unsigned char>(character) < 0x20) throw aetheris::service::ProtocolError("escaped_string_rejected");
        value += character;
    }
    throw aetheris::service::ProtocolError("unterminated_string");
}

Fields parse_flat_object(std::string_view text) {
    std::size_t index = 0;
    skip_space(text, index);
    if (index >= text.size() || text[index++] != '{') throw aetheris::service::ProtocolError("object_expected");
    Fields fields;
    for (;;) {
        skip_space(text, index);
        if (index < text.size() && text[index] == '}') { ++index; break; }
        const auto key = parse_string(text, index);
        skip_space(text, index);
        if (index >= text.size() || text[index++] != ':') throw aetheris::service::ProtocolError("colon_expected");
        skip_space(text, index);
        std::string value;
        if (index < text.size() && text[index] == '"') {
            value = parse_string(text, index);
        } else {
            const auto start = index;
            while (index < text.size() && text[index] >= '0' && text[index] <= '9') ++index;
            if (start == index) throw aetheris::service::ProtocolError("scalar_expected");
            value.assign(text.substr(start, index - start));
        }
        if (!fields.emplace(key, value).second) throw aetheris::service::ProtocolError("duplicate_field");
        skip_space(text, index);
        if (index < text.size() && text[index] == ',') { ++index; continue; }
        if (index < text.size() && text[index] == '}') { ++index; break; }
        throw aetheris::service::ProtocolError("delimiter_expected");
    }
    skip_space(text, index);
    if (index != text.size()) throw aetheris::service::ProtocolError("trailing_data");
    return fields;
}

std::uint64_t number(const Fields& fields, const std::string& name) {
    const auto found = fields.find(name);
    if (found == fields.end()) throw aetheris::service::ProtocolError("field_missing");
    std::uint64_t value = 0;
    const auto parsed = std::from_chars(found->second.data(), found->second.data() + found->second.size(), value);
    if (parsed.ec != std::errc{} || parsed.ptr != found->second.data() + found->second.size()) throw aetheris::service::ProtocolError("number_invalid");
    return value;
}

aetheris::service::IpcType parse_type(const std::string& value) {
    using aetheris::service::IpcType;
    if (value == "ready") return IpcType::ready;
    if (value == "heartbeat") return IpcType::heartbeat;
    if (value == "normal_exit") return IpcType::normal_exit;
    if (value == "resume") return IpcType::resume;
    if (value == "status_request") return IpcType::status_request;
    if (value == "status_response") return IpcType::status_response;
    if (value == "prepare_update") return IpcType::prepare_update;
    if (value == "update_ready") return IpcType::update_ready;
    if (value == "session_stop") return IpcType::session_stop;
    throw aetheris::service::ProtocolError("message_type_invalid");
}
}

namespace aetheris::service {
IpcMessage parse_ipc_message(std::string_view body) {
    if (body.empty() || body.size() > max_ipc_frame) throw ProtocolError("frame_size_invalid");
    if (MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, body.data(), static_cast<int>(body.size()), nullptr, 0) <= 0) {
        throw ProtocolError("utf8_invalid");
    }
    const auto fields = parse_flat_object(body);
    static const std::set<std::string> allowed{"version", "type", "pid", "session_id", "monotonic_ms"};
    for (const auto& [name, value] : fields) if (!allowed.count(name)) throw ProtocolError("unknown_field");
    if (fields.size() != allowed.size()) throw ProtocolError("field_missing");
    const auto version = number(fields, "version");
    const auto pid = number(fields, "pid");
    const auto session = number(fields, "session_id");
    if (version != 1 || pid == 0 || pid > (std::numeric_limits<DWORD>::max)() || session > (std::numeric_limits<DWORD>::max)()) {
        throw ProtocolError("field_range_invalid");
    }
    return {1, parse_type(fields.at("type")), static_cast<DWORD>(pid), static_cast<DWORD>(session), number(fields, "monotonic_ms")};
}

std::string ipc_type_name(IpcType type) {
    switch (type) {
    case IpcType::ready: return "ready";
    case IpcType::heartbeat: return "heartbeat";
    case IpcType::normal_exit: return "normal_exit";
    case IpcType::resume: return "resume";
    case IpcType::status_request: return "status_request";
    case IpcType::status_response: return "status_response";
    case IpcType::prepare_update: return "prepare_update";
    case IpcType::update_ready: return "update_ready";
    case IpcType::session_stop: return "session_stop";
    }
    throw ProtocolError("message_type_invalid");
}

std::string serialize_ipc_message(const IpcMessage& message) {
    return "{\"version\":1,\"type\":\"" + ipc_type_name(message.type) + "\",\"pid\":" +
        std::to_string(message.pid) + ",\"session_id\":" + std::to_string(message.session_id) +
        ",\"monotonic_ms\":" + std::to_string(message.monotonic_ms) + "}";
}

bool client_identity_matches(const ClientIdentity& expected, const ClientIdentity& actual) {
    return actual.pid != 0 && (expected.pid == 0 || expected.pid == actual.pid) && expected.session_id == actual.session_id &&
        !expected.user_sid.empty() && _wcsicmp(expected.user_sid.c_str(), actual.user_sid.c_str()) == 0;
}
}
