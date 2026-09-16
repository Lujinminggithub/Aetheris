#include "aetheris/provision.hpp"

#include "aetheris/crypto.hpp"
#include "aetheris/file.hpp"
#include "aetheris/json.hpp"

#include <windows.h>
#include <bcrypt.h>
#include <algorithm>
#include <cctype>
#include <cwctype>
#include <optional>
#include <stdexcept>
#include <vector>

namespace {
constexpr char credential_magic[] = "AETHERIS-DPAPI-1";

std::wstring trim(std::wstring value) {
    while (!value.empty() && std::iswspace(value.front())) value.erase(value.begin());
    while (!value.empty() && std::iswspace(value.back())) value.pop_back();
    return value;
}

std::wstring normalize_identifier(const std::wstring& input) {
    const auto stripped = trim(input);
    if (stripped.empty()) throw std::runtime_error("user_identifier_required");
    int normalized_size = NormalizeString(NormalizationKC, stripped.c_str(), static_cast<int>(stripped.size()), nullptr, 0);
    if (normalized_size <= 0) throw std::runtime_error("user_identifier_invalid");
    std::wstring normalized(static_cast<std::size_t>(normalized_size), L'\0');
    const int written = NormalizeString(NormalizationKC, stripped.c_str(), static_cast<int>(stripped.size()), normalized.data(), normalized_size);
    if (written <= 0) throw std::runtime_error("user_identifier_invalid");
    normalized.resize(static_cast<std::size_t>(written));
    CharLowerBuffW(normalized.data(), static_cast<DWORD>(normalized.size()));
    return normalized;
}

std::wstring wide_utf8(const std::string& value) {
    if (value.empty()) return {};
    const int size = MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), nullptr, 0);
    if (size <= 0) throw std::runtime_error("invalid_utf8");
    std::wstring result(static_cast<std::size_t>(size), L'\0');
    MultiByteToWideChar(CP_UTF8, MB_ERR_INVALID_CHARS, value.data(), static_cast<int>(value.size()), result.data(), size);
    return result;
}

std::vector<std::uint8_t> protected_file(const std::string& plaintext) {
    std::vector<std::uint8_t> sensitive(plaintext.begin(), plaintext.end());
    auto encrypted = aetheris::dpapi_protect(sensitive);
    if (!sensitive.empty()) SecureZeroMemory(sensitive.data(), sensitive.size());
    std::vector<std::uint8_t> result(std::begin(credential_magic), std::end(credential_magic) - 1);
    result.push_back(0);
    result.insert(result.end(), encrypted.begin(), encrypted.end());
    return result;
}

std::string random_control_token() {
    std::vector<std::uint8_t> bytes(32);
    if (BCryptGenRandom(nullptr, bytes.data(), static_cast<ULONG>(bytes.size()), BCRYPT_USE_SYSTEM_PREFERRED_RNG) < 0) {
        throw std::runtime_error("random_failed");
    }
    const auto token = aetheris::sha256_hex(bytes);
    SecureZeroMemory(bytes.data(), bytes.size());
    return token;
}

std::string project_roots_json(const std::vector<aetheris::ProjectSource>& projects) {
    std::string value = "[";
    for (std::size_t index = 0; index < projects.size(); ++index) {
        if (index) value += ',';
        value += "{\"path\":\"" + aetheris::json_escape(projects[index].path.wstring()) +
            "\",\"vcs\":\"" + aetheris::json_escape(projects[index].vcs) + "\"}";
    }
    return value + "]";
}

std::string authorized_roots_json(const std::vector<aetheris::ProjectSource>& projects) {
    std::string value = "[";
    for (std::size_t index = 0; index < projects.size(); ++index) {
        if (index) value += ',';
        value += "\"" + aetheris::json_escape(projects[index].path.wstring()) + "\"";
    }
    return value + "]";
}

