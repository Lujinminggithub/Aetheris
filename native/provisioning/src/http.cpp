#include "aetheris/provision.hpp"

#include <windows.h>
#include <winhttp.h>

#include <stdexcept>
#include <vector>

namespace {
struct InternetHandle {
    HINTERNET value = nullptr;
    ~InternetHandle() { if (value) WinHttpCloseHandle(value); }
};

std::wstring widen_ascii(const std::string& value) {
    return std::wstring(value.begin(), value.end());
}

aetheris::HttpResponse post_with_headers(
    const std::wstring& gateway,
    const std::wstring& endpoint,
    const std::string& body,
    const std::wstring& headers) {
    URL_COMPONENTS parts{};
    parts.dwStructSize = sizeof(parts);
    parts.dwSchemeLength = static_cast<DWORD>(-1);
    parts.dwHostNameLength = static_cast<DWORD>(-1);
    parts.dwUrlPathLength = static_cast<DWORD>(-1);
    if (!WinHttpCrackUrl(gateway.c_str(), static_cast<DWORD>(gateway.size()), 0, &parts)) throw std::runtime_error("invalid_gateway_url");
    const std::wstring host(parts.lpszHostName, parts.dwHostNameLength);
    std::wstring base_path(parts.lpszUrlPath, parts.dwUrlPathLength);
    if (base_path == L"/") base_path.clear();
    const auto request_path = base_path + endpoint;
    InternetHandle session{WinHttpOpen(L"AetherisSetup/0.4.15", WINHTTP_ACCESS_TYPE_AUTOMATIC_PROXY, WINHTTP_NO_PROXY_NAME, WINHTTP_NO_PROXY_BYPASS, 0)};
    if (!session.value) throw std::runtime_error("network_open_failed");
    WinHttpSetTimeouts(session.value, 10000, 10000, 10000, 15000);
    InternetHandle connection{WinHttpConnect(session.value, host.c_str(), parts.nPort, 0)};
    if (!connection.value) throw std::runtime_error("network_connect_failed");
    const DWORD flags = parts.nScheme == INTERNET_SCHEME_HTTPS ? WINHTTP_FLAG_SECURE : 0;
    InternetHandle request{WinHttpOpenRequest(connection.value, L"POST", request_path.c_str(), nullptr, WINHTTP_NO_REFERER, WINHTTP_DEFAULT_ACCEPT_TYPES, flags)};
    if (!request.value) throw std::runtime_error("network_request_failed");
    if (!WinHttpSendRequest(request.value, headers.c_str(), static_cast<DWORD>(-1), const_cast<char*>(body.data()), static_cast<DWORD>(body.size()), static_cast<DWORD>(body.size()), 0) || !WinHttpReceiveResponse(request.value, nullptr)) {
        throw std::runtime_error("network_send_failed");
    }
    DWORD status = 0, status_size = sizeof(status);
    if (!WinHttpQueryHeaders(request.value, WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER, nullptr, &status, &status_size, nullptr)) throw std::runtime_error("network_status_failed");
    std::string response;
    while (true) {
        DWORD available = 0;
        if (!WinHttpQueryDataAvailable(request.value, &available)) throw std::runtime_error("network_read_failed");
        if (!available) break;
        if (response.size() + available > 1024 * 1024) throw std::runtime_error("response_too_large");
        const auto offset = response.size(); response.resize(offset + available);
        DWORD read = 0;
        if (!WinHttpReadData(request.value, response.data() + offset, available, &read)) throw std::runtime_error("network_read_failed");
        response.resize(offset + read);
    }
    return aetheris::HttpResponse{static_cast<int>(status), std::move(response)};
}
}

namespace aetheris {
HttpResponse winhttp_post(const std::wstring& gateway, const std::wstring& endpoint, const std::string& body, const std::string& bearer) {
    std::wstring headers = L"Content-Type: application/json\r\nAccept: application/json\r\n";
    if (!bearer.empty()) headers += L"Authorization: Bearer " + widen_ascii(bearer) + L"\r\n";
    return post_with_headers(gateway, endpoint, body, headers);
}

HttpResponse winhttp_local_control_post(const std::wstring& local_url, const std::string& control_token) {
    std::wstring origin = local_url;
    while (!origin.empty() && origin.back() == L'/') origin.pop_back();
    const auto headers = L"Content-Type: application/json\r\nOrigin: " + origin +
        L"\r\nX-Aetheris-Control: local-lens\r\nCookie: aetheris_control=" + widen_ascii(control_token) + L"\r\n";
    return post_with_headers(local_url, L"/api/actions/exit", "{}", headers);
}
}
