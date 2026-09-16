#include "aetheris/recovery_policy.hpp"

#include <algorithm>

namespace aetheris::service {
RecoveryPolicy::RecoveryPolicy(
    Clock clock,
    std::chrono::seconds restart_delay,
    std::size_t failure_limit,
    std::chrono::minutes failure_window,
    std::chrono::minutes backoff,
    std::chrono::minutes healthy_reset,
    SuppressionSnapshot snapshot)
    : clock_(std::move(clock)), restart_delay_(restart_delay), failure_limit_(failure_limit),
      failure_window_(failure_window), backoff_(backoff), healthy_reset_(healthy_reset),
      suppression_(snapshot) {}

void RecoveryPolicy::on_user_available(std::uint32_t session_id, std::uint64_t boot_epoch_100ns) {
    session_id_ = session_id;
    boot_epoch_100ns_ = boot_epoch_100ns;
    if (suppression_.suppressed && (suppression_.session_id != session_id || suppression_.boot_epoch_100ns != boot_epoch_100ns)) {
        suppression_ = {};
    }
    if (suppression_.suppressed) {
        state_ = SupervisorState::user_suppressed;
        return;
    }
    if (state_ == SupervisorState::waiting_for_user || state_ == SupervisorState::user_suppressed) {
        state_ = SupervisorState::starting;
    }
}

void RecoveryPolicy::on_ready() {
    last_ready_ = clock_();
    state_ = SupervisorState::running;
}

void RecoveryPolicy::on_heartbeat() {
    if (state_ == SupervisorState::starting) {
        on_ready();
        return;
    }
    if (state_ == SupervisorState::running && last_ready_ != std::chrono::steady_clock::time_point{} &&
        clock_() - last_ready_ >= healthy_reset_) {
        failures_.clear();
    }
    last_ready_ = clock_();
}

void RecoveryPolicy::on_authenticated_normal_exit(std::uint32_t session_id, std::uint64_t boot_epoch_100ns) {
    suppression_.session_id = session_id;
    suppression_.boot_epoch_100ns = boot_epoch_100ns;
    suppression_.suppressed = true;
    suppression_.exited_at = std::chrono::system_clock::now();
    state_ = SupervisorState::user_suppressed;
}

void RecoveryPolicy::on_resume(std::uint32_t session_id, std::uint64_t boot_epoch_100ns) {
    suppression_ = {};
    session_id_ = session_id;
    boot_epoch_100ns_ = boot_epoch_100ns;
    state_ = SupervisorState::starting;
}

void RecoveryPolicy::on_process_exit(DWORD, bool graceful) {
    if (graceful && suppression_.suppressed) {
        state_ = SupervisorState::user_suppressed;
        return;
    }
    suppression_ = {};
    const auto now = clock_();
    failures_.erase(std::remove_if(failures_.begin(), failures_.end(), [&](const auto value) {
        return now - value > failure_window_;
    }), failures_.end());
    failures_.push_back(now);
    if (failures_.size() >= failure_limit_) {
        state_ = SupervisorState::crash_loop_backoff;
        restart_at_ = now + backoff_;
    } else {
        state_ = SupervisorState::restart_wait;
        restart_at_ = now + restart_delay_;
    }
}

void RecoveryPolicy::on_logoff(std::uint32_t session_id) {
    if (suppression_.session_id == session_id) suppression_ = {};
    session_id_ = 0;
    state_ = SupervisorState::waiting_for_user;
}

void RecoveryPolicy::on_boot_change(std::uint64_t boot_epoch_100ns) {
    if (boot_epoch_100ns_ != boot_epoch_100ns) suppression_ = {};
    boot_epoch_100ns_ = boot_epoch_100ns;
}

RecoveryDecision RecoveryPolicy::decision() const {
    const auto now = clock_();
    RecoveryDecision result{state_, false, restart_at_};
    if (state_ == SupervisorState::starting) result.launch = true;
    if ((state_ == SupervisorState::restart_wait || state_ == SupervisorState::crash_loop_backoff) && now >= restart_at_) result.launch = true;
    return result;
}

const SuppressionSnapshot& RecoveryPolicy::suppression() const { return suppression_; }
}
