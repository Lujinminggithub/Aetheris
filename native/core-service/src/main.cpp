#include "aetheris/service_host.hpp"

#include <windows.h>

int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int) {
    SERVICE_TABLE_ENTRYW table[] = {
        {const_cast<wchar_t*>(aetheris::service::service_name), aetheris::service::service_main},
        {nullptr, nullptr},
    };
    return StartServiceCtrlDispatcherW(table) ? 0 : static_cast<int>(GetLastError());
}