std::optional<std::pair<std::size_t, std::size_t>> top_level_string_value(
    const std::string& json,
    const std::string& expected_key) {
    int depth = 0;
    std::optional<std::pair<std::size_t, std::size_t>> found;
    for (std::size_t index = 0; index < json.size();) {
        if (json[index] == '{') { ++depth; ++index; continue; }
        if (json[index] == '}') { --depth; if (depth < 0) return std::nullopt; ++index; continue; }
        if (json[index] != '"') { ++index; continue; }
        const auto token_start = ++index;
        bool escaped = false;
        while (index < json.size()) {
            const char value = json[index];
            if (escaped) escaped = false;
            else if (value == '\\') escaped = true;
            else if (value == '"') break;
            ++index;
        }
        if (index >= json.size()) return std::nullopt;
        const auto token_end = index++;
        if (depth != 1 || json.compare(token_start, token_end - token_start, expected_key) != 0) continue;
        auto cursor = index;
        while (cursor < json.size() && std::isspace(static_cast<unsigned char>(json[cursor]))) ++cursor;
        if (cursor >= json.size() || json[cursor++] != ':') continue;
        while (cursor < json.size() && std::isspace(static_cast<unsigned char>(json[cursor]))) ++cursor;
        if (cursor >= json.size() || json[cursor++] != '"') return std::nullopt;
        const auto value_start = cursor;
        escaped = false;
        while (cursor < json.size()) {
            const char value = json[cursor];
            if (escaped) escaped = false;
            else if (value == '\\') escaped = true;
            else if (value == '"') break;
            ++cursor;
        }
        if (cursor >= json.size() || found) return std::nullopt;
        found = std::make_pair(value_start, cursor);
        index = cursor + 1;
    }
    return depth == 0 ? found : std::nullopt;
}
}

