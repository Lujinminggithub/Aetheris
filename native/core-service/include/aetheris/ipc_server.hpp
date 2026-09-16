#pragma once

#include "aetheris/ipc_protocol.hpp"

#include <atomic>
#include <functional>
#include <optional>
#include <string>
#include <thread>

namespace aetheris::service {

class IpcServer {
public:
    using ExpectedClient = std::function<std::optional<ClientIdentity>()>;
    using Handler = std::function<void(const IpcMessage&)>;

    IpcServer(std::wstring allowed_user_sid, ExpectedClient expected, Handler handler);
    ~IpcServer();
    bool start();
    void stop();

private:
    void serve();
    std::wstring allowed_user_sid_;
    ExpectedClient expected_;
    Handler handler_;
    std::atomic<bool> stopping_{false};
    std::thread thread_;
};

}
