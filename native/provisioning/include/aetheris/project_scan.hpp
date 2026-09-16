#pragma once

#include <cstddef>
#include <filesystem>
#include <string>
#include <vector>

namespace aetheris {
struct ProjectSource {
    std::filesystem::path path;
    std::wstring vcs;
};

enum class ScanState { idle, running, complete, warning };

struct ScanSnapshot {
    ScanState state = ScanState::idle;
    std::size_t visited = 0;
    std::vector<ProjectSource> projects;
    bool truncated = false;
    std::wstring error_code;
};

ScanSnapshot scan_projects(const std::filesystem::path& root, std::size_t max_depth = 8, std::size_t limit = 100);

class ProjectScanner {
public:
    ProjectScanner();
    ~ProjectScanner();
    ProjectScanner(const ProjectScanner&) = delete;
    ProjectScanner& operator=(const ProjectScanner&) = delete;
    void start(const std::filesystem::path& root, std::size_t max_depth = 8, std::size_t limit = 100);
    ScanSnapshot snapshot() const;

private:
    struct Impl;
    Impl* impl_;
};
}