namespace aetheris {
ProvisionResult provision_device(const ProvisionRequest& request, const HttpTransport& transport) {
    ProvisionResult result;
    try {
        const auto user = normalize_identifier(request.user_identifier);
        const auto machine = normalize_identifier(request.machine_guid);
        const auto user_bytes = utf8(user);
        const auto machine_bytes = utf8(machine);
        const auto subject = "subject-" + sha256_hex(std::vector<std::uint8_t>(user_bytes.begin(), user_bytes.end())).substr(0, 16);
        std::vector<std::uint8_t> device_material(machine_bytes.begin(), machine_bytes.end());
        device_material.push_back(0);
        device_material.insert(device_material.end(), user_bytes.begin(), user_bytes.end());
        const auto device = "device-" + sha256_hex(device_material).substr(0, 16);
        result.subject_id = wide_utf8(subject);
        result.device_id = wide_utf8(device);

        const auto bootstrap_body = std::string("{\"enrollment_secret\":\"") + json_escape(request.enrollment_code) +
            "\",\"device_id\":\"" + device + "\",\"subject_id\":\"" + subject +
            "\",\"subject_name\":\"" + json_escape(request.user_identifier) + "\",\"client_version\":\"" +
            json_escape(request.client_version) + "\",\"hostname\":\"" + json_escape(request.hostname) + "\"}";
        const auto bootstrap = transport(request.gateway_url, L"/api/v1/device/bootstrap", bootstrap_body, "");
        if (bootstrap.status == 401 || bootstrap.status == 403) {
            result.error_code = L"invalid_enrollment";
            return result;
        }
        if (bootstrap.status < 200 || bootstrap.status >= 300) {
            result.error_code = bootstrap.status >= 500 ? L"server_unavailable" : L"bootstrap_failed";
            return result;
        }
        auto token = json_get_string(bootstrap.body, "device_token");
        const auto tenant = json_get_string(bootstrap.body, "tenant_id");
        const auto response_subject = json_get_string(bootstrap.body, "subject_id");
        const auto response_device = json_get_string(bootstrap.body, "device_id");
        if (token.empty() || tenant.empty() || response_subject != subject || response_device != device) {
            if (!token.empty()) SecureZeroMemory(token.data(), token.size());
            if (token.empty()) result.error_code = L"bootstrap_token_missing";
            else if (tenant.empty()) result.error_code = L"bootstrap_tenant_missing";
            else if (response_subject != subject) result.error_code = L"bootstrap_subject_mismatch";
            else result.error_code = L"bootstrap_device_mismatch";
            return result;
        }
        const auto heartbeat = transport(request.gateway_url, L"/api/v1/device/heartbeat", "{}", token);
        if (heartbeat.status != 200 || json_get_string(heartbeat.body, "status") != "online" ||
            json_get_string(heartbeat.body, "subject_id") != subject || json_get_string(heartbeat.body, "device_id") != device) {
            SecureZeroMemory(token.data(), token.size());
            result.error_code = L"heartbeat_identity_mismatch";
            return result;
        }

        const auto config_dir = request.install_root / L"config";
        atomic_write(config_dir / L"device.credential", protected_file(token));
        auto control_token = random_control_token();
        atomic_write(config_dir / L"control.credential", protected_file(control_token));
        SecureZeroMemory(control_token.data(), control_token.size());
        SecureZeroMemory(token.data(), token.size());

        const auto project_fields = request.projects.empty() ? std::string{} :
            "\"project_root\":\"" + json_escape(request.projects.front().path.wstring()) + "\",";
        const auto config_text = std::string("{\"config_version\":3,\"core_version\":\"") + json_escape(request.client_version) +
            "\",\"gateway_url\":\"" + json_escape(request.gateway_url) + "\",\"tenant_id\":\"" + tenant +
            "\",\"subject_id\":\"" + subject + "\",\"device_id\":\"" + device +
            "\",\"credential_kind\":\"device_token\",\"credential_file\":\"" + json_escape((config_dir / L"device.credential").wstring()) +
            "\",\"control_credential_file\":\"" + json_escape((config_dir / L"control.credential").wstring()) +
            "\",\"project_revision\":0,\"browser_policy_revision\":0,\"browser_allowlist\":[]," + project_fields + "\"project_roots\":" + project_roots_json(request.projects) +
            ",\"authorized_roots\":" + authorized_roots_json(request.projects) + ",\"queue\":\"" +
            json_escape((request.install_root / L"data" / L"client.db").wstring()) + "\",\"interval_seconds\":15,\"heartbeat_seconds\":60,\"show_status_on_first_run\":true}";
        atomic_write(config_dir / L"aetheris.json", std::vector<std::uint8_t>(config_text.begin(), config_text.end()));
        result.success = true;
        result.tenant_id = wide_utf8(tenant);
        return result;
    } catch (...) {
        result.error_code = L"provisioning_failed";
        return result;
    }
}

bool verify_core_status(const std::filesystem::path& status_path, const std::wstring& version, const std::wstring& device_id) {
    try {
        const auto raw = read_bytes(status_path);
        const std::string body(raw.begin(), raw.end());
        return json_get_string(body, "version") == utf8(version) &&
            json_get_string(body, "device_id") == utf8(device_id) &&
            json_get_string(body, "state") == "registered";
    } catch (...) {
        return false;
    }
}

std::string read_dpapi_credential(const std::filesystem::path& path) {
    auto raw = read_bytes(path);
    const std::vector<std::uint8_t> magic(std::begin(credential_magic), std::end(credential_magic) - 1);
    if (raw.size() <= magic.size() || !std::equal(magic.begin(), magic.end(), raw.begin()) || raw[magic.size()] != 0) {
        throw std::runtime_error("credential_format_invalid");
    }
    std::vector<std::uint8_t> encrypted(raw.begin() + static_cast<std::ptrdiff_t>(magic.size() + 1), raw.end());
    auto plaintext = dpapi_unprotect(encrypted);
    std::string value(plaintext.begin(), plaintext.end());
    if (!plaintext.empty()) SecureZeroMemory(plaintext.data(), plaintext.size());
    return value;
}

RevokeResult revoke_device(const std::filesystem::path& install_root, const HttpTransport& server_transport, const LocalControlTransport& local_transport) {
    RevokeResult result;
    std::string device_token;
    std::string control_token;
    try {
        const auto config_raw = read_bytes(install_root / L"config" / L"aetheris.json");
        const std::string config(config_raw.begin(), config_raw.end());
        const auto status_raw = read_bytes(install_root / L"data" / L"core-status.json");
        const std::string status(status_raw.begin(), status_raw.end());
        const auto gateway = wide_utf8(json_get_string(config, "gateway_url"));
        const auto local_url = wide_utf8(json_get_string(status, "local_view_url"));
        device_token = read_dpapi_credential(install_root / L"config" / L"device.credential");
        control_token = read_dpapi_credential(install_root / L"config" / L"control.credential");
        const auto local_response = local_transport ? local_transport(local_url, control_token) : winhttp_local_control_post(local_url, control_token);
        result.local_exit_requested = local_response.status == 202 || local_response.status == 204;
        const auto server_response = server_transport(gateway, L"/api/v1/device/revoke", "{}", device_token);
        result.credential_revoked = server_response.status == 200 || server_response.status == 204 || server_response.status == 401;
        if (!result.local_exit_requested) result.error_code = L"local_exit_failed";
        else if (!result.credential_revoked) result.error_code = L"revocation_pending";
    } catch (...) {
        result.error_code = L"revocation_pending";
    }
    if (!device_token.empty()) SecureZeroMemory(device_token.data(), device_token.size());
    if (!control_token.empty()) SecureZeroMemory(control_token.data(), control_token.size());
    return result;
}

bool request_existing_core_exit(const std::filesystem::path& install_root, const LocalControlTransport& local_transport) {
    try {
        const auto status_raw = read_bytes(install_root / L"data" / L"core-status.json");
        const std::string status(status_raw.begin(), status_raw.end());
        const auto local_url = wide_utf8(json_get_string(status, "local_view_url"));
        auto control_token = read_dpapi_credential(install_root / L"config" / L"control.credential");
        const auto response = local_transport ? local_transport(local_url, control_token) : winhttp_local_control_post(local_url, control_token);
        SecureZeroMemory(control_token.data(), control_token.size());
        return response.status == 202 || response.status == 204;
    } catch (...) {
        return true;
    }
}

ExistingDeviceResult verify_existing_device(
    const std::filesystem::path& install_root,
    const std::wstring& gateway,
    const HttpTransport& transport) {
    ExistingDeviceResult result;
    try {
        const auto config_raw = read_bytes(install_root / L"config" / L"aetheris.json");
        const std::string config(config_raw.begin(), config_raw.end());
        const auto subject = json_get_string(config, "subject_id");
        const auto device = json_get_string(config, "device_id");
        const auto credential_path = json_get_string(config, "credential_file");
        if (subject.empty() || device.empty() || credential_path.empty()) return result;
        auto token = read_dpapi_credential(std::filesystem::path(wide_utf8(credential_path)));
        const auto heartbeat = transport(gateway, L"/api/v1/device/heartbeat", "{}", token);
        SecureZeroMemory(token.data(), token.size());
        if (heartbeat.status == 401 || heartbeat.status == 403) {
            result.state = ExistingDeviceState::credential_invalid;
            return result;
        }
        if (heartbeat.status != 200) {
            result.state = ExistingDeviceState::server_unavailable;
            return result;
        }
        if (json_get_string(heartbeat.body, "status") != "online" ||
            json_get_string(heartbeat.body, "subject_id") != subject ||
            json_get_string(heartbeat.body, "device_id") != device) {
            result.state = ExistingDeviceState::credential_invalid;
            return result;
        }
        result.state = ExistingDeviceState::reusable;
        result.device_id = wide_utf8(device);
        return result;
    } catch (const std::runtime_error& error) {
        const std::string message = error.what();
        result.state = message.find("credential") != std::string::npos
            ? ExistingDeviceState::credential_invalid : ExistingDeviceState::missing;
        return result;
    }
}

bool update_existing_core_version(const std::filesystem::path& install_root, const std::wstring& version) {
    try {
        if (version.empty()) return false;
        for (const wchar_t value : version) {
            const bool allowed = (value >= L'a' && value <= L'z') || (value >= L'A' && value <= L'Z') ||
                (value >= L'0' && value <= L'9') || value == L'.' || value == L'-' || value == L'+';
            if (!allowed) return false;
        }
        const auto path = install_root / L"config" / L"aetheris.json";
        const auto raw = read_bytes(path);
        std::string config(raw.begin(), raw.end());
        const auto value = top_level_string_value(config, "core_version");
        if (!value) return false;
        config.replace(value->first, value->second - value->first, json_escape(version));
        atomic_write(path, std::vector<std::uint8_t>(config.begin(), config.end()));
        const auto verified = read_bytes(path);
        return json_get_string(std::string(verified.begin(), verified.end()), "core_version") == utf8(version);
    } catch (...) {
        return false;
    }
}
}
