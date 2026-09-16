#include "aetheris/file.hpp"

#include <windows.h>

#include <fstream>
#include <stdexcept>
#include <string>

namespace aetheris {
void atomic_write(const std::filesystem::path& path, const std::vector<std::uint8_t>& value) {
    if (path.has_parent_path()) std::filesystem::create_directories(path.parent_path());
    const auto temporary = std::filesystem::path(path.wstring() + L".tmp-" + std::to_wstring(GetCurrentProcessId()) + L"-" + std::to_wstring(GetTickCount64()));
    HANDLE handle = CreateFileW(temporary.c_str(), GENERIC_WRITE, 0, nullptr, CREATE_NEW, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (handle == INVALID_HANDLE_VALUE) throw std::runtime_error("CreateFileW failed");
    try {
        DWORD written = 0;
        if (!value.empty() && (!WriteFile(handle, value.data(), static_cast<DWORD>(value.size()), &written, nullptr) || written != value.size())) {
            throw std::runtime_error("WriteFile failed");
        }
        if (!FlushFileBuffers(handle)) throw std::runtime_error("FlushFileBuffers failed");
        CloseHandle(handle);
        handle = INVALID_HANDLE_VALUE;
        if (!MoveFileExW(temporary.c_str(), path.c_str(), MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH)) {
            throw std::runtime_error("MoveFileExW failed");
        }
    } catch (...) {
        if (handle != INVALID_HANDLE_VALUE) CloseHandle(handle);
        DeleteFileW(temporary.c_str());
        throw;
    }
}

std::vector<std::uint8_t> read_bytes(const std::filesystem::path& path) {
    std::ifstream input(path, std::ios::binary);
    if (!input) throw std::runtime_error("open failed");
    return std::vector<std::uint8_t>(std::istreambuf_iterator<char>(input), std::istreambuf_iterator<char>());
}
}
