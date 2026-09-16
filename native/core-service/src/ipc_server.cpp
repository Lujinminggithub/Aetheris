#include "aetheris/ipc_server.hpp"

#include <sddl.h>

#include <array>
#include <vector>

namespace {
constexpr wchar_t pipe_name[] = LR"(\\.\pipe\Aetheris.Core.Service.v1)";

std::wstring process_sid(HANDLE process) {
    HANDLE token = nullptr;
    if (!OpenProcessToken(process, TOKEN_QUERY, &token)) return {};
    DWORD size = 0;
    GetTokenInformation(token, TokenUser, nullptr, 0, &size);
    std::vector<unsigned char> buffer(size);
    const bool ok = size && GetTokenInformation(token, TokenUser, buffer.data(), size, &size);
    CloseHandle(token);
    if (!ok) return {};
    wchar_t* raw = nullptr;
    if (!ConvertSidToStringSidW(reinterpret_cast<TOKEN_USER*>(buffer.data())->User.Sid, &raw)) return {};
    std::wstring result(raw);
    LocalFree(raw);
    return result;
}

bool read_exact(HANDLE pipe, void* target, DWORD size) {
    auto* bytes = static_cast<unsigned char*>(target);
    DWORD offset = 0;
    while (offset < size) {
        DWORD read = 0;
        if (!ReadFile(pipe, bytes + offset, size - offset, &read, nullptr) || read == 0) return false;
        offset += read;
    }
    return true;
}
}

namespace aetheris::service {
IpcServer::IpcServer(std::wstring allowed_user_sid, ExpectedClient expected, Handler handler)
    : allowed_user_sid_(std::move(allowed_user_sid)), expected_(std::move(expected)), handler_(std::move(handler)) {}

IpcServer::~IpcServer() { stop(); }

bool IpcServer::start() {
    if (thread_.joinable()) return true;
    stopping_ = false;
    thread_ = std::thread([this] { serve(); });
    return true;
}

void IpcServer::stop() {
    stopping_ = true;
    if (thread_.joinable()) {
        HANDLE wake = CreateFileW(pipe_name, GENERIC_READ | GENERIC_WRITE, 0, nullptr, OPEN_EXISTING, 0, nullptr);
        if (wake != INVALID_HANDLE_VALUE) CloseHandle(wake);
        thread_.join();
    }
}

void IpcServer::serve() {
    const auto sddl = L"D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;" + allowed_user_sid_ + L")";
    PSECURITY_DESCRIPTOR descriptor = nullptr;
    if (!ConvertStringSecurityDescriptorToSecurityDescriptorW(sddl.c_str(), SDDL_REVISION_1, &descriptor, nullptr)) return;
    SECURITY_ATTRIBUTES security{sizeof(SECURITY_ATTRIBUTES), descriptor, FALSE};
    while (!stopping_) {
        HANDLE pipe = CreateNamedPipeW(
            pipe_name, PIPE_ACCESS_DUPLEX, PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS,
            1, max_ipc_frame + 4, max_ipc_frame + 4, 1000, &security);
        if (pipe == INVALID_HANDLE_VALUE) break;
        const bool connected = ConnectNamedPipe(pipe, nullptr) || GetLastError() == ERROR_PIPE_CONNECTED;
        if (connected && !stopping_) {
            ULONG client_pid = 0;
            DWORD client_session = 0;
            ClientIdentity actual;
            if (GetNamedPipeClientProcessId(pipe, &client_pid)) {
                actual.pid = client_pid;
                ProcessIdToSessionId(client_pid, &client_session);
                actual.session_id = client_session;
                HANDLE process = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, client_pid);
                if (process) { actual.user_sid = process_sid(process); CloseHandle(process); }
            }
            const auto expected = expected_ ? expected_() : std::nullopt;
            if (expected && client_identity_matches(*expected, actual)) {
                std::uint32_t length = 0;
                if (read_exact(pipe, &length, sizeof(length)) && length > 0 && length <= max_ipc_frame) {
                    std::string body(length, '\0');
                    if (read_exact(pipe, body.data(), length)) {
                        try {
                            const auto message = parse_ipc_message(body);
                            if (message.pid == actual.pid && message.session_id == actual.session_id) handler_(message);
                        } catch (const ProtocolError&) {}
                    }
                }
            }
        }
        FlushFileBuffers(pipe);
        DisconnectNamedPipe(pipe);
        CloseHandle(pipe);
    }
    LocalFree(descriptor);
}
}
