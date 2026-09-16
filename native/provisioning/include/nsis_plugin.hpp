#pragma once

#include <windows.h>

#include <string>

namespace nsis {
struct stack_t {
    stack_t* next;
    wchar_t text[1];
};

struct extra_parameters;

inline std::wstring pop(stack_t** stack) {
    if (!stack || !*stack) return {};
    stack_t* item = *stack;
    std::wstring value(item->text);
    *stack = item->next;
    GlobalFree(item);
    return value;
}

inline void push(stack_t** stack, int string_size, const std::wstring& value) {
    if (!stack || string_size < 2) return;
    const auto bytes = sizeof(stack_t) + static_cast<std::size_t>(string_size) * sizeof(wchar_t);
    auto* item = static_cast<stack_t*>(GlobalAlloc(GPTR, bytes));
    if (!item) return;
    lstrcpynW(item->text, value.c_str(), string_size);
    item->next = *stack;
    *stack = item;
}
}

#define NSIS_PLUGIN_FUNCTION(name) \
    extern "C" __declspec(dllexport) void name( \
        HWND, int string_size, wchar_t*, nsis::stack_t** stack, nsis::extra_parameters*, ...)
