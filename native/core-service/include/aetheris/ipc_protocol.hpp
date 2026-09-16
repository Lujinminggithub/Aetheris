#pragma once

#include <windows.h>

#include <cstdint>
#include <optional>
#include <stdexcept>
#include <string>
#include <string_view>

namespace aetheris::service {

constexpr std::uint32_t max_ipc_frame = 16 * 1024;

enum class IpcType {
    ready,
    heartbeat,
    normal_exit,
    resume,
    status_request,
    status_response,
    prepare_update,
    update_ready,
    session_stop,
};

struct IpcMessage {
    int version = 1;
    IpcType type = IpcType::heartbeat;
    DWORD pid = 0;
    DWORD session_id = 0;
    std::uint64_t monotonic_ms = 0;
};

struct ClientIdentity {
    DWORD pid = 0;
    DWORD session_id = 0;
    std::wstring user_sid;
};

class ProtocolError final : public std::runtime_error {
public:
    using std::runtime_error::runtime_error;
};

IpcMessage parse_ipc_message(std::string_view body);
std::string serialize_ipc_message(const IpcMessage& message);
std::string ipc_type_name(IpcType type);
bool client_identity_matches(const ClientIdentity& expected, const ClientIdentity& actual);

}
