cmake_minimum_required(VERSION 3.24)
project(AetherisProvisioning LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_library(provisioning_core STATIC
    src/crypto.cpp
    src/json.cpp
    src/file.cpp
    src/project_scan.cpp
    src/http.cpp
    src/provision.cpp
    src/core_service_install.cpp
)
target_include_directories(provisioning_core PUBLIC include)
target_link_libraries(provisioning_core PUBLIC advapi32 bcrypt crypt32 winhttp wintrust normaliz secur32 wtsapi32 ole32 oleaut32)
target_compile_options(provisioning_core PUBLIC /utf-8)

add_executable(provisioning_tests tests/provisioning_tests.cpp)
target_link_libraries(provisioning_tests PRIVATE provisioning_core)

add_library(AetherisProvisioning SHARED
    src/plugin.cpp
)
target_include_directories(AetherisProvisioning PRIVATE include)
target_compile_options(AetherisProvisioning PRIVATE /utf-8)
target_link_libraries(AetherisProvisioning PRIVATE provisioning_core winhttp comctl32)
set_target_properties(AetherisProvisioning PROPERTIES PREFIX "")

enable_testing()
add_test(NAME provisioning_tests COMMAND provisioning_tests)
