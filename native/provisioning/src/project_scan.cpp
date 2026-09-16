#include "aetheris/project_scan.hpp"

#include <windows.h>

#include <algorithm>
#include <mutex>
#include <set>
#include <thread>
#include <functional>

namespace {
std::wstring folded(std::wstring value) {
    if (!value.empty()) CharLowerBuffW(value.data(), static_cast<DWORD>(value.size()));
    return value;
}

bool excluded(const std::filesystem::path& path) {
    static const std::set<std::wstring> names{
        L".git", L".svn", L"node_modules", L".venv", L"venv", L"dist",
        L"build", L".cache", L"__pycache__", L"bin", L"obj",
    };
    return names.count(folded(path.filename().wstring())) > 0;
}

bool reparse_point(const std::filesystem::path& path) {
    const DWORD attributes = GetFileAttributesW(path.c_str());
    return attributes != INVALID_FILE_ATTRIBUTES && (attributes & FILE_ATTRIBUTE_REPARSE_POINT) != 0;
}

std::wstring detect_vcs(const std::filesystem::path& path) {
    std::error_code error;
    if (std::filesystem::exists(path / L".git", error)) return L"git";
    error.clear();
    if (std::filesystem::is_directory(path / L".svn", error)) return L"svn";
    return {};
}
}

namespace aetheris {
namespace {
ScanSnapshot scan_projects_impl(
    const std::filesystem::path& root,
    std::size_t max_depth,
    std::size_t limit,
    const std::function<void(const ScanSnapshot&)>& publish) {
    ScanSnapshot result;
    result.state = ScanState::running;
    std::error_code error;
    if (!std::filesystem::is_directory(root, error)) {
        result.state = ScanState::warning;
        result.error_code = L"root_unavailable";
        publish(result);
        return result;
    }

    std::set<std::wstring> seen;
    auto add_project = [&](const std::filesystem::path& candidate, const std::wstring& vcs) {
        if (vcs.empty()) return true;
        auto canonical = std::filesystem::weakly_canonical(candidate, error);
        if (error) {
            error.clear();
            canonical = candidate.lexically_normal();
        }
        const auto key = folded(canonical.wstring());
        if (!seen.insert(key).second) return true;
        if (result.projects.size() >= limit) {
            result.truncated = true;
            return false;
        }
        result.projects.push_back(ProjectSource{canonical, vcs});
        return true;
    };

    if (!add_project(root, detect_vcs(root))) return result;
    publish(result);
    std::size_t published_projects = result.projects.size();
    std::filesystem::recursive_directory_iterator iterator(root, std::filesystem::directory_options::skip_permission_denied, error);
    const std::filesystem::recursive_directory_iterator end;
    while (!error && iterator != end) {
        const auto entry = *iterator;
        if (entry.is_directory(error)) {
            ++result.visited;
            if (excluded(entry.path()) || reparse_point(entry.path())) {
                iterator.disable_recursion_pending();
            } else {
                if (!add_project(entry.path(), detect_vcs(entry.path()))) break;
                if (static_cast<std::size_t>(iterator.depth() + 1) >= max_depth) iterator.disable_recursion_pending();
            }
            if (result.visited % 16 == 0 || result.projects.size() != published_projects) {
                publish(result);
                published_projects = result.projects.size();
            }
        }
        error.clear();
        iterator.increment(error);
    }
    if (error) {
        result.state = ScanState::warning;
        result.error_code = L"scan_partial";
    } else {
        result.state = ScanState::complete;
    }
    std::sort(result.projects.begin(), result.projects.end(), [](const ProjectSource& left, const ProjectSource& right) {
        return folded(left.path.wstring()) < folded(right.path.wstring());
    });
    publish(result);
    return result;
}
}

ScanSnapshot scan_projects(const std::filesystem::path& root, std::size_t max_depth, std::size_t limit) {
    return scan_projects_impl(root, max_depth, limit, [](const ScanSnapshot&) {});
}

struct ProjectScanner::Impl {
    mutable std::mutex mutex;
    std::thread worker;
    ScanSnapshot value;
};

ProjectScanner::ProjectScanner() : impl_(new Impl()) {}

ProjectScanner::~ProjectScanner() {
    if (impl_->worker.joinable()) impl_->worker.join();
    delete impl_;
}

void ProjectScanner::start(const std::filesystem::path& root, std::size_t max_depth, std::size_t limit) {
    if (impl_->worker.joinable()) impl_->worker.join();
    {
        std::lock_guard<std::mutex> guard(impl_->mutex);
        impl_->value = ScanSnapshot{};
        impl_->value.state = ScanState::running;
    }
    impl_->worker = std::thread([this, root, max_depth, limit] {
        auto value = scan_projects_impl(root, max_depth, limit, [this](const ScanSnapshot& progress) {
            std::lock_guard<std::mutex> guard(impl_->mutex);
            impl_->value = progress;
        });
        std::lock_guard<std::mutex> guard(impl_->mutex);
        impl_->value = std::move(value);
    });
}

ScanSnapshot ProjectScanner::snapshot() const {
    std::lock_guard<std::mutex> guard(impl_->mutex);
    return impl_->value;
}
}
