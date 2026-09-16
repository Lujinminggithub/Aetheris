#include "aetheris/crypto.hpp"

#include <windows.h>
#include <bcrypt.h>
#include <dpapi.h>

#include <iomanip>
#include <sstream>
#include <stdexcept>

namespace {
void check_nt(NTSTATUS status, const char* operation) {
    if (status < 0) throw std::runtime_error(operation);
}
}

namespace aetheris {
std::string sha256_hex(const std::vector<std::uint8_t>& value) {
    BCRYPT_ALG_HANDLE algorithm = nullptr;
    BCRYPT_HASH_HANDLE hash = nullptr;
    DWORD object_size = 0;
    DWORD hash_size = 0;
    DWORD copied = 0;
    check_nt(BCryptOpenAlgorithmProvider(&algorithm, BCRYPT_SHA256_ALGORITHM, nullptr, 0), "BCryptOpenAlgorithmProvider failed");
    try {
        check_nt(BCryptGetProperty(algorithm, BCRYPT_OBJECT_LENGTH, reinterpret_cast<PUCHAR>(&object_size), sizeof(object_size), &copied, 0), "BCrypt object size failed");
        check_nt(BCryptGetProperty(algorithm, BCRYPT_HASH_LENGTH, reinterpret_cast<PUCHAR>(&hash_size), sizeof(hash_size), &copied, 0), "BCrypt hash size failed");
        std::vector<std::uint8_t> object(object_size);
        std::vector<std::uint8_t> digest(hash_size);
        check_nt(BCryptCreateHash(algorithm, &hash, object.data(), object_size, nullptr, 0, 0), "BCryptCreateHash failed");
        check_nt(BCryptHashData(hash, const_cast<PUCHAR>(value.data()), static_cast<ULONG>(value.size()), 0), "BCryptHashData failed");
        check_nt(BCryptFinishHash(hash, digest.data(), hash_size, 0), "BCryptFinishHash failed");
        BCryptDestroyHash(hash);
        hash = nullptr;
        std::ostringstream output;
        output << std::hex << std::setfill('0');
        for (auto byte : digest) output << std::setw(2) << static_cast<unsigned>(byte);
        const auto result = output.str();
        BCryptCloseAlgorithmProvider(algorithm, 0);
        return result;
    } catch (...) {
        if (hash) BCryptDestroyHash(hash);
        BCryptCloseAlgorithmProvider(algorithm, 0);
        throw;
    }
}

std::vector<std::uint8_t> dpapi_protect(const std::vector<std::uint8_t>& value) {
    DATA_BLOB input{static_cast<DWORD>(value.size()), const_cast<BYTE*>(value.data())};
    DATA_BLOB output{};
    if (!CryptProtectData(&input, L"Aetheris device credential", nullptr, nullptr, nullptr, CRYPTPROTECT_UI_FORBIDDEN, &output)) {
        throw std::runtime_error("CryptProtectData failed");
    }
    std::vector<std::uint8_t> result(output.pbData, output.pbData + output.cbData);
    SecureZeroMemory(output.pbData, output.cbData);
    LocalFree(output.pbData);
    return result;
}

std::vector<std::uint8_t> dpapi_unprotect(const std::vector<std::uint8_t>& value) {
    DATA_BLOB input{static_cast<DWORD>(value.size()), const_cast<BYTE*>(value.data())};
    DATA_BLOB output{};
    if (!CryptUnprotectData(&input, nullptr, nullptr, nullptr, nullptr, CRYPTPROTECT_UI_FORBIDDEN, &output)) {
        throw std::runtime_error("CryptUnprotectData failed");
    }
    std::vector<std::uint8_t> result(output.pbData, output.pbData + output.cbData);
    SecureZeroMemory(output.pbData, output.cbData);
    LocalFree(output.pbData);
    return result;
}
}
