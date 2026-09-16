#pragma once

#include <chrono>
#include <cstdint>
#include <functional>
#include <string>
#include <vector>

#include <windows.h>

namespace aetheris::service {

enum class SupervisorState {
    waiting_for_user,
    starting,
    running,
    restart_wait,
    crash_loop_backoff,
    user_suppressed,
};

struct RecoveryDecision {
    SupervisorState state = SupervisorState::waiting_for_user;
    bool launch = false;
    std::chrono::steady_clock::time_point launch_at{};
};

struct SuppressionSnapshot {
    std::uint32_t session_id = 0;
    std::uint64_t boot_epoch_100ns = 0;
    std::chrono::system_clock::time_point exited_at{};
    bool suppressed = false;
};

class RecoveryPolicy {
public:
    using Clock = std::function<std::chrono::steady_clock::time_point()>;

    RecoveryPolicy(
        Clock clock,
        std::chrono::seconds restart_delay = std::chrono::seconds(60),
        std::size_t failure_limit = 5,
        std::chrono::minutes failure_window = std::chrono::minutes(10),
        std::chrono::minutes backoff = std::chrono::minutes(15),
        std::chrono::minutes healthy_reset = std::chrono::minutes(30),
        SuppressionSnapshot snapshot = {});

    void on_user_available(std::uint32_t session_id, std::uint64_t boot_epoch_100ns);
    void on_ready();
    void on_heartbeat();
    void on_authenticated_normal_exit(std::uint32_t session_id, std::uint64_t boot_epoch_100ns);
    void on_resume(std::uint32_t session_id, std::uint64_t boot_epoch_100ns);
    void on_process_exit(DWORD exit_code, bool graceful);
    void on_logoff(std::uint32_t session_id);
    void on_boot_change(std::uint64_t boot_epoch_100ns);

    RecoveryDecision decision() const;
    const SuppressionSnapshot& suppression() const;

private:
    Clock clock_;
    std::chrono::seconds restart_delay_;
    std::size_t failure_limit_;
    std::chrono::minutes failure_window_;
    std::chrono::minutes backoff_;
    std::chrono::minutes healthy_reset_;
    std::uint32_t session_id_ = 0;
    std::uint64_t boot_epoch_100ns_ = 0;
    std::chrono::steady_clock::time_point last_ready_{};
    std::chrono::steady_clock::time_point restart_at_{};
    SupervisorState state_ = SupervisorState::waiting_for_user;
    std::vector<std::chrono::steady_clock::time_point> failures_;
    SuppressionSnapshot suppression_{};
};

}
