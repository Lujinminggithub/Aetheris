#pragma once

#include <filesystem>
#include <functional>
#include <string>
#include <vector>

#include "aetheris/project_scan.hpp"

namespace aetheris {
struct HttpResponse {
    int status = 0;
    std::string body;
};

using HttpTransport = std::function<HttpResponse(
    const std::wstring& gateway,
    const std::wstring& endpoint,
    const std::string& body,
    const std::string& bearer)>;
using LocalControlTransport = std::function<HttpResponse(const std::wstring& local_url, const std::string& control_token)>;

struct ProvisionRequest {
    std::wstring gateway_url;
    std::wstring enrollment_code;
    std::wstring user_identifier;
    std::wstring client_version;
    std::wstring hostname;
    std::wstring machine_guid;
    std::filesystem::path install_root;
    std::vector<ProjectSource> projects;
};

struct ProvisionResult {
    bool success = false;
    std::wstring error_code;
    std::wstring tenant_id;
    std::wstring subject_id;
    std::wstring device_id;
};

HttpResponse winhttp_post(
    const std::wstring& gateway,
    const std::wstring& endpoint,
    const std::string& body,
    const std::string& bearer);
HttpResponse winhttp_local_control_post(const std::wstring& local_url, const std::string& control_token);

ProvisionResult provision_device(const ProvisionRequest& request, const HttpTransport& transport = winhttp_post);
bool verify_core_status(const std::filesystem::path& status_path, const std::wstring& version, const std::wstring& device_id);

struct RevokeResult {
    bool local_exit_requested = false;
    bool credential_revoked = false;
    std::wstring error_code;
};

std::string read_dpapi_credential(const std::filesystem::path& path);
RevokeResult revoke_device(
    const std::filesystem::path& install_root,
    const HttpTransport& server_transport = winhttp_post,
    const LocalControlTransport& local_transport = {});
bool request_existing_core_exit(
    const std::filesystem::path& install_root,
    const LocalControlTransport& local_transport = {});

enum class ExistingDeviceState { reusable, missing, credential_invalid, server_unavailable };

struct ExistingDeviceResult {
    ExistingDeviceState state = ExistingDeviceState::missing;
    std::wstring device_id;
};

ExistingDeviceResult verify_existing_device(
    const std::filesystem::path& install_root,
    const std::wstring& gateway,
    const HttpTransport& transport = winhttp_post);
bool update_existing_core_version(const std::filesystem::path& install_root, const std::wstring& version);
}
