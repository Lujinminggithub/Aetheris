#include "aetheris/service_log.hpp"

#include <fstream>
#include <sstream>

namespace {
const char* event_name(aetheris::service::ServiceEventCode code) {
    using aetheris::service::ServiceEventCode;
    switch (code) {
    case ServiceEventCode::service_start: return "service_start";
    case ServiceEventCode::service_stop: return "service_stop";
    case ServiceEventCode::core_launch: return "core_launch";
    case ServiceEventCode::core_ready: return "core_ready";
    case ServiceEventCode::core_exit: return "core_exit";
    case ServiceEventCode::restart_wait: return "restart_wait";
    case ServiceEventCode::backoff: return "backoff";
    case ServiceEventCode::user_suppressed: return "user_suppressed";
    case ServiceEventCode::ipc_rejected: return "ipc_rejected";
    }
    return "unknown";
}
}

namespace aetheris::service {
ServiceLog::ServiceLog(std::filesystem::path path, std::uintmax_t max_bytes, unsigned backups)
    : path_(std::move(path)), max_bytes_(max_bytes), backups_(backups) {}

void ServiceLog::write(ServiceEventCode code, const ServiceLogFields& fields) {
    std::ostringstream line;
    line << "event_code=" << event_name(code);
    if (fields.session_id) line << " session_id=" << *fields.session_id;
    if (fields.core_pid) line << " core_pid=" << *fields.core_pid;
    if (fields.exit_code) line << " exit_code=" << *fields.exit_code;
    if (fields.attempt) line << " attempt=" << *fields.attempt;
    if (fields.next_retry_utc) line << " next_retry_utc=" << *fields.next_retry_utc;
    line << '\n';
    std::error_code error;
    std::filesystem::create_directories(path_.parent_path(), error);
    const auto size = std::filesystem::is_regular_file(path_, error) ? std::filesystem::file_size(path_, error) : 0;
    if (size + line.str().size() > max_bytes_) rotate();
    std::ofstream output(path_, std::ios::binary | std::ios::app);
    output << line.str();
}

void ServiceLog::rotate() {
    std::error_code error;
    std::filesystem::remove(path_.wstring() + L"." + std::to_wstring(backups_), error);
    for (unsigned index = backups_; index > 1; --index) {
        const auto source = std::filesystem::path(path_.wstring() + L"." + std::to_wstring(index - 1));
        const auto target = std::filesystem::path(path_.wstring() + L"." + std::to_wstring(index));
        if (std::filesystem::exists(source, error)) std::filesystem::rename(source, target, error);
    }
    if (std::filesystem::exists(path_, error)) std::filesystem::rename(path_, path_.wstring() + L".1", error);
}
}
