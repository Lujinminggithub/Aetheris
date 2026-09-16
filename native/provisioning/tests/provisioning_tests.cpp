#include "aetheris/crypto.hpp"
#include "aetheris/file.hpp"
#include "aetheris/json.hpp"
#include "aetheris/project_scan.hpp"
#include "aetheris/provision.hpp"
#include "aetheris/core_service_install.hpp"

#include <windows.h>
#include <sddl.h>

#include <algorithm>
#include <filesystem>
#include <chrono>
#include <iostream>
#include <stdexcept>
#include <string>
#include <vector>
#include <thread>

namespace {
void require(bool condition, const char* message) {
    if (!condition) throw std::runtime_error(message);
}
}

int wmain() {
    try {
        const std::vector<std::uint8_t> abc{'a', 'b', 'c'};
        require(aetheris::sha256_hex(abc) == "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", "sha256 mismatch");
        require(aetheris::json_escape(L"A\"B\n\\C") == "A\\\"B\\n\\\\C", "json escaping mismatch");
        require(aetheris::utf8(L"项目") == "\xE9\xA1\xB9\xE7\x9B\xAE", "unicode conversion mismatch");

        const std::vector<std::uint8_t> secret{'s', 'e', 'c', 'r', 'e', 't'};
        const auto protected_value = aetheris::dpapi_protect(secret);
        require(protected_value != secret, "DPAPI returned plaintext");
        require(aetheris::dpapi_unprotect(protected_value) == secret, "DPAPI round-trip failed");

        const auto path = std::filesystem::temp_directory_path() / L"aetheris-provisioning-atomic-test.bin";
        aetheris::atomic_write(path, abc);
        require(aetheris::read_bytes(path) == abc, "atomic write mismatch");
        std::filesystem::remove(path);

        const auto scan_root = std::filesystem::temp_directory_path() / L"aetheris-project-scan-test";
        std::filesystem::remove_all(scan_root);
        std::filesystem::create_directories(scan_root / L"repo-a" / L".git");
        std::filesystem::create_directories(scan_root / L"group" / L"repo-b" / L".svn");
        std::filesystem::create_directories(scan_root / L"node_modules" / L"ignored" / L".git");
        const auto scan = aetheris::scan_projects(scan_root, 8, 100);
        require(scan.state == aetheris::ScanState::complete, "project scan did not complete");
        require(scan.projects.size() == 2, "project scan count mismatch");
        const auto git = std::find_if(scan.projects.begin(), scan.projects.end(), [](const auto& project) { return project.vcs == L"git"; });
        const auto svn = std::find_if(scan.projects.begin(), scan.projects.end(), [](const auto& project) { return project.vcs == L"svn"; });
        require(git != scan.projects.end() && git->path.filename() == L"repo-a", "git project missing");
        require(svn != scan.projects.end() && svn->path.filename() == L"repo-b", "svn project missing");

        for (int index = 0; index < 105; ++index) {
            std::filesystem::create_directories(scan_root / (L"many-" + std::to_wstring(index)) / L".git");
        }
        const auto limited = aetheris::scan_projects(scan_root, 8, 100);
        require(limited.projects.size() == 100, "project limit mismatch");
        require(limited.truncated, "project truncation not reported");

        aetheris::ProjectScanner scanner;
        scanner.start(scan_root, 8, 100);
        auto async = scanner.snapshot();
        for (int attempt = 0; attempt < 200 && async.state == aetheris::ScanState::running; ++attempt) {
            std::this_thread::sleep_for(std::chrono::milliseconds(10));
            async = scanner.snapshot();
        }
        require(async.state == aetheris::ScanState::complete, "async scan did not complete");
        require(async.projects.size() == 100, "async scan result mismatch");
        scanner.start(scan_root, 8, 100);
        const auto second_start = std::chrono::steady_clock::now();
        const auto restarted = scanner.snapshot();
        require(std::chrono::steady_clock::now() - second_start < std::chrono::milliseconds(100), "scan restart blocked caller");
        require(restarted.state == aetheris::ScanState::running, "scan restart did not enter running state");
        std::filesystem::remove_all(scan_root);

        const auto provision_root = std::filesystem::temp_directory_path() / L"aetheris-provision-test";
        std::filesystem::remove_all(provision_root);
        aetheris::ProvisionRequest request{
            L"http://127.0.0.1:18080", L"enroll-once", L"DOMAIN\\Alice", L"0.4.6",
            L"workstation", L"machine-guid", provision_root,
        };
        std::string issued_token = "issued-device-token";
        const auto expected_subject = "subject-" + aetheris::sha256_hex(std::vector<std::uint8_t>{'d','o','m','a','i','n','\\','a','l','i','c','e'}).substr(0, 16);
        std::string expected_device_material = "machine-guid";
        expected_device_material.push_back('\0');
        expected_device_material += "domain\\alice";
        const auto expected_device = "device-" + aetheris::sha256_hex(std::vector<std::uint8_t>(expected_device_material.begin(), expected_device_material.end())).substr(0, 16);
        auto success_transport = [&](const std::wstring&, const std::wstring& endpoint, const std::string& body, const std::string& bearer) {
            if (endpoint == L"/api/v1/device/bootstrap") {
                require(body.find("enroll-once") != std::string::npos, "bootstrap enrollment missing");
                require(bearer.empty(), "bootstrap unexpectedly authenticated");
                return aetheris::HttpResponse{200, "{\"device_token\":\"" + issued_token + "\",\"tenant_id\":\"tenant-local\",\"subject_id\":\"" + expected_subject + "\",\"device_id\":\"" + expected_device + "\"}"};
            }
            require(endpoint == L"/api/v1/device/heartbeat", "unexpected endpoint");
            require(bearer == issued_token, "heartbeat token mismatch");
            return aetheris::HttpResponse{200, "{\"status\":\"online\",\"subject_id\":\"" + expected_subject + "\",\"device_id\":\"" + expected_device + "\"}"};
        };
        const auto provisioned = aetheris::provision_device(request, success_transport);
        if (!provisioned.success) std::wcerr << L"provision error: " << provisioned.error_code << L" subject=" << provisioned.subject_id << L" device=" << provisioned.device_id << L"\n";
        require(provisioned.success, "provisioning failed");
        require(provisioned.tenant_id == L"tenant-local", "tenant missing");
        const auto credential = aetheris::read_bytes(provision_root / L"config" / L"device.credential");
        require(std::search(credential.begin(), credential.end(), issued_token.begin(), issued_token.end()) == credential.end(), "credential contains plaintext token");
        const auto config = aetheris::read_bytes(provision_root / L"config" / L"aetheris.json");
        const std::string config_text(config.begin(), config.end());
        require(config_text.find("\"browser_policy_revision\":0") != std::string::npos, "browser policy revision missing from config");
        require(config_text.find("\"browser_allowlist\":[]") != std::string::npos, "browser allowlist missing from config");
        require(config_text.find("\"project_roots\":[]") != std::string::npos, "zero-project config missing");
        require(config_text.find(issued_token) == std::string::npos, "config contains token");

        request.install_root = provision_root / L"with-project";
        request.projects.push_back(aetheris::ProjectSource{scan_root / L"repo-a", L"git"});
        const auto with_project = aetheris::provision_device(request, success_transport);
        require(with_project.success, "project provisioning failed");
        const auto project_config = aetheris::read_bytes(request.install_root / L"config" / L"aetheris.json");
        const std::string project_config_text(project_config.begin(), project_config.end());
        require(project_config_text.find("\"vcs\":\"git\"") != std::string::npos, "project vcs missing from config");
        require(project_config_text.find("repo-a") != std::string::npos, "project path missing from config");
        const auto reusable = aetheris::verify_existing_device(request.install_root, L"http://127.0.0.1:18080", success_transport);
        require(reusable.state == aetheris::ExistingDeviceState::reusable && reusable.device_id == std::wstring(expected_device.begin(), expected_device.end()), "existing device not reusable");
        const auto invalid_existing = aetheris::verify_existing_device(request.install_root, L"http://127.0.0.1:18080", [](const auto&, const auto&, const auto&, const auto&) {
            return aetheris::HttpResponse{401, "{}"};
        });
        require(invalid_existing.state == aetheris::ExistingDeviceState::credential_invalid, "invalid existing credential accepted");
        require(aetheris::update_existing_core_version(request.install_root, L"0.4.8"), "existing version update failed");
        const auto updated_config = aetheris::read_bytes(request.install_root / L"config" / L"aetheris.json");
        require(aetheris::json_get_string(std::string(updated_config.begin(), updated_config.end()), "core_version") == "0.4.8", "existing version not persisted");
        const auto formatted_root = provision_root / L"formatted-config";
        const auto formatted_path = formatted_root / L"config" / L"aetheris.json";
        const std::string formatted_json = "{\n  \"config_version\": 3,\n  \"core_version\": \"0.4.7\",\n  \"preserve\": true\n}\n";
        aetheris::atomic_write(formatted_path, std::vector<std::uint8_t>(formatted_json.begin(), formatted_json.end()));
        require(aetheris::update_existing_core_version(formatted_root, L"0.4.8"), "formatted version update failed");
        const auto formatted_updated = aetheris::read_bytes(formatted_path);
        const std::string formatted_text(formatted_updated.begin(), formatted_updated.end());
        require(aetheris::json_get_string(formatted_text, "core_version") == "0.4.8", "formatted version not persisted");
        require(formatted_text.find("\"preserve\": true") != std::string::npos, "formatted config fields changed");
        const std::string duplicate_json = "{\"core_version\":\"0.4.7\",\"core_version\":\"0.4.6\"}";
        aetheris::atomic_write(formatted_path, std::vector<std::uint8_t>(duplicate_json.begin(), duplicate_json.end()));
        require(!aetheris::update_existing_core_version(formatted_root, L"0.4.8"), "duplicate version keys accepted");

        const auto status_path = request.install_root / L"data" / L"core-status.json";
        const auto status_text = std::string("{\"version\":\"0.4.6\",\"device_id\":\"") + expected_device + "\",\"registration\":{\"state\":\"registered\"}}";
        aetheris::atomic_write(status_path, std::vector<std::uint8_t>(status_text.begin(), status_text.end()));
        require(aetheris::verify_core_status(status_path, L"0.4.6", std::wstring(expected_device.begin(), expected_device.end())), "valid core status rejected");
        require(!aetheris::verify_core_status(status_path, L"0.4.7", std::wstring(expected_device.begin(), expected_device.end())), "wrong version accepted");
        require(aetheris::read_dpapi_credential(request.install_root / L"config" / L"device.credential") == issued_token, "device credential decrypt mismatch");
        aetheris::atomic_write(status_path, std::vector<std::uint8_t>{'{','"','l','o','c','a','l','_','v','i','e','w','_','u','r','l','"',':','"','h','t','t','p',':','/','/','1','2','7','.','0','.','0','.','1',':','9','9','9','9','/','"','}'});
        bool local_called = false;
        bool server_called = false;
        const auto revoked = aetheris::revoke_device(
            request.install_root,
            [&](const auto&, const auto& endpoint, const auto&, const auto& bearer) {
                server_called = endpoint == L"/api/v1/device/revoke" && bearer == issued_token;
                return aetheris::HttpResponse{204, ""};
            },
            [&](const auto& local_url, const auto& control_token) {
                local_called = local_url == L"http://127.0.0.1:9999/" && !control_token.empty();
                return aetheris::HttpResponse{202, "{}"};
            });
        require(revoked.local_exit_requested && local_called, "local exit was not requested");
        require(revoked.credential_revoked && server_called, "device credential was not revoked");

        const auto service_spec = aetheris::core_service_spec(provision_root / L"AetherisCoreService.exe");
        require(service_spec.account == L"LocalSystem", "service account mismatch");
        require(service_spec.start_type == SERVICE_AUTO_START && service_spec.delayed_auto_start, "service startup mismatch");
        require(service_spec.recovery_delay_ms == 60000, "service recovery mismatch");
        PSECURITY_DESCRIPTOR service_descriptor = nullptr;
        require(ConvertStringSecurityDescriptorToSecurityDescriptorW(
            aetheris::core_service_binary_sddl(), SDDL_REVISION_1, &service_descriptor, nullptr) != FALSE,
            "service binary SDDL invalid");
        LocalFree(service_descriptor);

        const auto failed_root = provision_root / L"failed";
        request.install_root = failed_root;
        const auto rejected = aetheris::provision_device(request, [](const auto&, const auto&, const auto&, const auto&) {
            return aetheris::HttpResponse{401, "{\"error\":\"bootstrap_failed\"}"};
        });
        require(!rejected.success && rejected.error_code == L"invalid_enrollment", "401 mapping mismatch");
        require(!std::filesystem::exists(failed_root / L"config" / L"device.credential"), "failed provisioning left credential");
        std::filesystem::remove_all(provision_root);
        std::wcout << L"provisioning tests passed\n";
        return 0;
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n';
        return 1;
    }
}
