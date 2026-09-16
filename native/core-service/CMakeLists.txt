cmake_minimum_required(VERSION 3.24)
project(AetherisCoreService LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")

add_library(core_service_core STATIC
    src/service_config.cpp
    src/recovery_policy.cpp
    src/state_store.cpp
    src/session_launcher.cpp
    src/ipc_protocol.cpp
    src/ipc_server.cpp
    src/service_log.cpp
    src/service_host.cpp
)
target_include_directories(core_service_core PUBLIC include)
target_compile_options(core_service_core PUBLIC /utf-8)
target_link_libraries(core_service_core PUBLIC advapi32 bcrypt userenv wtsapi32)

add_executable(core_service_tests tests/service_tests.cpp)
target_link_libraries(core_service_tests PRIVATE core_service_core)

add_executable(AetherisCoreService WIN32 src/main.cpp)
target_link_libraries(AetherisCoreService PRIVATE core_service_core)
target_compile_options(AetherisCoreService PRIVATE /utf-8)

enable_testing()
add_test(NAME core_service_tests COMMAND core_service_tests)
