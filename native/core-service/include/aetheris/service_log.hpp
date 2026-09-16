#pragma once

#include <filesystem>
#include <optional>
#include <string>

#include <windows.h>

namespace aetheris::service {

enum class ServiceEventCode { service_start, service_stop, core_launch, core_ready, core_exit, restart_wait, backoff, user_suppressed, ipc_rejected };

struct ServiceLogFields {
    std::optional<DWORD> session_id;
    std::optional<DWORD> core_pid;
    std::optional<DWORD> exit_code;
    std::optional<unsigned> attempt;
    std::optional<std::uint64_t> next_retry_utc;
};

class ServiceLog {
public:
    explicit ServiceLog(std::filesystem::path path, std::uintmax_t max_bytes = 1024 * 1024, unsigned backups = 5);
    void write(ServiceEventCode code, const ServiceLogFields& fields = {});
private:
    void rotate();
    std::filesystem::path path_;
    std::uintmax_t max_bytes_;
    unsigned backups_;
};

}
